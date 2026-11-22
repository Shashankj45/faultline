package models

import (
	"encoding/json"
	"time"
)

// Role defines RBAC access roles.
type Role string

const (
	RoleAdmin    Role = "admin"
	RoleOperator Role = "operator"
	RoleViewer   Role = "viewer"
)

// FaultType identifies the chaos injection mechanism.
type FaultType string

const (
	FaultLatency        FaultType = "latency"
	FaultHTTPError      FaultType = "http_error"
	FaultConnectionDrop FaultType = "connection_drop"
	FaultTimeout        FaultType = "timeout"
	FaultBandwidthLimit FaultType = "bandwidth_limit"
	FaultPacketLoss     FaultType = "packet_loss"
	FaultBurst          FaultType = "burst"
)

// ExperimentStatus tracks experiment lifecycle states.
type ExperimentStatus string

const (
	StatusDraft     ExperimentStatus = "draft"
	StatusScheduled ExperimentStatus = "scheduled"
	StatusRunning   ExperimentStatus = "running"
	StatusPaused    ExperimentStatus = "paused"
	StatusCompleted ExperimentStatus = "completed"
	StatusAborted   ExperimentStatus = "aborted"
)

// Upstream defines a registered, authorized backend service.
type Upstream struct {
	ID                         string    `json:"id"`
	Name                       string    `json:"name"`
	Slug                       string    `json:"slug"`
	TargetURL                  string    `json:"target_url"`
	AllowLocalhost             bool      `json:"allow_localhost"`
	AllowPrivateNetwork        bool      `json:"allow_private_network"`
	TimeoutSeconds             int       `json:"timeout_seconds"`
	MaxIdleConns               int       `json:"max_idle_conns"`
	IdleConnTimeoutSeconds     int       `json:"idle_conn_timeout_seconds"`
	ResponseHeaderTimeoutSec   int       `json:"response_header_timeout_sec"`
	HealthCheckPath            string    `json:"health_check_path"`
	HealthCheckIntervalSeconds int       `json:"health_check_interval_seconds"`
	IsActive                   bool      `json:"is_active"`
	CreatedAt                  time.Time `json:"created_at"`
	UpdatedAt                  time.Time `json:"updated_at"`
}

// LatencyDistribution models latency randomness.
type LatencyDistribution string

const (
	DistFixed    LatencyDistribution = "fixed"
	DistUniform  LatencyDistribution = "uniform"
	DistGaussian LatencyDistribution = "gaussian"
	DistRamp     LatencyDistribution = "ramp"
)

// LatencyConfig defines artificial latency parameters.
type LatencyConfig struct {
	Distribution        LatencyDistribution `json:"distribution"`
	FixedLatencyMs      int                 `json:"fixed_latency_ms,omitempty"`
	JitterMinMs         int                 `json:"jitter_min_ms,omitempty"`
	JitterMaxMs         int                 `json:"jitter_max_ms,omitempty"`
	MeanMs              float64             `json:"mean_ms,omitempty"`
	StdDevMs            float64             `json:"std_dev_ms,omitempty"`
	RampStartMs         int                 `json:"ramp_start_ms,omitempty"`
	RampEndMs           int                 `json:"ramp_end_ms,omitempty"`
	RampDurationSeconds int                 `json:"ramp_duration_seconds,omitempty"`
}

// HTTPErrorConfig defines injected HTTP response errors.
type HTTPErrorConfig struct {
	StatusCode   int               `json:"status_code"`
	ResponseBody string            `json:"response_body"`
	Headers      map[string]string `json:"headers,omitempty"`
	PreUpstream  bool              `json:"pre_upstream"` // true = short-circuit before calling upstream
}

// DropType defines how connection drops occur.
type DropType string

const (
	DropTCPReset       DropType = "tcp_reset"       // Abrupt TCP RST (via SetLinger(0))
	DropEmptyEOF       DropType = "empty_eof"       // Immediate silent close (0 bytes)
	DropPartialBody    DropType = "partial_body"    // Send partial body then abruptly close
	DropMalformedBytes DropType = "malformed_bytes" // Send garbage HTTP framing bytes
)

// ConnectionDropConfig configures network connection teardowns.
type ConnectionDropConfig struct {
	DropType         DropType `json:"drop_type"`
	PartialByteCount int      `json:"partial_byte_count,omitempty"`
}

// TimeoutType defines timeout behaviors.
type TimeoutType string

const (
	TimeoutClientStall    TimeoutType = "client_stall"    // Hold until client context cancels
	TimeoutGatewayTimeout TimeoutType = "gateway_timeout" // Sleep duration then return 504
	TimeoutSlowRead       TimeoutType = "slow_read"       // Read request body at throttled bps
	TimeoutSlowWrite      TimeoutType = "slow_write"      // Drip response byte by byte
)

// TimeoutConfig configures slow-burn timeout scenarios.
type TimeoutConfig struct {
	TimeoutType    TimeoutType `json:"timeout_type"`
	DelayMs        int         `json:"delay_ms,omitempty"`
	ChunkIntervalMs int        `json:"chunk_interval_ms,omitempty"`
	ChunkSizeBytes  int        `json:"chunk_size_bytes,omitempty"`
}

// BandwidthDirection specifies whether throttling is ingress or egress.
type BandwidthDirection string

const (
	BandwidthUpstream   BandwidthDirection = "upstream"   // Throttle request upload
	BandwidthDownstream BandwidthDirection = "downstream" // Throttle response download
)

