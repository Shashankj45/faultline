package proxy

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/faultline-proxy/faultline/internal/faults"
	"github.com/faultline-proxy/faultline/internal/metrics"
	"github.com/faultline-proxy/faultline/internal/store"
	"github.com/faultline-proxy/faultline/pkg/models"
)

func setupTestProxy(t *testing.T) (*ReverseProxyServer, *store.SQLStore, *httptest.Server) {
	// Create mock upstream backend
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = fmt.Fprintf(w, `{"backend_path":%q,"method":%q}`, r.URL.Path, r.Method)
	}))
	t.Cleanup(backend.Close)

	sqlStore, err := store.NewSQLStore("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("failed to create sql store: %v", err)
	}
	t.Cleanup(func() { _ = sqlStore.Close() })

	engine := faults.NewEngine()
	m := metrics.NewMetrics()
	proxyServer := NewReverseProxyServer(sqlStore, engine, m)

	// Register backend upstream
	u := &models.Upstream{
		ID:                  "u-backend",
		Name:                "Test Backend",
		Slug:                "test-backend",
		TargetURL:           backend.URL,
		AllowLocalhost:      true, // Needed for httptest loopback
		AllowPrivateNetwork: true,
		TimeoutSeconds:      5,
		MaxIdleConns:        10,
		IsActive:            true,
	}
	if err := sqlStore.CreateUpstream(context.Background(), u); err != nil {
		t.Fatalf("failed to create upstream: %v", err)
	}

	return proxyServer, sqlStore, backend
}

func TestProxyNormalForwarding(t *testing.T) {
	proxyServer, _, _ := setupTestProxy(t)

	// Test Path-based routing: /proxy/test-backend/v1/ping
	req := httptest.NewRequest("GET", "/proxy/test-backend/v1/ping", nil)
	rec := httptest.NewRecorder()

	proxyServer.ServeHTTP(rec, req)

	resp := rec.Result()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected status 200, got %d", resp.StatusCode)
	}
	if resp.Header.Get("X-FaultLine-Proxy") != "1.0" {
		t.Fatal("expected X-FaultLine-Proxy header")
	}

	body, _ := io.ReadAll(resp.Body)
	var parsed map[string]string
	_ = json.Unmarshal(body, &parsed)
	if parsed["backend_path"] != "/v1/ping" {
		t.Fatalf("expected backend_path '/v1/ping', got %q", parsed["backend_path"])
	}
}

func TestProxyHTTPErrorInjection(t *testing.T) {
	proxyServer, sqlStore, _ := setupTestProxy(t)
	ctx := context.Background()

	// Create running experiment with HTTP error rule
	exp := &models.Experiment{
		ID:         "exp-err",
		UpstreamID: "u-backend",
		Name:       "Error Injection Test",
		Status:     models.StatusRunning,
	}
	_ = sqlStore.CreateExperiment(ctx, exp)

	errCfg, _ := json.Marshal(models.HTTPErrorConfig{
		StatusCode:   http.StatusServiceUnavailable,
		ResponseBody: `{"error":"chaos simulated 503"}`,
		PreUpstream:  true,
	})

	rule := &models.FaultRule{
		ID:           "r-err-1",
		ExperimentID: "exp-err",
		Name:         "503 Injection",
		FaultType:    models.FaultHTTPError,
		Probability:  1.0,
		Priority:     10,
		Config:       errCfg,
		IsEnabled:    true,
	}
	_ = sqlStore.CreateFaultRule(ctx, rule)

	// Request proxy
	req := httptest.NewRequest("GET", "/proxy/test-backend/checkout", nil)
	rec := httptest.NewRecorder()
	proxyServer.ServeHTTP(rec, req)

	resp := rec.Result()
	if resp.StatusCode != http.StatusServiceUnavailable {
		t.Fatalf("expected injected status 503, got %d", resp.StatusCode)
	}
	if resp.Header.Get("X-FaultLine-Injected") != "true" {
		t.Fatal("expected X-FaultLine-Injected header")
	}

	body, _ := io.ReadAll(resp.Body)
	var parsed map[string]string
	_ = json.Unmarshal(body, &parsed)
	if parsed["error"] != "chaos simulated 503" {
		t.Fatalf("unexpected injected body: %s", string(body))
	}
}

