package faults

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/faultline-proxy/faultline/pkg/models"
)

func TestMatcher(t *testing.T) {
	matcher := NewMatcher()

	rule := &models.FaultRule{
		ID:               "r-1",
		IsEnabled:        true,
		MatchMethods:     []string{"POST", "PUT"},
		MatchPathPattern: "^/api/v1/orders/.*",
		MatchHeaders: map[string]string{
			"X-User-Tier": "^(gold|platinum)$",
		},
		MatchQueryParams: map[string]string{
			"simulate": "true",
		},
		MatchClientIPs: []string{"192.168.1.0/24", "10.0.0.15"},
		Probability:    1.0,
	}

	// 1. Matches exact criteria
	req, _ := http.NewRequest("POST", "/api/v1/orders/12345?simulate=true", nil)
	req.Header.Set("X-User-Tier", "platinum")
	if !matcher.Matches(req, "192.168.1.45", rule) {
		t.Fatal("expected request to match rule criteria")
	}

	// 2. Mismatched method
	getReq, _ := http.NewRequest("GET", "/api/v1/orders/12345?simulate=true", nil)
	getReq.Header.Set("X-User-Tier", "platinum")
	if matcher.Matches(getReq, "192.168.1.45", rule) {
		t.Fatal("expected GET to not match POST/PUT rule")
	}

	// 3. Mismatched path
	badPathReq, _ := http.NewRequest("POST", "/api/v2/other/12345?simulate=true", nil)
	badPathReq.Header.Set("X-User-Tier", "platinum")
	if matcher.Matches(badPathReq, "192.168.1.45", rule) {
		t.Fatal("expected bad path to not match")
	}

	// 4. Mismatched header
	badHeaderReq, _ := http.NewRequest("POST", "/api/v1/orders/12345?simulate=true", nil)
	badHeaderReq.Header.Set("X-User-Tier", "silver")
	if matcher.Matches(badHeaderReq, "192.168.1.45", rule) {
		t.Fatal("expected silver tier to not match gold|platinum")
	}

	// 5. Mismatched IP CIDR
	if matcher.Matches(req, "172.16.5.1", rule) {
		t.Fatal("expected external IP to not match 192.168.1.0/24 or 10.0.0.15")
	}
}

func TestApplyLatency(t *testing.T) {
	engine := NewEngine()

	// 1. Fixed latency
	fixedCfg := &models.LatencyConfig{
		Distribution:   models.DistFixed,
		FixedLatencyMs: 50,
	}
	start := time.Now()
	err := engine.ApplyLatency(context.Background(), fixedCfg, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	elapsed := time.Since(start)
	if elapsed < 40*time.Millisecond {
		t.Fatalf("expected delay of at least 40ms, got %v", elapsed)
	}

	// 2. Context Cancellation check
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Abort immediately

	err = engine.ApplyLatency(ctx, fixedCfg, nil)
	if err != ErrRequestAborted {
		t.Fatalf("expected ErrRequestAborted on cancelled context, got %v", err)
	}
}

func TestInjectHTTPError(t *testing.T) {
	engine := NewEngine()
	rec := httptest.NewRecorder()

	cfg := &models.HTTPErrorConfig{
		StatusCode:   http.StatusServiceUnavailable,
		ResponseBody: `{"error":"upstream down for maintenance"}`,
		Headers: map[string]string{
			"Retry-After": "120",
		},
	}

	engine.InjectHTTPError(rec, cfg)

	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected status 503, got %d", rec.Code)
	}
	if rec.Header().Get("Retry-After") != "120" {
		t.Fatalf("expected Retry-After 120, got %s", rec.Header().Get("Retry-After"))
	}
	if rec.Header().Get("X-FaultLine-Injected") != "true" {
		t.Fatal("expected X-FaultLine-Injected header to be true")
	}

	var parsed map[string]string
	_ = json.Unmarshal(rec.Body.Bytes(), &parsed)
	if parsed["error"] != "upstream down for maintenance" {
		t.Fatalf("unexpected body: %s", rec.Body.String())
	}
}

func TestBurstDetection(t *testing.T) {
	cfg := &models.BurstConfig{
		BurstIntervalMs: 1000,
		BurstDurationMs: 500,
	}

	// Burst active check returns boolean without panicking
	_ = IsBurstActive(cfg)
}

func TestThrottledStream(t *testing.T) {
	data := bytes.Repeat([]byte("A"), 1024) // 1KB
	reader := bytes.NewReader(data)

	// Throttle at 20KB/s
	throttled := WrapThrottledReader(reader, 20*1024)
	buf := make([]byte, 512)
	n, err := throttled.Read(buf)
	if err != nil && err != io.EOF {
		t.Fatalf("unexpected read error: %v", err)
	}
	if n != 512 {
		t.Fatalf("expected 512 bytes, got %d", n)
	}
}
