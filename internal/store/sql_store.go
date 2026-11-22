package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/faultline-proxy/faultline/pkg/models"
	_ "github.com/lib/pq"
	_ "modernc.org/sqlite"
)

var (
	ErrNotFound      = errors.New("record not found")
	ErrConflict      = errors.New("resource conflict or unique constraint violation")
	ErrInvalidDriver = errors.New("unsupported database driver")
)

type SQLStore struct {
	db     *sql.DB
	driver string

	// In-memory hot cache for proxy lookups
	mu          sync.RWMutex
	upstreams   map[string]*models.Upstream // key: slug
	upstreamIDs map[string]*models.Upstream // key: id
	activeRules map[string][]models.FaultRule // key: upstream_id
	killSwitch  models.KillSwitchState
}

// NewSQLStore creates a new SQL store, runs schema migrations, and primes the hot-cache.
func NewSQLStore(driverName, dataSourceName string) (*SQLStore, error) {
	if driverName != "postgres" && driverName != "sqlite" {
		return nil, fmt.Errorf("%w: %s (must be postgres or sqlite)", ErrInvalidDriver, driverName)
	}

	db, err := sql.Open(driverName, dataSourceName)
	if err != nil {
		return nil, fmt.Errorf("failed to open database: %w", err)
	}

	// Connection pool tuning
	if driverName == "sqlite" {
		db.SetMaxOpenConns(1) // SQLite single-writer safety
	} else {
		db.SetMaxOpenConns(25)
		db.SetMaxIdleConns(10)
		db.SetConnMaxLifetime(5 * time.Minute)
	}

	if err := db.Ping(); err != nil {
		return nil, fmt.Errorf("failed to ping database: %w", err)
	}

	store := &SQLStore{
		db:          db,
		driver:      driverName,
		upstreams:   make(map[string]*models.Upstream),
		upstreamIDs: make(map[string]*models.Upstream),
		activeRules: make(map[string][]models.FaultRule),
	}

	if err := store.migrate(); err != nil {
		return nil, fmt.Errorf("schema migration failed: %w", err)
	}

	// Warm the in-memory cache
	if err := store.ReloadCache(context.Background()); err != nil {
		return nil, fmt.Errorf("failed to warm proxy cache: %w", err)
	}

	return store, nil
}

func (s *SQLStore) Close() error {
	return s.db.Close()
}

