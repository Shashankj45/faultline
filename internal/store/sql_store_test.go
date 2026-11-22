package store

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/faultline-proxy/faultline/pkg/models"
)

func newTestStore(t *testing.T) *SQLStore {
	store, err := NewSQLStore("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("failed to create test store: %v", err)
	}
	t.Cleanup(func() {
		_ = store.Close()
	})
	return store
}

func TestSQLStoreUpstreamCRUD(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()

	u := &models.Upstream{
		ID:                         "u-1",
		Name:                       "Payment Gateway",
		Slug:                       "payments",
		TargetURL:                  "https://payments.example.com",
		AllowLocalhost:             false,
		AllowPrivateNetwork:        false,
		TimeoutSeconds:             15,
		MaxIdleConns:               50,
		IdleConnTimeoutSeconds:     60,
		ResponseHeaderTimeoutSec:   10,
		HealthCheckPath:            "/healthz",
		HealthCheckIntervalSeconds: 5,
		IsActive:                   true,
	}

	// Create
	if err := store.CreateUpstream(ctx, u); err != nil {
		t.Fatalf("failed to create upstream: %v", err)
	}

	// Duplicate slug conflict check
	dup := *u
	dup.ID = "u-2"
	if err := store.CreateUpstream(ctx, &dup); err != ErrConflict {
		t.Fatalf("expected ErrConflict on duplicate slug, got %v", err)
	}

	// Get by ID
	fetched, err := store.GetUpstreamByID(ctx, "u-1")
	if err != nil {
		t.Fatalf("failed to get upstream: %v", err)
	}
	if fetched.Name != "Payment Gateway" || fetched.Slug != "payments" {
		t.Fatalf("unexpected upstream values: %+v", fetched)
	}

	// Cached lookup
	cached, ok := store.CachedUpstreamBySlug("payments")
	if !ok || cached.ID != "u-1" {
		t.Fatalf("expected cached upstream by slug, got %v, ok=%v", cached, ok)
	}

	// Update
	u.Name = "Payment Gateway V2"
	if err := store.UpdateUpstream(ctx, u); err != nil {
		t.Fatalf("failed to update upstream: %v", err)
	}
	updated, _ := store.GetUpstreamByID(ctx, "u-1")
	if updated.Name != "Payment Gateway V2" {
		t.Fatalf("expected updated name, got %s", updated.Name)
	}

	// Delete
	if err := store.DeleteUpstream(ctx, "u-1"); err != nil {
		t.Fatalf("failed to delete upstream: %v", err)
	}
	_, err = store.GetUpstreamByID(ctx, "u-1")
	if err != ErrNotFound {
		t.Fatalf("expected ErrNotFound after deletion, got %v", err)
	}
}

func TestSQLStoreExperimentsAndActiveRules(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()

	u := &models.Upstream{
		ID:        "u-order",
		Name:      "Order Service",
		Slug:      "orders",
		TargetURL: "https://orders.internal",
		IsActive:  true,
	}
	if err := store.CreateUpstream(ctx, u); err != nil {
		t.Fatalf("create upstream failed: %v", err)
	}

	exp := &models.Experiment{
		ID:                 "exp-1",
		UpstreamID:         "u-order",
		Name:               "Black Friday Latency Injection",
		Status:             models.StatusDraft,
		MaxDurationSeconds: 1800,
		CreatedBy:          "lead-sre",
	}
	if err := store.CreateExperiment(ctx, exp); err != nil {
		t.Fatalf("create experiment failed: %v", err)
	}

	ruleCfg, _ := json.Marshal(models.LatencyConfig{
		Distribution:   models.DistFixed,
		FixedLatencyMs: 300,
	})

	rule := &models.FaultRule{
		ID:               "r-1",
		ExperimentID:     "exp-1",
		Name:             "Checkout Latency",
		FaultType:        models.FaultLatency,
		Probability:      1.0,
		Priority:         10,
		MatchMethods:     []string{"POST"},
		MatchPathPattern: "/checkout",
		Config:           ruleCfg,
		IsEnabled:        true,
	}
	if err := store.CreateFaultRule(ctx, rule); err != nil {
		t.Fatalf("create rule failed: %v", err)
	}

	// While experiment is in Draft, active rules must be empty!
	active, err := store.GetActiveRulesForUpstream(ctx, "u-order")
	if err != nil {
		t.Fatalf("query active rules failed: %v", err)
	}
	if len(active) != 0 {
		t.Fatalf("expected 0 active rules while experiment is in draft, got %d", len(active))
	}

	// Start experiment
	if err := store.UpdateExperimentStatus(ctx, "exp-1", models.StatusRunning); err != nil {
		t.Fatalf("update status failed: %v", err)
	}

	// Active rules must now contain the rule
	active, err = store.GetActiveRulesForUpstream(ctx, "u-order")
	if err != nil {
		t.Fatalf("query active rules failed: %v", err)
	}
	if len(active) != 1 {
		t.Fatalf("expected 1 active rule when running, got %d", len(active))
	}
	if active[0].ID != "r-1" {
		t.Fatalf("unexpected rule id: %s", active[0].ID)
	}
}