// BandwidthConfig configures rate limiting on streams.
type BandwidthConfig struct {
	Direction        BandwidthDirection `json:"direction"`
	RateBytesPerSec  int64              `json:"rate_bytes_per_sec"`
	BufferBurstBytes int64              `json:"buffer_burst_bytes"`
}

// PacketLossConfig simulates dropped chunks or periodic resets.
type PacketLossConfig struct {
	DropChunkPercent float64 `json:"drop_chunk_percent"` // 0.0 - 1.0 probability of dropping a chunk
	CorruptBytes     bool    `json:"corrupt_bytes"`       // Inject random byte mutations
}

// BurstConfig configures burst failure spikes.
type BurstConfig struct {
	BurstDurationMs  int         `json:"burst_duration_ms"`  // How long the failure spike lasts
	BurstIntervalMs  int         `json:"burst_interval_ms"`  // Cycle period between bursts
	BurstFaultType   FaultType   `json:"burst_fault_type"`   // Fault to inject during burst
	BurstStatusCode  int         `json:"burst_status_code,omitempty"`
	BurstLatencyMs   int         `json:"burst_latency_ms,omitempty"`
}

// FaultRule represents an actionable chaos rule.
type FaultRule struct {
	ID                string            `json:"id"`
	ExperimentID      string            `json:"experiment_id"`
	Name              string            `json:"name"`
	FaultType         FaultType         `json:"fault_type"`
	Probability       float64           `json:"probability"` // 0.0 to 1.0 (e.g. 0.25 = 25%)
	Priority          int               `json:"priority"`
	MatchMethods      []string          `json:"match_methods,omitempty"`
	MatchPathPattern  string            `json:"match_path_pattern,omitempty"` // Regex or prefix
	MatchHeaders      map[string]string `json:"match_headers,omitempty"`      // Key: header, Value: regex
	MatchQueryParams  map[string]string `json:"match_query_params,omitempty"`  // Key: param, Value: regex
	MatchClientIPs    []string          `json:"match_client_ips,omitempty"`    // CIDRs or exact IPs
	Config            json.RawMessage   `json:"config"`                       // Serialized specific config
	IsEnabled         bool              `json:"is_enabled"`
	CreatedAt         time.Time         `json:"created_at"`
	UpdatedAt         time.Time         `json:"updated_at"`
}

// Experiment encapsulates a collection of chaos rules targeting an upstream.
type Experiment struct {
	ID                 string           `json:"id"`
	UpstreamID         string           `json:"upstream_id"`
	Name               string           `json:"name"`
	Description        string           `json:"description"`
	Status             ExperimentStatus `json:"status"`
	ScheduleStart      *time.Time       `json:"schedule_start,omitempty"`
	ScheduleEnd        *time.Time       `json:"schedule_end,omitempty"`
	MaxDurationSeconds int              `json:"max_duration_seconds"` // Safety auto-abort TTL
	StartedAt          *time.Time       `json:"started_at,omitempty"`
	AutoStopAt         *time.Time       `json:"auto_stop_at,omitempty"`
	CreatedBy          string           `json:"created_by"`
	Rules              []FaultRule      `json:"rules,omitempty"`
	CreatedAt          time.Time        `json:"created_at"`
	UpdatedAt          time.Time        `json:"updated_at"`
}

// KillSwitchState tracks emergency killswitch status.
type KillSwitchState struct {
	GlobalActive bool      `json:"global_active"`
	ActivatedAt  time.Time `json:"activated_at"`
	ActivatedBy  string    `json:"activated_by"`
	Reason       string    `json:"reason"`
}

// AuditLog records tamper-evident administrative events.
type AuditLog struct {
	ID           string    `json:"id"`
	UserID       string    `json:"user_id"`
	Action       string    `json:"action"`
	ResourceType string    `json:"resource_type"`
	ResourceID   string    `json:"resource_id"`
	DetailsJSON  string    `json:"details_json"`
	ClientIP     string    `json:"client_ip"`
	Timestamp    time.Time `json:"timestamp"`
}

// User defines an authenticated platform operator.
type User struct {
	ID           string    `json:"id"`
	Username     string    `json:"username"`
	Email        string    `json:"email"`
	PasswordHash string    `json:"-"`
	Role         Role      `json:"role"`
	IsActive     bool      `json:"is_active"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
}

// APIKey allows programmatic authentication.
type APIKey struct {
	ID        string     `json:"id"`
	Name      string     `json:"name"`
	KeyHash   string     `json:"-"`
	KeyPrefix string     `json:"key_prefix"`
	Role      Role       `json:"role"`
	ExpiresAt *time.Time `json:"expires_at,omitempty"`
	LastUsedAt *time.Time `json:"last_used_at,omitempty"`
	IsActive  bool       `json:"is_active"`
	CreatedAt time.Time  `json:"created_at"`
}

// UpstreamHealth represents live probe telemetry.
type UpstreamHealth struct {
	UpstreamID          string    `json:"upstream_id"`
	Healthy             bool      `json:"healthy"`
	LastCheckTime       time.Time `json:"last_check_time"`
	ConsecutiveFailures int       `json:"consecutive_failures"`
	LatencyMs           int64     `json:"latency_ms"`
	LastHTTPStatus      int       `json:"last_http_status"`
	LastErrorMessage    string    `json:"last_error_message,omitempty"`
}
