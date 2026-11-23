package audit

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/faultline-proxy/faultline/internal/store"
	"github.com/faultline-proxy/faultline/pkg/models"
	"github.com/google/uuid"
)

// Logger writes tamper-evident audit records to the storage layer with automatic secret redaction.
type Logger struct {
	store store.Store
}

func NewLogger(s store.Store) *Logger {
	return &Logger{store: s}
}

// Log records an administrative or operational event, strictly scrubbing sensitive credentials.
func (l *Logger) Log(ctx context.Context, userID, action, resType, resID string, details any, clientIP string) {
	detailsJSON := "{}"
	if details != nil {
		if b, err := json.Marshal(details); err == nil {
			detailsJSON = sanitizeDetails(b)
		}
	}

	record := &models.AuditLog{
		ID:           fmt.Sprintf("aud_%s", uuid.New().String()[:8]),
		UserID:       userID,
		Action:       action,
		ResourceType: resType,
		ResourceID:   resID,
		DetailsJSON:  detailsJSON,
		ClientIP:     clientIP,
		Timestamp:    time.Now().UTC(),
	}

	go func(r *models.AuditLog) {
		_ = l.store.CreateAuditLog(context.Background(), r)
	}(record)
}

// sanitizeDetails deep-scans JSON data and redacts API keys, tokens, hashes, and passwords.
func sanitizeDetails(rawJSON []byte) string {
	var m map[string]any
	if err := json.Unmarshal(rawJSON, &m); err != nil {
		// If not a JSON object, return safely stringified
		return string(rawJSON)
	}

	redactMap(m)

	sanitized, err := json.Marshal(m)
	if err != nil {
		return "{}"
	}
	return string(sanitized)
}

func redactMap(m map[string]any) {
	sensitiveKeys := []string{
		"secret_key", "key_hash", "token", "password", "password_hash",
		"authorization", "api_key", "secret", "private_key",
	}

	for k, v := range m {
		lowerKey := strings.ToLower(k)
		for _, sens := range sensitiveKeys {
			if strings.Contains(lowerKey, sens) {
				m[k] = "[REDACTED]"
				break
			}
		}

		// Recurse into nested maps
		if childMap, ok := v.(map[string]any); ok {
			redactMap(childMap)
		}
	}
}
