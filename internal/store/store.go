package store

import (
	"context"
	"time"

	"github.com/faultline-proxy/faultline/pkg/models"
)

// Store defines the persistence contract for FaultLine.
type Store interface {
	// Lifecycle
	Close() error

	// Upstreams
	CreateUpstream(ctx context.Context, u *models.Upstream) error
	GetUpstreamByID(ctx context.Context, id string) (*models.Upstream, error)
	GetUpstreamBySlug(ctx context.Context, slug string) (*models.Upstream, error)
	ListUpstreams(ctx context.Context) ([]models.Upstream, error)
	UpdateUpstream(ctx context.Context, u *models.Upstream) error
	DeleteUpstream(ctx context.Context, id string) error

	// Experiments
	CreateExperiment(ctx context.Context, exp *models.Experiment) error
	GetExperimentByID(ctx context.Context, id string) (*models.Experiment, error)
	ListExperiments(ctx context.Context, upstreamID string) ([]models.Experiment, error)
	UpdateExperiment(ctx context.Context, exp *models.Experiment) error
	UpdateExperimentStatus(ctx context.Context, id string, status models.ExperimentStatus) error
	DeleteExperiment(ctx context.Context, id string) error

	// Fault Rules
	CreateFaultRule(ctx context.Context, rule *models.FaultRule) error
	GetFaultRuleByID(ctx context.Context, id string) (*models.FaultRule, error)
	ListRulesByExperiment(ctx context.Context, experimentID string) ([]models.FaultRule, error)
	UpdateFaultRule(ctx context.Context, rule *models.FaultRule) error
	DeleteFaultRule(ctx context.Context, id string) error

	// Active Rule Snapshot for Hot-Path
	GetActiveRulesForUpstream(ctx context.Context, upstreamID string) ([]models.FaultRule, error)

	// Kill Switch
	GetKillSwitch(ctx context.Context) (*models.KillSwitchState, error)
	SetKillSwitch(ctx context.Context, state *models.KillSwitchState) error

	// Audit Logs
	CreateAuditLog(ctx context.Context, log *models.AuditLog) error
	ListAuditLogs(ctx context.Context, limit, offset int) ([]models.AuditLog, error)

	// Users & Auth
	CreateUser(ctx context.Context, user *models.User) error
	GetUserByUsername(ctx context.Context, username string) (*models.User, error)
	GetUserByID(ctx context.Context, id string) (*models.User, error)

	// API Keys
	CreateAPIKey(ctx context.Context, key *models.APIKey) error
	GetAPIKeyByHash(ctx context.Context, keyHash string) (*models.APIKey, error)
	ListAPIKeys(ctx context.Context) ([]models.APIKey, error)
	RevokeAPIKey(ctx context.Context, id string) error
	UpdateAPIKeyLastUsed(ctx context.Context, id string, t time.Time) error
}