func (s *SQLStore) migrate() error {
	boolType := "BOOLEAN"
	textType := "TEXT"
	timeType := "TIMESTAMP"
	if s.driver == "sqlite" {
		boolType = "INTEGER"
		timeType = "DATETIME"
	}

	schema := fmt.Sprintf(`
	CREATE TABLE IF NOT EXISTS upstreams (
		id TEXT PRIMARY KEY,
		name TEXT NOT NULL,
		slug TEXT NOT NULL UNIQUE,
		target_url TEXT NOT NULL,
		allow_localhost %s NOT NULL DEFAULT 0,
		allow_private_network %s NOT NULL DEFAULT 0,
		timeout_seconds INTEGER NOT NULL DEFAULT 30,
		max_idle_conns INTEGER NOT NULL DEFAULT 100,
		idle_conn_timeout_seconds INTEGER NOT NULL DEFAULT 90,
		response_header_timeout_sec INTEGER NOT NULL DEFAULT 30,
		health_check_path TEXT NOT NULL DEFAULT '/healthz',
		health_check_interval_seconds INTEGER NOT NULL DEFAULT 10,
		is_active %s NOT NULL DEFAULT 1,
		created_at %s NOT NULL,
		updated_at %s NOT NULL
	);

	CREATE TABLE IF NOT EXISTS experiments (
		id TEXT PRIMARY KEY,
		upstream_id TEXT NOT NULL REFERENCES upstreams(id) ON DELETE CASCADE,
		name TEXT NOT NULL,
		description TEXT NOT NULL DEFAULT '',
		status TEXT NOT NULL DEFAULT 'draft',
		schedule_start %s,
		schedule_end %s,
		max_duration_seconds INTEGER NOT NULL DEFAULT 3600,
		started_at %s,
		auto_stop_at %s,
		created_by TEXT NOT NULL DEFAULT 'admin',
		created_at %s NOT NULL,
		updated_at %s NOT NULL
	);

	CREATE TABLE IF NOT EXISTS fault_rules (
		id TEXT PRIMARY KEY,
		experiment_id TEXT NOT NULL REFERENCES experiments(id) ON DELETE CASCADE,
		name TEXT NOT NULL,
		fault_type TEXT NOT NULL,
		probability REAL NOT NULL DEFAULT 1.0,
		priority INTEGER NOT NULL DEFAULT 0,
		match_methods TEXT NOT NULL DEFAULT '',
		match_path_pattern TEXT NOT NULL DEFAULT '',
		match_headers TEXT NOT NULL DEFAULT '{}',
		match_query_params TEXT NOT NULL DEFAULT '{}',
		match_client_ips TEXT NOT NULL DEFAULT '',
		config TEXT NOT NULL DEFAULT '{}',
		is_enabled %s NOT NULL DEFAULT 1,
		created_at %s NOT NULL,
		updated_at %s NOT NULL
	);

	CREATE TABLE IF NOT EXISTS kill_switch (
		id INTEGER PRIMARY KEY CHECK (id = 1),
		global_active %s NOT NULL DEFAULT 0,
		activated_at %s,
		activated_by TEXT NOT NULL DEFAULT '',
		reason TEXT NOT NULL DEFAULT ''
	);

	CREATE TABLE IF NOT EXISTS audit_logs (
		id TEXT PRIMARY KEY,
		user_id TEXT NOT NULL,
		action TEXT NOT NULL,
		resource_type TEXT NOT NULL,
		resource_id TEXT NOT NULL,
		details_json %s NOT NULL DEFAULT '{}',
		client_ip TEXT NOT NULL DEFAULT '',
		timestamp %s NOT NULL
	);

	CREATE TABLE IF NOT EXISTS users (
		id TEXT PRIMARY KEY,
		username TEXT NOT NULL UNIQUE,
		email TEXT NOT NULL UNIQUE,
		password_hash TEXT NOT NULL,
		role TEXT NOT NULL DEFAULT 'viewer',
		is_active %s NOT NULL DEFAULT 1,
		created_at %s NOT NULL,
		updated_at %s NOT NULL
	);

	CREATE TABLE IF NOT EXISTS api_keys (
		id TEXT PRIMARY KEY,
		name TEXT NOT NULL,
		key_hash TEXT NOT NULL UNIQUE,
		key_prefix TEXT NOT NULL,
		role TEXT NOT NULL DEFAULT 'viewer',
		expires_at %s,
		last_used_at %s,
		is_active %s NOT NULL DEFAULT 1,
		created_at %s NOT NULL
	);

	INSERT INTO kill_switch (id, global_active, activated_at, activated_by, reason)
	SELECT 1, 0, NULL, '', ''
	WHERE NOT EXISTS (SELECT 1 FROM kill_switch WHERE id = 1);
	`,
		boolType, boolType, boolType, timeType, timeType,
		timeType, timeType, timeType, timeType, timeType, timeType,
		boolType, timeType, timeType,
		boolType, timeType,
		textType, timeType,
		boolType, timeType, timeType,
		timeType, timeType, boolType, timeType,
	)

	_, err := s.db.Exec(schema)
	return err
}

// ReloadCache refreshes the in-memory cache for fast, lock-free proxy matching.
func (s *SQLStore) ReloadCache(ctx context.Context) error {
	upstreams, err := s.ListUpstreams(ctx)
	if err != nil {
		return err
	}

	newBySlug := make(map[string]*models.Upstream, len(upstreams))
	newByID := make(map[string]*models.Upstream, len(upstreams))
	for i := range upstreams {
		u := upstreams[i]
		newBySlug[u.Slug] = &u
		newByID[u.ID] = &u
	}

	// Fetch active rules grouped by upstream
	newActiveRules := make(map[string][]models.FaultRule)
	for _, u := range upstreams {
		rules, err := s.queryActiveRules(ctx, u.ID)
		if err != nil {
			return err
		}
		newActiveRules[u.ID] = rules
	}

	ks, err := s.GetKillSwitch(ctx)
	if err != nil {
		return err
	}

	s.mu.Lock()
	s.upstreams = newBySlug
	s.upstreamIDs = newByID
	s.activeRules = newActiveRules
	if ks != nil {
		s.killSwitch = *ks
	}
	s.mu.Unlock()

	return nil
}

// CachedUpstreamBySlug retrieves an upstream from memory with O(1) complexity.
func (s *SQLStore) CachedUpstreamBySlug(slug string) (*models.Upstream, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	u, ok := s.upstreams[slug]
	return u, ok
}

// CachedUpstreamByID retrieves an upstream by ID from memory.
func (s *SQLStore) CachedUpstreamByID(id string) (*models.Upstream, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	u, ok := s.upstreamIDs[id]
	return u, ok
}

// CachedActiveRules retrieves active rules for an upstream from memory.
func (s *SQLStore) CachedActiveRules(upstreamID string) []models.FaultRule {
	s.mu.RLock()
	defer s.mu.RUnlock()
	rules, ok := s.activeRules[upstreamID]
	if !ok {
		return nil
	}
	// Return slice copy for thread-safety
	copied := make([]models.FaultRule, len(rules))
	copy(copied, rules)
	return copied
}

