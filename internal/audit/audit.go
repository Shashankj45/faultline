package audit

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/faultline-proxy/faultline/internal/store"
	"github.com/faultline-proxy/faultline/pkg/models"
	"github.com/google/uuid"
)

// Logger writes tamper-evident audit records to the storage layer.
type Logger struct {
	store store.Store
}

func NewLogger(s store.Store) *Logger {
	return &Logger{store: s}
}

// Log records an administrative or operational event.
func (l *Logger) Log(ctx context.Context, userID, action, resType, resID string, details any, clientIP string) {
	detailsJSON := "{}"
	if details != nil {
		if b, err := json.Marshal(details); err == nil {
			detailsJSON = string(b)
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