func TestSQLStoreKillSwitch(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()

	ks, err := store.GetKillSwitch(ctx)
	if err != nil {
		t.Fatalf("failed to get kill switch: %v", err)
	}
	if ks.GlobalActive {
		t.Fatal("expected kill switch to be inactive initially")
	}

	// Activate kill switch
	newKS := &models.KillSwitchState{
		GlobalActive: true,
		ActivatedBy:  "incident-commander",
		Reason:       "High customer error rate detected in production",
	}
	if err := store.SetKillSwitch(ctx, newKS); err != nil {
		t.Fatalf("failed to set kill switch: %v", err)
	}

	updated, err := store.GetKillSwitch(ctx)
	if err != nil {
		t.Fatalf("failed to get kill switch: %v", err)
	}
	if !updated.GlobalActive || updated.ActivatedBy != "incident-commander" {
		t.Fatalf("unexpected kill switch state: %+v", updated)
	}

	// Check cached kill switch
	cached := store.CachedKillSwitch()
	if !cached.GlobalActive {
		t.Fatal("expected cached kill switch to be active")
	}
}

func TestSQLStoreAuditLogsAndAPIKeys(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()

	// Audit Log
	log := &models.AuditLog{
		ID:           "audit-1",
		UserID:       "usr-1",
		Action:       "TRIGGER_KILL_SWITCH",
		ResourceType: "system",
		ResourceID:   "global",
		DetailsJSON:  `{"reason":"test emergency"}`,
		ClientIP:     "192.168.1.5",
		Timestamp:    time.Now().UTC(),
	}
	if err := store.CreateAuditLog(ctx, log); err != nil {
		t.Fatalf("create audit log failed: %v", err)
	}

	logs, err := store.ListAuditLogs(ctx, 10, 0)
	if err != nil {
		t.Fatalf("list audit logs failed: %v", err)
	}
	if len(logs) != 1 || logs[0].Action != "TRIGGER_KILL_SWITCH" {
		t.Fatalf("unexpected audit log: %+v", logs)
	}

	// API Keys
	apiKey := &models.APIKey{
		ID:        "key-1",
		Name:      "CI/CD Runner",
		KeyHash:   "hash_abc_123",
		KeyPrefix: "fl_live_",
		Role:      models.RoleOperator,
		IsActive:  true,
		CreatedAt: time.Now().UTC(),
	}
	if err := store.CreateAPIKey(ctx, apiKey); err != nil {
		t.Fatalf("create api key failed: %v", err)
	}

	fetchedKey, err := store.GetAPIKeyByHash(ctx, "hash_abc_123")
	if err != nil {
		t.Fatalf("get api key failed: %v", err)
	}
	if fetchedKey.Role != models.RoleOperator {
		t.Fatalf("unexpected api key role: %s", fetchedKey.Role)
	}

	if err := store.RevokeAPIKey(ctx, "key-1"); err != nil {
		t.Fatalf("revoke api key failed: %v", err)
	}
	revokedKey, _ := store.GetAPIKeyByHash(ctx, "hash_abc_123")
	if revokedKey.IsActive {
		t.Fatal("expected revoked api key to have is_active = false")
	}
}