// CachedKillSwitch returns the instantaneous kill switch state.
func (s *SQLStore) CachedKillSwitch() models.KillSwitchState {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.killSwitch
}

// --- Upstream CRUD ---

func (s *SQLStore) CreateUpstream(ctx context.Context, u *models.Upstream) error {
	now := time.Now().UTC()
	u.CreatedAt = now
	u.UpdatedAt = now

	query := `
	INSERT INTO upstreams (
		id, name, slug, target_url, allow_localhost, allow_private_network,
		timeout_seconds, max_idle_conns, idle_conn_timeout_seconds, response_header_timeout_sec,
		health_check_path, health_check_interval_seconds, is_active, created_at, updated_at
	) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15)`

	if s.driver == "sqlite" {
		query = adaptPlaceholders(query)
	}

	_, err := s.db.ExecContext(ctx, query,
		u.ID, u.Name, u.Slug, u.TargetURL, u.AllowLocalhost, u.AllowPrivateNetwork,
		u.TimeoutSeconds, u.MaxIdleConns, u.IdleConnTimeoutSeconds, u.ResponseHeaderTimeoutSec,
		u.HealthCheckPath, u.HealthCheckIntervalSeconds, u.IsActive, u.CreatedAt, u.UpdatedAt,
	)
	if err != nil {
		if strings.Contains(err.Error(), "UNIQUE") || strings.Contains(err.Error(), "duplicate") {
			return ErrConflict
		}
		return err
	}

	return s.ReloadCache(ctx)
}

func (s *SQLStore) GetUpstreamByID(ctx context.Context, id string) (*models.Upstream, error) {
	query := `
	SELECT id, name, slug, target_url, allow_localhost, allow_private_network,
		timeout_seconds, max_idle_conns, idle_conn_timeout_seconds, response_header_timeout_sec,
		health_check_path, health_check_interval_seconds, is_active, created_at, updated_at
	FROM upstreams WHERE id = $1`

	if s.driver == "sqlite" {
		query = adaptPlaceholders(query)
	}

	row := s.db.QueryRowContext(ctx, query, id)
	return scanUpstream(row)
}

func (s *SQLStore) GetUpstreamBySlug(ctx context.Context, slug string) (*models.Upstream, error) {
	query := `
	SELECT id, name, slug, target_url, allow_localhost, allow_private_network,
		timeout_seconds, max_idle_conns, idle_conn_timeout_seconds, response_header_timeout_sec,
		health_check_path, health_check_interval_seconds, is_active, created_at, updated_at
	FROM upstreams WHERE slug = $1`

	if s.driver == "sqlite" {
		query = adaptPlaceholders(query)
	}

	row := s.db.QueryRowContext(ctx, query, slug)
	return scanUpstream(row)
}

