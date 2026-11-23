export type Role = 'admin' | 'operator' | 'viewer';

export type FaultType =
  | 'latency'
  | 'http_error'
  | 'connection_drop'
  | 'timeout'
  | 'bandwidth_limit'
  | 'packet_loss'
  | 'burst';

export type ExperimentStatus = 'draft' | 'scheduled' | 'running' | 'paused' | 'completed' | 'aborted';

export interface UpstreamHealth {
  upstream_id: string;
  healthy: boolean;
  last_check_time: string;
  consecutive_failures: number;
  latency_ms: number;
  last_http_status: number;
  last_error_message?: string;
}

export interface Upstream {
  id: string;
  name: string;
  slug: string;
  target_url: string;
  allow_localhost: boolean;
  allow_private_network: boolean;
  timeout_seconds: number;
  max_idle_conns: number;
  idle_conn_timeout_seconds: number;
  response_header_timeout_sec: number;
  health_check_path: string;
  health_check_interval_seconds: number;
  is_active: boolean;
  created_at: string;
  updated_at: string;
  health?: UpstreamHealth;
}

export interface FaultRule {
  id: string;
  experiment_id: string;
  name: string;
  fault_type: FaultType;
  probability: number;
  priority: number;
  match_methods?: string[];
  match_path_pattern?: string;
  match_headers?: Record<string, string>;
  match_query_params?: Record<string, string>;
  match_client_ips?: string[];
  config: any;
  is_enabled: boolean;
  created_at: string;
  updated_at: string;
}

export interface Experiment {
  id: string;
  upstream_id: string;
  name: string;
  description: string;
  status: ExperimentStatus;
  schedule_start?: string;
  schedule_end?: string;
  max_duration_seconds: number;
  started_at?: string;
  auto_stop_at?: string;
  created_by: string;
  rules?: FaultRule[];
  created_at: string;
  updated_at: string;
}

export interface KillSwitchState {
  global_active: boolean;
  activated_at?: string;
  activated_by: string;
  reason: string;
}

export interface AuditLog {
  id: string;
  user_id: string;
  action: string;
  resource_type: string;
  resource_id: string;
  details_json: string;
  client_ip: string;
  timestamp: string;
}

export interface APIKey {
  id: string;
  name: string;
  key_prefix: string;
  role: Role;
  expires_at?: string;
  last_used_at?: string;
  is_active: boolean;
  created_at: string;
}

export interface LiveStats {
  requests_per_sec: number;
  error_rate_percent: number;
  p50_latency_ms: number;
  p95_latency_ms: number;
  p99_latency_ms: number;
  total_requests: number;
  total_faults: number;
  fault_breakdown: Record<string, number>;
  recent_history: {
    time_iso: string;
    request_count: number;
    injected_count: number;
    avg_latency_ms: number;
  }[];
}

export interface TestStudioResult {
  status_code: number;
  status_text: string;
  duration_ms: number;
  injected_fault?: string;
  is_injected: boolean;
  response_body: string;
  response_headers: Record<string, string>;
}
