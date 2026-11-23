package auth

import (
	"context"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/faultline-proxy/faultline/internal/store"
	"github.com/faultline-proxy/faultline/pkg/models"
)

func TestAuthServiceAuthenticationAndRoles(t *testing.T) {
	sqlStore, err := store.NewSQLStore("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("failed to create sql store: %v", err)
	}
	defer sqlStore.Close()

	authService := NewAuthService(sqlStore)

	// 1. Generate an operator key
	rawKey, keyModel, err := GenerateAPIKey("Test Operator Key", models.RoleOperator, nil)
	if err != nil {
		t.Fatalf("failed to generate key: %v", err)
	}

	if err := sqlStore.CreateAPIKey(context.Background(), keyModel); err != nil {
		t.Fatalf("failed to insert key: %v", err)
	}

	// 2. Authenticate with X-API-Key header
	req := httptest.NewRequest("GET", "/api/v1/experiments", nil)
	req.Header.Set("X-API-Key", rawKey)

	authedKey, err := authService.Authenticate(req)
	if err != nil {
		t.Fatalf("expected successful authentication, got error: %v", err)
	}

	if authedKey.ID != keyModel.ID || authedKey.Role != models.RoleOperator {
		t.Fatalf("unexpected authenticated key: %+v", authedKey)
	}

	// 3. Test Role Enforcement
	if !EnforceRole(authedKey.Role, models.RoleViewer) {
		t.Fatal("operator should satisfy viewer role requirement")
	}
	if !EnforceRole(authedKey.Role, models.RoleOperator) {
		t.Fatal("operator should satisfy operator role requirement")
	}
	if EnforceRole(authedKey.Role, models.RoleAdmin) {
		t.Fatal("operator should NOT satisfy admin role requirement")
	}

	// 4. Test Bearer Token header authentication
	reqBearer := httptest.NewRequest("GET", "/api/v1/experiments", nil)
	reqBearer.Header.Set("Authorization", "Bearer "+rawKey)

	authedBearerKey, err := authService.Authenticate(reqBearer)
	if err != nil {
		t.Fatalf("expected successful Bearer authentication, got error: %v", err)
	}
	if authedBearerKey.ID != keyModel.ID {
		t.Fatalf("expected key ID %s, got %s", keyModel.ID, authedBearerKey.ID)
	}

	// 5. Test Invalid Key
	reqBad := httptest.NewRequest("GET", "/api/v1/experiments", nil)
	reqBad.Header.Set("X-API-Key", "fl_live_invalidkey1234567890")
	_, errBad := authService.Authenticate(reqBad)
	if errBad != ErrUnauthorized {
		t.Fatalf("expected ErrUnauthorized for bad key, got %v", errBad)
	}

	// Wait briefly to allow async goroutine to update last_used_at without panic
	time.Sleep(50 * time.Millisecond)
}

func TestRevokedKeyRejectionAndRotation(t *testing.T) {
	sqlStore, err := store.NewSQLStore("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("failed to create store: %v", err)
	}
	defer sqlStore.Close()

	authService := NewAuthService(sqlStore)
	ctx := context.Background()

	// 1. Generate initial admin key
	oldRawKey, oldKeyModel, err := GenerateAPIKey("Old Admin Key", models.RoleAdmin, nil)
	if err != nil {
		t.Fatalf("failed to generate key: %v", err)
	}
	if err := sqlStore.CreateAPIKey(ctx, oldKeyModel); err != nil {
		t.Fatalf("failed to insert key: %v", err)
	}

	// Verify old key works initially
	reqOld := httptest.NewRequest("GET", "/api/v1/upstreams", nil)
	reqOld.Header.Set("X-API-Key", oldRawKey)
	k, err := authService.Authenticate(reqOld)
	if err != nil || k.ID != oldKeyModel.ID {
		t.Fatalf("expected old key to work initially, got error: %v", err)
	}

	// 2. Revoke the old key
	if err := sqlStore.RevokeAPIKey(ctx, oldKeyModel.ID); err != nil {
		t.Fatalf("failed to revoke old key: %v", err)
	}

	// Verify old key immediately fails authentication
	_, errRevoked := authService.Authenticate(reqOld)
	if errRevoked != ErrUnauthorized {
		t.Fatalf("expected ErrUnauthorized for revoked key, got %v", errRevoked)
	}

	// 3. Provision new replacement key (rotation)
	newRawKey, newKeyModel, err := GenerateAPIKey("Rotated Replacement Admin Key", models.RoleAdmin, nil)
	if err != nil {
		t.Fatalf("failed to generate replacement key: %v", err)
	}
	if err := sqlStore.CreateAPIKey(ctx, newKeyModel); err != nil {
		t.Fatalf("failed to insert replacement key: %v", err)
	}

	// 4. Verify replacement key succeeds
	reqNew := httptest.NewRequest("GET", "/api/v1/upstreams", nil)
	reqNew.Header.Set("X-API-Key", newRawKey)
	kNew, errNew := authService.Authenticate(reqNew)
	if errNew != nil || kNew.ID != newKeyModel.ID {
		t.Fatalf("expected replacement key to succeed, got error: %v", errNew)
	}

	// 5. Verify old key STILL fails
	_, errOldAgain := authService.Authenticate(reqOld)
	if errOldAgain != ErrUnauthorized {
		t.Fatalf("expected old key to remain rejected after rotation, got %v", errOldAgain)
	}
}