func (s *SQLStore) ListUpstreams(ctx context.Context) ([]models.Upstream, error) {
	query := `
	SELECT id, name, slug, target_url, allow_localhost, allow_private_network,
		timeout_seconds, max_idle_conns, idle_conn_timeout_seconds, response_header_timeout_sec,
		health_check_path, health_check_interval_seconds, is_active, created_at, updated_at
	FROM upstreams ORDER BY created_at DESC`

	rows, err := s.db.QueryContext(ctx, query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var upstreams []models.Upstream
	for rows.Next() {
		u, err := scanUpstream(rows)
		if err != nil {
			return nil, err
		}
		upstreams = append(upstreams, *u)
	}
	return upstreams, rows.Err()
}

func (s *SQLStore) UpdateUpstream(ctx context.Context, u *models.Upstream) error {
	u.UpdatedAt = time.Now().UTC()
	query := `
	UPDATE upstreams SET
		name = $1, slug = $2, target_url = $3, allow_localhost = $4, allow_private_network = $5,
		timeout_seconds = $6, max_idle_conns = $7, idle_conn_timeout_seconds = $8,
		response_header_timeout_sec = $9, health_check_path = $10,
		health_check_interval_seconds = $11, is_active = $12, updated_at = $13
	WHERE id = $14`

	if s.driver == "sqlite" {
		query = adaptPlaceholders(query)
	}

	res, err := s.db.ExecContext(ctx, query,
		u.Name, u.Slug, u.TargetURL, u.AllowLocalhost, u.AllowPrivateNetwork,
		u.TimeoutSeconds, u.MaxIdleConns, u.IdleConnTimeoutSeconds, u.ResponseHeaderTimeoutSec,
		u.HealthCheckPath, u.HealthCheckIntervalSeconds, u.IsActive, u.UpdatedAt,
		u.ID,
	)
	if err != nil {
		return err
	}
	rows, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if rows == 0 {
		return ErrNotFound
	}
	return s.ReloadCache(ctx)
}

func (s *SQLStore) DeleteUpstream(ctx context.Context, id string) error {
	query := `DELETE FROM upstreams WHERE id = $1`
	if s.driver == "sqlite" {
		query = adaptPlaceholders(query)
	}
	res, err := s.db.ExecContext(ctx, query, id)
	if err != nil {
		return err
	}
	rows, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if rows == 0 {
		return ErrNotFound
	}
	return s.ReloadCache(ctx)
}

// --- Experiments CRUD ---

func (s *SQLStore) CreateExperiment(ctx context.Context, exp *models.Experiment) error {
	now := time.Now().UTC()
	exp.CreatedAt = now
	exp.UpdatedAt = now

	query := `
	INSERT INTO experiments (
		id, upstream_id, name, description, status, schedule_start, schedule_end,
		max_duration_seconds, started_at, auto_stop_at, created_by, created_at, updated_at
	) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13)`

	if s.driver == "sqlite" {
		query = adaptPlaceholders(query)
	}

	_, err := s.db.ExecContext(ctx, query,
		exp.ID, exp.UpstreamID, exp.Name, exp.Description, exp.Status,
		exp.ScheduleStart, exp.ScheduleEnd, exp.MaxDurationSeconds,
		exp.StartedAt, exp.AutoStopAt, exp.CreatedBy, exp.CreatedAt, exp.UpdatedAt,
	)
	if err != nil {
		return err
	}

	// Insert rules if provided
	for i := range exp.Rules {
		r := &exp.Rules[i]
		r.ExperimentID = exp.ID
		if err := s.CreateFaultRule(ctx, r); err != nil {
			return err
		}
	}

	return s.ReloadCache(ctx)
}

func (s *SQLStore) GetExperimentByID(ctx context.Context, id string) (*models.Experiment, error) {
	query := `
	SELECT id, upstream_id, name, description, status, schedule_start, schedule_end,
		max_duration_seconds, started_at, auto_stop_at, created_by, created_at, updated_at
	FROM experiments WHERE id = $1`

	if s.driver == "sqlite" {
		query = adaptPlaceholders(query)
	}

	row := s.db.QueryRowContext(ctx, query, id)
	exp, err := scanExperiment(row)
	if err != nil {
		return nil, err
	}

	// Fetch rules
	rules, err := s.ListRulesByExperiment(ctx, exp.ID)
	if err != nil {
		return nil, err
	}
	exp.Rules = rules

	return exp, nil
}

func (s *SQLStore) ListExperiments(ctx context.Context, upstreamID string) ([]models.Experiment, error) {
	var query string
	var args []any

	if upstreamID != "" {
		query = `
		SELECT id, upstream_id, name, description, status, schedule_start, schedule_end,
			max_duration_seconds, started_at, auto_stop_at, created_by, created_at, updated_at
		FROM experiments WHERE upstream_id = $1 ORDER BY created_at DESC`
		args = append(args, upstreamID)
	} else {
		query = `
		SELECT id, upstream_id, name, description, status, schedule_start, schedule_end,
			max_duration_seconds, started_at, auto_stop_at, created_by, created_at, updated_at
		FROM experiments ORDER BY created_at DESC`
	}

	if s.driver == "sqlite" {
		query = adaptPlaceholders(query)
	}

	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var exps []models.Experiment
	for rows.Next() {
		exp, err := scanExperiment(rows)
		if err != nil {
			return nil, err
		}
		// Fetch rules count or rules
		rules, _ := s.ListRulesByExperiment(ctx, exp.ID)
		exp.Rules = rules
		exps = append(exps, *exp)
	}
	return exps, rows.Err()
}

func (s *SQLStore) UpdateExperiment(ctx context.Context, exp *models.Experiment) error {
	exp.UpdatedAt = time.Now().UTC()
	query := `
	UPDATE experiments SET
		name = $1, description = $2, status = $3, schedule_start = $4, schedule_end = $5,
		max_duration_seconds = $6, started_at = $7, auto_stop_at = $8, updated_at = $9
	WHERE id = $10`

	if s.driver == "sqlite" {
		query = adaptPlaceholders(query)
	}

	res, err := s.db.ExecContext(ctx, query,
		exp.Name, exp.Description, exp.Status, exp.ScheduleStart, exp.ScheduleEnd,
		exp.MaxDurationSeconds, exp.StartedAt, exp.AutoStopAt, exp.UpdatedAt,
		exp.ID,
	)
	if err != nil {
		return err
	}
	rows, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if rows == 0 {
		return ErrNotFound
	}
	return s.ReloadCache(ctx)
}

func (s *SQLStore) UpdateExperimentStatus(ctx context.Context, id string, status models.ExperimentStatus) error {
	now := time.Now().UTC()
	var startedAt, autoStopAt *time.Time

	if status == models.StatusRunning {
		startedAt = &now
		// Fetch max_duration_seconds
		exp, err := s.GetExperimentByID(ctx, id)
		if err == nil && exp.MaxDurationSeconds > 0 {
			stop := now.Add(time.Duration(exp.MaxDurationSeconds) * time.Second)
			autoStopAt = &stop
		}
	}

	query := `
	UPDATE experiments SET status = $1, started_at = COALESCE($2, started_at),
		auto_stop_at = COALESCE($3, auto_stop_at), updated_at = $4
	WHERE id = $5`

	if s.driver == "sqlite" {
		query = adaptPlaceholders(query)
	}

	res, err := s.db.ExecContext(ctx, query, status, startedAt, autoStopAt, now, id)
	if err != nil {
		return err
	}
	rows, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if rows == 0 {
		return ErrNotFound
	}
	return s.ReloadCache(ctx)
}

func (s *SQLStore) DeleteExperiment(ctx context.Context, id string) error {
	query := `DELETE FROM experiments WHERE id = $1`
	if s.driver == "sqlite" {
		query = adaptPlaceholders(query)
	}
	res, err := s.db.ExecContext(ctx, query, id)
	if err != nil {
		return err
	}
	rows, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if rows == 0 {
		return ErrNotFound
	}
	return s.ReloadCache(ctx)
}

// --- Fault Rules CRUD ---

func (s *SQLStore) CreateFaultRule(ctx context.Context, r *models.FaultRule) error {
	now := time.Now().UTC()
	r.CreatedAt = now
	r.UpdatedAt = now

	methods := strings.Join(r.MatchMethods, ",")
	headersJSON, _ := json.Marshal(r.MatchHeaders)
	queryJSON, _ := json.Marshal(r.MatchQueryParams)
	clientIPs := strings.Join(r.MatchClientIPs, ",")
	configJSON := string(r.Config)
	if configJSON == "" {
		configJSON = "{}"
	}

	query := `
	INSERT INTO fault_rules (
		id, experiment_id, name, fault_type, probability, priority,
		match_methods, match_path_pattern, match_headers, match_query_params,
		match_client_ips, config, is_enabled, created_at, updated_at
	) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15)`

	if s.driver == "sqlite" {
		query = adaptPlaceholders(query)
	}

	_, err := s.db.ExecContext(ctx, query,
		r.ID, r.ExperimentID, r.Name, r.FaultType, r.Probability, r.Priority,
		methods, r.MatchPathPattern, string(headersJSON), string(queryJSON),
		clientIPs, configJSON, r.IsEnabled, r.CreatedAt, r.UpdatedAt,
	)
	if err != nil {
		return err
	}
	return s.ReloadCache(ctx)
}

func (s *SQLStore) GetFaultRuleByID(ctx context.Context, id string) (*models.FaultRule, error) {
	query := `
	SELECT id, experiment_id, name, fault_type, probability, priority,
		match_methods, match_path_pattern, match_headers, match_query_params,
		match_client_ips, config, is_enabled, created_at, updated_at
	FROM fault_rules WHERE id = $1`

	if s.driver == "sqlite" {
		query = adaptPlaceholders(query)
	}

	row := s.db.QueryRowContext(ctx, query, id)
	return scanFaultRule(row)
}

func (s *SQLStore) ListRulesByExperiment(ctx context.Context, experimentID string) ([]models.FaultRule, error) {
	query := `
	SELECT id, experiment_id, name, fault_type, probability, priority,
		match_methods, match_path_pattern, match_headers, match_query_params,
		match_client_ips, config, is_enabled, created_at, updated_at
	FROM fault_rules WHERE experiment_id = $1 ORDER BY priority DESC, created_at ASC`

	if s.driver == "sqlite" {
		query = adaptPlaceholders(query)
	}

	rows, err := s.db.QueryContext(ctx, query, experimentID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var rules []models.FaultRule
	for rows.Next() {
		r, err := scanFaultRule(rows)
		if err != nil {
			return nil, err
		}
		rules = append(rules, *r)
	}
	return rules, rows.Err()
}

func (s *SQLStore) UpdateFaultRule(ctx context.Context, r *models.FaultRule) error {
	r.UpdatedAt = time.Now().UTC()

	methods := strings.Join(r.MatchMethods, ",")
	headersJSON, _ := json.Marshal(r.MatchHeaders)
	queryJSON, _ := json.Marshal(r.MatchQueryParams)
	clientIPs := strings.Join(r.MatchClientIPs, ",")
	configJSON := string(r.Config)
	if configJSON == "" {
		configJSON = "{}"
	}

	query := `
	UPDATE fault_rules SET
		name = $1, fault_type = $2, probability = $3, priority = $4,
		match_methods = $5, match_path_pattern = $6, match_headers = $7,
		match_query_params = $8, match_client_ips = $9, config = $10,
		is_enabled = $11, updated_at = $12
	WHERE id = $13`

	if s.driver == "sqlite" {
		query = adaptPlaceholders(query)
	}

	res, err := s.db.ExecContext(ctx, query,
		r.Name, r.FaultType, r.Probability, r.Priority,
		methods, r.MatchPathPattern, string(headersJSON), string(queryJSON),
		clientIPs, configJSON, r.IsEnabled, r.UpdatedAt,
		r.ID,
	)
	if err != nil {
		return err
	}
	rows, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if rows == 0 {
		return ErrNotFound
	}
	return s.ReloadCache(ctx)
}

func (s *SQLStore) DeleteFaultRule(ctx context.Context, id string) error {
	query := `DELETE FROM fault_rules WHERE id = $1`
	if s.driver == "sqlite" {
		query = adaptPlaceholders(query)
	}
	res, err := s.db.ExecContext(ctx, query, id)
	if err != nil {
		return err
	}
	rows, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if rows == 0 {
		return ErrNotFound
	}
	return s.ReloadCache(ctx)
}

// GetActiveRulesForUpstream queries active rules for an upstream whose experiment is RUNNING.
func (s *SQLStore) GetActiveRulesForUpstream(ctx context.Context, upstreamID string) ([]models.FaultRule, error) {
	return s.queryActiveRules(ctx, upstreamID)
}

func (s *SQLStore) queryActiveRules(ctx context.Context, upstreamID string) ([]models.FaultRule, error) {
	query := `
	SELECT r.id, r.experiment_id, r.name, r.fault_type, r.probability, r.priority,
		r.match_methods, r.match_path_pattern, r.match_headers, r.match_query_params,
		r.match_client_ips, r.config, r.is_enabled, r.created_at, r.updated_at
	FROM fault_rules r
	JOIN experiments e ON r.experiment_id = e.id
	WHERE e.upstream_id = $1 AND e.status = 'running' AND r.is_enabled = true
	ORDER BY r.priority DESC, r.created_at ASC`

	if s.driver == "sqlite" {
		query = strings.ReplaceAll(query, "r.is_enabled = true", "r.is_enabled = 1")
		query = adaptPlaceholders(query)
	}

	rows, err := s.db.QueryContext(ctx, query, upstreamID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var rules []models.FaultRule
	for rows.Next() {
		r, err := scanFaultRule(rows)
		if err != nil {
			return nil, err
		}
		rules = append(rules, *r)
	}
	return rules, rows.Err()
}

// --- Kill Switch ---

func (s *SQLStore) GetKillSwitch(ctx context.Context) (*models.KillSwitchState, error) {
	query := `SELECT global_active, activated_at, activated_by, reason FROM kill_switch WHERE id = 1`
	row := s.db.QueryRowContext(ctx, query)

	var ks models.KillSwitchState
	var actAt sql.NullTime
	err := row.Scan(&ks.GlobalActive, &actAt, &ks.ActivatedBy, &ks.Reason)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return &models.KillSwitchState{GlobalActive: false}, nil
		}
		return nil, err
	}
	if actAt.Valid {
		ks.ActivatedAt = actAt.Time
	}
	return &ks, nil
}

func (s *SQLStore) SetKillSwitch(ctx context.Context, state *models.KillSwitchState) error {
	query := `
	UPDATE kill_switch SET global_active = $1, activated_at = $2, activated_by = $3, reason = $4
	WHERE id = 1`

	if s.driver == "sqlite" {
		query = adaptPlaceholders(query)
	}

	var actAt *time.Time
	if state.GlobalActive {
		now := time.Now().UTC()
		state.ActivatedAt = now
		actAt = &now
	}

	_, err := s.db.ExecContext(ctx, query, state.GlobalActive, actAt, state.ActivatedBy, state.Reason)
	if err != nil {
		return err
	}

	s.mu.Lock()
	s.killSwitch = *state
	s.mu.Unlock()

	return nil
}

// --- Audit Logs ---

func (s *SQLStore) CreateAuditLog(ctx context.Context, log *models.AuditLog) error {
	if log.Timestamp.IsZero() {
		log.Timestamp = time.Now().UTC()
	}

	query := `
	INSERT INTO audit_logs (id, user_id, action, resource_type, resource_id, details_json, client_ip, timestamp)
	VALUES ($1, $2, $3, $4, $5, $6, $7, $8)`

	if s.driver == "sqlite" {
		query = adaptPlaceholders(query)
	}

	_, err := s.db.ExecContext(ctx, query,
		log.ID, log.UserID, log.Action, log.ResourceType, log.ResourceID,
		log.DetailsJSON, log.ClientIP, log.Timestamp,
	)
	return err
}

func (s *SQLStore) ListAuditLogs(ctx context.Context, limit, offset int) ([]models.AuditLog, error) {
	if limit <= 0 {
		limit = 50
	}
	query := `
	SELECT id, user_id, action, resource_type, resource_id, details_json, client_ip, timestamp
	FROM audit_logs ORDER BY timestamp DESC LIMIT $1 OFFSET $2`

	if s.driver == "sqlite" {
		query = adaptPlaceholders(query)
	}

	rows, err := s.db.QueryContext(ctx, query, limit, offset)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var logs []models.AuditLog
	for rows.Next() {
		var l models.AuditLog
		if err := rows.Scan(&l.ID, &l.UserID, &l.Action, &l.ResourceType, &l.ResourceID, &l.DetailsJSON, &l.ClientIP, &l.Timestamp); err != nil {
			return nil, err
		}
		logs = append(logs, l)
	}
	return logs, rows.Err()
}

// --- Users & Auth ---

func (s *SQLStore) CreateUser(ctx context.Context, u *models.User) error {
	now := time.Now().UTC()
	u.CreatedAt = now
	u.UpdatedAt = now

	query := `
	INSERT INTO users (id, username, email, password_hash, role, is_active, created_at, updated_at)
	VALUES ($1, $2, $3, $4, $5, $6, $7, $8)`

	if s.driver == "sqlite" {
		query = adaptPlaceholders(query)
	}

	_, err := s.db.ExecContext(ctx, query,
		u.ID, u.Username, u.Email, u.PasswordHash, u.Role, u.IsActive, u.CreatedAt, u.UpdatedAt,
	)
	if err != nil {
		if strings.Contains(err.Error(), "UNIQUE") || strings.Contains(err.Error(), "duplicate") {
			return ErrConflict
		}
		return err
	}
	return nil
}

func (s *SQLStore) GetUserByUsername(ctx context.Context, username string) (*models.User, error) {
	query := `SELECT id, username, email, password_hash, role, is_active, created_at, updated_at FROM users WHERE username = $1`
	if s.driver == "sqlite" {
		query = adaptPlaceholders(query)
	}
	row := s.db.QueryRowContext(ctx, query, username)
	return scanUser(row)
}

func (s *SQLStore) GetUserByID(ctx context.Context, id string) (*models.User, error) {
	query := `SELECT id, username, email, password_hash, role, is_active, created_at, updated_at FROM users WHERE id = $1`
	if s.driver == "sqlite" {
		query = adaptPlaceholders(query)
	}
	row := s.db.QueryRowContext(ctx, query, id)
	return scanUser(row)
}

// --- API Keys ---

func (s *SQLStore) CreateAPIKey(ctx context.Context, k *models.APIKey) error {
	if k.CreatedAt.IsZero() {
		k.CreatedAt = time.Now().UTC()
	}

	query := `
	INSERT INTO api_keys (id, name, key_hash, key_prefix, role, expires_at, last_used_at, is_active, created_at)
	VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)`

	if s.driver == "sqlite" {
		query = adaptPlaceholders(query)
	}

	_, err := s.db.ExecContext(ctx, query,
		k.ID, k.Name, k.KeyHash, k.KeyPrefix, k.Role, k.ExpiresAt, k.LastUsedAt, k.IsActive, k.CreatedAt,
	)
	if err != nil {
		if strings.Contains(err.Error(), "UNIQUE") || strings.Contains(err.Error(), "duplicate") {
			return ErrConflict
		}
		return err
	}
	return nil
}

func (s *SQLStore) GetAPIKeyByHash(ctx context.Context, keyHash string) (*models.APIKey, error) {
	query := `
	SELECT id, name, key_hash, key_prefix, role, expires_at, last_used_at, is_active, created_at
	FROM api_keys WHERE key_hash = $1`

	if s.driver == "sqlite" {
		query = adaptPlaceholders(query)
	}

	row := s.db.QueryRowContext(ctx, query, keyHash)
	return scanAPIKey(row)
}

func (s *SQLStore) ListAPIKeys(ctx context.Context) ([]models.APIKey, error) {
	query := `
	SELECT id, name, key_hash, key_prefix, role, expires_at, last_used_at, is_active, created_at
	FROM api_keys ORDER BY created_at DESC`

	rows, err := s.db.QueryContext(ctx, query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var keys []models.APIKey
	for rows.Next() {
		k, err := scanAPIKey(rows)
		if err != nil {
			return nil, err
		}
		keys = append(keys, *k)
	}
	return keys, rows.Err()
}

func (s *SQLStore) RevokeAPIKey(ctx context.Context, id string) error {
	query := `UPDATE api_keys SET is_active = $1 WHERE id = $2`
	val := any(false)
	if s.driver == "sqlite" {
		query = adaptPlaceholders(query)
		val = 0
	}
	res, err := s.db.ExecContext(ctx, query, val, id)
	if err != nil {
		return err
	}
	rows, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if rows == 0 {
		return ErrNotFound
	}
	return nil
}

func (s *SQLStore) UpdateAPIKeyLastUsed(ctx context.Context, id string, t time.Time) error {
	query := `UPDATE api_keys SET last_used_at = $2 WHERE id = $1`
	if s.driver == "sqlite" {
		query = adaptPlaceholders(query)
	}
	_, err := s.db.ExecContext(ctx, query, id, t)
	return err
}

// --- Helpers & Scanners ---

type scannable interface {
	Scan(dest ...any) error
}

func scanUpstream(s scannable) (*models.Upstream, error) {
	var u models.Upstream
	err := s.Scan(
		&u.ID, &u.Name, &u.Slug, &u.TargetURL, &u.AllowLocalhost, &u.AllowPrivateNetwork,
		&u.TimeoutSeconds, &u.MaxIdleConns, &u.IdleConnTimeoutSeconds, &u.ResponseHeaderTimeoutSec,
		&u.HealthCheckPath, &u.HealthCheckIntervalSeconds, &u.IsActive, &u.CreatedAt, &u.UpdatedAt,
	)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return &u, nil
}

func scanExperiment(s scannable) (*models.Experiment, error) {
	var exp models.Experiment
	var schedStart, schedEnd, startedAt, autoStopAt sql.NullTime

	err := s.Scan(
		&exp.ID, &exp.UpstreamID, &exp.Name, &exp.Description, &exp.Status,
		&schedStart, &schedEnd, &exp.MaxDurationSeconds, &startedAt, &autoStopAt,
		&exp.CreatedBy, &exp.CreatedAt, &exp.UpdatedAt,
	)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}

	if schedStart.Valid {
		exp.ScheduleStart = &schedStart.Time
	}
	if schedEnd.Valid {
		exp.ScheduleEnd = &schedEnd.Time
	}
	if startedAt.Valid {
		exp.StartedAt = &startedAt.Time
	}
	if autoStopAt.Valid {
		exp.AutoStopAt = &autoStopAt.Time
	}

	return &exp, nil
}

func scanFaultRule(s scannable) (*models.FaultRule, error) {
	var r models.FaultRule
	var methods, headersStr, queryStr, clientIPs, configStr string

	err := s.Scan(
		&r.ID, &r.ExperimentID, &r.Name, &r.FaultType, &r.Probability, &r.Priority,
		&methods, &r.MatchPathPattern, &headersStr, &queryStr,
		&clientIPs, &configStr, &r.IsEnabled, &r.CreatedAt, &r.UpdatedAt,
	)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}

	if methods != "" {
		r.MatchMethods = strings.Split(methods, ",")
	}
	if clientIPs != "" {
		r.MatchClientIPs = strings.Split(clientIPs, ",")
	}
	if headersStr != "" {
		_ = json.Unmarshal([]byte(headersStr), &r.MatchHeaders)
	}
	if queryStr != "" {
		_ = json.Unmarshal([]byte(queryStr), &r.MatchQueryParams)
	}
	r.Config = json.RawMessage(configStr)

	return &r, nil
}