func TestProxyKillSwitchBypassesChaos(t *testing.T) {
	proxyServer, sqlStore, _ := setupTestProxy(t)
	ctx := context.Background()

	// Create running experiment with error rule
	exp := &models.Experiment{
		ID:         "exp-kill",
		UpstreamID: "u-backend",
		Name:       "Kill Switch Experiment",
		Status:     models.StatusRunning,
	}
	_ = sqlStore.CreateExperiment(ctx, exp)

	errCfg, _ := json.Marshal(models.HTTPErrorConfig{
		StatusCode:   http.StatusInternalServerError,
		ResponseBody: `{"error":"failure"}`,
		PreUpstream:  true,
	})
	rule := &models.FaultRule{
		ID:           "r-kill-1",
		ExperimentID: "exp-kill",
		Name:         "500 Rule",
		FaultType:    models.FaultHTTPError,
		Probability:  1.0,
		Config:       errCfg,
		IsEnabled:    true,
	}
	_ = sqlStore.CreateFaultRule(ctx, rule)

	// Activate Emergency Kill Switch!
	_ = sqlStore.SetKillSwitch(ctx, &models.KillSwitchState{
		GlobalActive: true,
		ActivatedBy:  "lead-engineer",
		Reason:       "emergency test",
	})

	// Perform request - Kill Switch must bypass chaos and return 200 OK from backend!
	req := httptest.NewRequest("GET", "/proxy/test-backend/data", nil)
	rec := httptest.NewRecorder()
	proxyServer.ServeHTTP(rec, req)

	resp := rec.Result()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected kill switch to bypass chaos and return 200 OK, got %d", resp.StatusCode)
	}
	if resp.Header.Get("X-FaultLine-Injected") == "true" {
		t.Fatal("expected no injected chaos header when kill switch is active")
	}
}

func TestProxyLatencyInjection(t *testing.T) {
	proxyServer, sqlStore, _ := setupTestProxy(t)
	ctx := context.Background()

	exp := &models.Experiment{
		ID:         "exp-lat",
		UpstreamID: "u-backend",
		Name:       "Latency Experiment",
		Status:     models.StatusRunning,
	}
	_ = sqlStore.CreateExperiment(ctx, exp)

	latCfg, _ := json.Marshal(models.LatencyConfig{
		Distribution:   models.DistFixed,
		FixedLatencyMs: 60,
	})
	rule := &models.FaultRule{
		ID:           "r-lat-1",
		ExperimentID: "exp-lat",
		Name:         "Fixed Latency",
		FaultType:    models.FaultLatency,
		Probability:  1.0,
		Config:       latCfg,
		IsEnabled:    true,
	}
	_ = sqlStore.CreateFaultRule(ctx, rule)

	start := time.Now()
	req := httptest.NewRequest("GET", "/proxy/test-backend/delay-test", nil)
	rec := httptest.NewRecorder()
	proxyServer.ServeHTTP(rec, req)

	duration := time.Since(start)
	if duration < 50*time.Millisecond {
		t.Fatalf("expected injected delay of at least 50ms, got %v", duration)
	}
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 OK after delay, got %d", rec.Code)
	}
}

func TestProxyLoopDetection(t *testing.T) {
	proxyServer, _, _ := setupTestProxy(t)

	// Send request with hops count >= 10
	req := httptest.NewRequest("GET", "/proxy/test-backend/loop", nil)
	req.Header.Set("X-FaultLine-Hops", "10")
	rec := httptest.NewRecorder()

	proxyServer.ServeHTTP(rec, req)

	if rec.Code != http.StatusLoopDetected {
		t.Fatalf("expected 508 Loop Detected, got %d", rec.Code)
	}
}

func TestProxyEviction(t *testing.T) {
	proxyServer, sqlStore, _ := setupTestProxy(t)

	// Make request to populate cache
	req := httptest.NewRequest("GET", "/proxy/test-backend/ping", nil)
	rec := httptest.NewRecorder()
	proxyServer.ServeHTTP(rec, req)

	proxyServer.proxiesMu.RLock()
	_, found := proxyServer.proxies["u-backend"]
	proxyServer.proxiesMu.RUnlock()
	if !found {
		t.Fatal("expected u-backend to be cached in reverse proxy map")
	}

	// Evict proxy
	proxyServer.EvictProxy("u-backend")

	proxyServer.proxiesMu.RLock()
	_, foundAfter := proxyServer.proxies["u-backend"]
	proxyServer.proxiesMu.RUnlock()
	if foundAfter {
		t.Fatal("expected u-backend to be evicted from reverse proxy map")
	}
	_ = sqlStore
}

func TestHijackerInterface(t *testing.T) {
	rec := &statusRecorder{
		ResponseWriter: httptest.NewRecorder(),
	}
	// Verify it satisfies http.Hijacker
	var _ http.Hijacker = rec

	pl := &packetLossResponseWriter{
		ResponseWriter: httptest.NewRecorder(),
	}
	var _ http.Hijacker = pl
}
