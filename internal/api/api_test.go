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
	router.SetDevMode(true)
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

func TestKillSwitchBlocksExperimentActivation(t *testing.T) {
	router, sqlStore := setupTestAPI(t)
	ctx := context.Background()

	// 1. Pre-seed upstream and experiment
	u := &models.Upstream{
		ID:        "u-ks-test",
		Name:      "KS Test Upstream",
		Slug:      "ks-test",
		TargetURL: "https://example.com",
		IsActive:  true,
	}
	_ = sqlStore.CreateUpstream(ctx, u)

	exp := &models.Experiment{
		ID:         "exp-ks-blocked",
		UpstreamID: "u-ks-test",
		Name:       "Blocked Under Kill Switch",
		Status:     models.StatusDraft,
	}
	_ = sqlStore.CreateExperiment(ctx, exp)

	// 2. Engage Emergency Kill Switch
	_ = sqlStore.SetKillSwitch(ctx, &models.KillSwitchState{
		GlobalActive: true,
		ActivatedBy:  "lead-engineer",
		Reason:       "emergency test",
	})

	// 3. Attempt to transition draft experiment to running -> Must be rejected with 409 Conflict
	statusPayload := `{"status": "running"}`
	req := httptest.NewRequest("POST", "/api/v1/experiments/exp-ks-blocked/status", bytes.NewBufferString(statusPayload))
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusConflict {
		t.Fatalf("expected 409 Conflict when activating experiment under kill switch, got %d: %s", rec.Code, rec.Body.String())
	}

	// 4. Attempt to create a new experiment directly with status 'running' -> Must be rejected with 409 Conflict
	createRunningPayload := `{
		"upstream_id": "u-ks-test",
		"name": "Direct Running Experiment",
		"status": "running"
	}`
	req = httptest.NewRequest("POST", "/api/v1/experiments", bytes.NewBufferString(createRunningPayload))
	rec = httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusConflict {
		t.Fatalf("expected 409 Conflict when creating running experiment under kill switch, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestBootstrapSecurityProtection(t *testing.T) {
	router, sqlStore := setupTestAPI(t)
	router.SetDevMode(false) // Test strict production security mode

	// Verify initial state: no keys exist in the database
	if sqlStore.HasAPIKeys() {
		t.Fatal("expected empty store to have HasAPIKeys() == false")
	}

	// 1. Remote request to destructive administrative endpoint (kill switch) without key must be blocked (401)
	remoteKillReq := httptest.NewRequest("POST", "/api/v1/killswitch", bytes.NewBufferString(`{"global_active": true}`))
	remoteKillReq.RemoteAddr = "198.51.100.23:44123" // External non-loopback IP
	recKill := httptest.NewRecorder()
	router.ServeHTTP(recKill, remoteKillReq)
	if recKill.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 Unauthorized for remote destructive action in bootstrap mode, got %d: %s", recKill.Code, recKill.Body.String())
	}

	// 2. Remote request to create initial admin key must be permitted in bootstrap mode
	createKeyReq := httptest.NewRequest("POST", "/api/v1/api-keys", bytes.NewBufferString(`{"name": "First Admin", "role": "admin"}`))
	createKeyReq.RemoteAddr = "198.51.100.23:44123"
	recKey := httptest.NewRecorder()
	router.ServeHTTP(recKey, createKeyReq)
	if recKey.Code != http.StatusCreated {
		t.Fatalf("expected 201 Created for initial key registration, got %d: %s", recKey.Code, recKey.Body.String())
	}

	var keyResp struct {
		APIKey    models.APIKey `json:"api_key"`
		SecretKey string        `json:"secret_key"`
	}
	if err := json.Unmarshal(recKey.Body.Bytes(), &keyResp); err != nil || keyResp.SecretKey == "" {
		t.Fatalf("failed to decode key generation response: %v", err)
	}

	// 3. System is now locked: HasAPIKeys() must be true
	if !sqlStore.HasAPIKeys() {
		t.Fatal("expected HasAPIKeys() to be true after key generation")
	}

	// 4. Any subsequent unauthenticated request (even on localhost) must be rejected with 401
	unauthReq := httptest.NewRequest("POST", "/api/v1/killswitch", bytes.NewBufferString(`{"global_active": true}`))
	unauthReq.RemoteAddr = "127.0.0.1:12345"
	recUnauth := httptest.NewRecorder()
	router.ServeHTTP(recUnauth, unauthReq)
	if recUnauth.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 Unauthorized once keys exist, got %d", recUnauth.Code)
	}

	// 5. Authenticated request with the provisioned key must succeed
	authReq := httptest.NewRequest("POST", "/api/v1/killswitch", bytes.NewBufferString(`{"global_active": true, "reason": "auth test"}`))
	authReq.RemoteAddr = "198.51.100.23:44123"
	authReq.Header.Set("X-API-Key", keyResp.SecretKey)
	recAuth := httptest.NewRecorder()
	router.ServeHTTP(recAuth, authReq)
	if recAuth.Code != http.StatusOK {
		t.Fatalf("expected 200 OK with valid API key, got %d: %s", recAuth.Code, recAuth.Body.String())
	}
}