func scanUser(s scannable) (*models.User, error) {
	var u models.User
	err := s.Scan(&u.ID, &u.Username, &u.Email, &u.PasswordHash, &u.Role, &u.IsActive, &u.CreatedAt, &u.UpdatedAt)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return &u, nil
}

func scanAPIKey(s scannable) (*models.APIKey, error) {
	var k models.APIKey
	var expAt, lastUsed sql.NullTime
	err := s.Scan(&k.ID, &k.Name, &k.KeyHash, &k.KeyPrefix, &k.Role, &expAt, &lastUsed, &k.IsActive, &k.CreatedAt)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	if expAt.Valid {
		k.ExpiresAt = &expAt.Time
	}
	if lastUsed.Valid {
		k.LastUsedAt = &lastUsed.Time
	}
	return &k, nil
}

// adaptPlaceholders converts PostgreSQL $1, $2 to SQLite ? placeholders.
func adaptPlaceholders(query string) string {
	var result strings.Builder
	for i := 0; i < len(query); i++ {
		if query[i] == '$' && i+1 < len(query) && query[i+1] >= '0' && query[i+1] <= '9' {
			result.WriteByte('?')
			// Skip digits
			for i+1 < len(query) && query[i+1] >= '0' && query[i+1] <= '9' {
				i++
			}
		} else {
			result.WriteByte(query[i])
		}
	}
	return result.String()
}
