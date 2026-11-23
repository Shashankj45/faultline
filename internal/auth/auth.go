package auth

import (
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/faultline-proxy/faultline/internal/store"
	"github.com/faultline-proxy/faultline/pkg/models"
	"golang.org/x/crypto/bcrypt"
)

var (
	ErrUnauthorized = errors.New("unauthorized: missing or invalid credentials")
	ErrForbidden    = errors.New("forbidden: insufficient role permissions")
)

type AuthService struct {
	store store.Store
}

func NewAuthService(s store.Store) *AuthService {
	return &AuthService{store: s}
}

// HashPassword hashes a plaintext password with bcrypt.
func HashPassword(password string) (string, error) {
	bytes, err := bcrypt.GenerateFromPassword([]byte(password), 12)
	return string(bytes), err
}

// CheckPasswordHash compares a plaintext password with its bcrypt hash.
func CheckPasswordHash(password, hash string) bool {
	return bcrypt.CompareHashAndPassword([]byte(hash), []byte(password)) == nil
}

// GenerateAPIKey creates a new API key and returns (rawKey, APIKeyModel, error).
func GenerateAPIKey(name string, role models.Role, expiresAt *time.Time) (string, *models.APIKey, error) {
	randomBytes := make([]byte, 24)
	if _, err := rand.Read(randomBytes); err != nil {
		return "", nil, err
	}

	secret := hex.EncodeToString(randomBytes)
	rawKey := fmt.Sprintf("fl_live_%s", secret)
	keyHash := HashKey(rawKey)
	keyPrefix := rawKey[:11] // "fl_live_..."

	apiKey := &models.APIKey{
		ID:        fmt.Sprintf("key_%s", hex.EncodeToString(randomBytes[:6])),
		Name:      name,
		KeyHash:   keyHash,
		KeyPrefix: keyPrefix,
		Role:      role,
		ExpiresAt: expiresAt,
		IsActive:  true,
		CreatedAt: time.Now().UTC(),
	}

	return rawKey, apiKey, nil
}

// HashKey computes SHA-256 for indexed lookup.
func HashKey(rawKey string) string {
	h := sha256.Sum256([]byte(rawKey))
	return hex.EncodeToString(h[:])
}

// Authenticate extracts and validates API key or Token from HTTP request.
func (a *AuthService) Authenticate(r *http.Request) (*models.APIKey, error) {
	rawKey := ""
	authHeader := r.Header.Get("Authorization")
	if strings.HasPrefix(authHeader, "Bearer ") {
		rawKey = strings.TrimPrefix(authHeader, "Bearer ")
	} else if r.Header.Get("X-API-Key") != "" {
		rawKey = r.Header.Get("X-API-Key")
	}

	if rawKey == "" {
		return nil, ErrUnauthorized
	}

	keyHash := HashKey(rawKey)
	apiKey, err := a.store.GetAPIKeyByHash(r.Context(), keyHash)
	if err != nil || !apiKey.IsActive {
		return nil, ErrUnauthorized
	}

	// Constant-time hash verification to eliminate timing attack vectors
	if subtle.ConstantTimeCompare([]byte(apiKey.KeyHash), []byte(keyHash)) != 1 {
		return nil, ErrUnauthorized
	}

	if apiKey.ExpiresAt != nil && apiKey.ExpiresAt.Before(time.Now()) {
		return nil, ErrUnauthorized
	}

	// Update last used time asynchronously
	go func(id string) {
		_ = a.store.UpdateAPIKeyLastUsed(r.Context(), id, time.Now().UTC())
	}(apiKey.ID)

	return apiKey, nil
}

// EnforceRole checks if user/key role has at least the required permission tier.
func EnforceRole(actual models.Role, required models.Role) bool {
	tier := map[models.Role]int{
		models.RoleViewer:   1,
		models.RoleOperator: 2,
		models.RoleAdmin:    3,
	}
	return tier[actual] >= tier[required]
}
