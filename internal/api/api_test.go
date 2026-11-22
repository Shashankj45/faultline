package api

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/faultline-proxy/faultline/internal/audit"
	"github.com/faultline-proxy/faultline/internal/auth"
	"github.com/faultline-proxy/faultline/internal/faults"
	"github.com/faultline-proxy/faultline/internal/health"
	"github.com/faultline-proxy/faultline/internal/metrics"
	"github.com/faultline-proxy/faultline/internal/proxy"
	"github.com/faultline-proxy/faultline/internal/store"
	"github.com/faultline-proxy/faultline/pkg/models"
)

func setupTestAPI(t *testing.T) (*Router, *store.SQLStore) {
	sqlStore, err := store.NewSQLStore("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("failed to create store: %v", err)
	}
	t.Cleanup(func() { _ = sqlStore.Close() })

	engine := faults.NewEngine()
	m := metrics.NewMetrics()
	p := proxy.NewReverseProxyServer(sqlStore, engine, m)
	hm := health.NewMonitor(m)
	al := audit.NewLogger(sqlStore)
	as := auth.NewAuthService(sqlStore)

	router := NewRouter(sqlStore, p, hm, m, al, as)
	return router, sqlStore
}

func TestAPIHealthAndLiveMetrics(t *testing.T) {
	router, _ := setupTestAPI(t)

	// Health
	req := httptest.NewRequest("GET", "/api/v1/health", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", rec.Code)
	}

	// Live Metrics
	req = httptest.NewRequest("GET", "/api/v1/metrics/live", nil)
	rec = httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", rec.Code)
	}

	var stats metrics.LiveStats
	_ = json.Unmarshal(rec.Body.Bytes(), &stats)
	if stats.RecentHistory == nil {
		t.Fatal("expected non-nil RecentHistory")
	}
}

func TestAPIKillSwitchToggle(t *testing.T) {
	router, sqlStore := setupTestAPI(t)

	// Activate Kill Switch
	payload := `{"global_active": true, "reason": "SRE drill test", "activated_by": "sre-lead"}`
	req := httptest.NewRequest("POST", "/api/v1/killswitch", bytes.NewBufferString(payload))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d: %s", rec.Code, rec.Body.String())
	}

	ks, _ := sqlStore.GetKillSwitch(context.Background())
	if !ks.GlobalActive || ks.ActivatedBy != "sre-lead" {
		t.Fatalf("unexpected kill switch state in DB: %+v", ks)
	}
}

func TestAPIUpstreamSSRFBlock(t *testing.T) {
	router, _ := setupTestAPI(t)

	// Attempt to register cloud metadata IP as an upstream
	payload := `{
		"name": "Cloud Metadata",
		"slug": "metadata",
		"target_url": "http://169.254.169.254/latest/meta-data",
		"allow_localhost": false,
		"allow_private_network": false
	}`
	req := httptest.NewRequest("POST", "/api/v1/upstreams", bytes.NewBufferString(payload))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	// Must be blocked with 422 Unprocessable Entity!
	if rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("expected SSRF block 422, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestAPIExperimentLifecycle(t *testing.T) {
	router, sqlStore := setupTestAPI(t)

	// Pre-seed an upstream
	u := &models.Upstream{
		ID:                  "u-demo",
		Name:                "Demo Service",
		Slug:                "demo",
		TargetURL:           "https://demo.example.com",
		AllowLocalhost:      false,
		AllowPrivateNetwork: false,
		IsActive:            true,
	}
	_ = sqlStore.CreateUpstream(context.Background(), u)

	// Create Experiment
	expPayload := `{
		"upstream_id": "u-demo",
		"name": "Chaos Experiment 1",
		"description": "Evaluate retry behavior"
	}`
	req := httptest.NewRequest("POST", "/api/v1/experiments", bytes.NewBufferString(expPayload))
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusCreated {
		t.Fatalf("expected 201 Created, got %d: %s", rec.Code, rec.Body.String())
	}

	var created models.Experiment
	_ = json.Unmarshal(rec.Body.Bytes(), &created)
	if created.ID == "" || created.Status != models.StatusDraft {
		t.Fatalf("unexpected experiment state: %+v", created)
	}

	// Update Status to Running
	statusPayload := `{"status": "running"}`
	req = httptest.NewRequest("POST", "/api/v1/experiments/"+created.ID+"/status", bytes.NewBufferString(statusPayload))
	rec = httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 OK, got %d: %s", rec.Code, rec.Body.String())
	}

	exp, _ := sqlStore.GetExperimentByID(context.Background(), created.ID)
	if exp.Status != models.StatusRunning {
		t.Fatalf("expected status 'running', got %s", exp.Status)
	}
}
