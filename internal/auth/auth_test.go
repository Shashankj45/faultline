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
