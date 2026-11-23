import {
  Upstream,
  Experiment,
  FaultRule,
  KillSwitchState,
  AuditLog,
  LiveStats,
  TestStudioResult,
  ExperimentStatus,
  APIKey,
  Role,
} from './types';

const BASE_URL = '';
const API_KEY_STORAGE_KEY = 'faultline_session_api_key';

export function getActiveApiKey(): string {
  try {
    return localStorage.getItem(API_KEY_STORAGE_KEY) || '';
  } catch {
    return '';
  }
}

export function setActiveApiKey(key: string): void {
  try {
    if (!key) {
      localStorage.removeItem(API_KEY_STORAGE_KEY);
    } else {
      localStorage.setItem(API_KEY_STORAGE_KEY, key.trim());
    }
  } catch {
    // Ignore storage errors in restricted contexts
  }
}

async function apiRequest<T>(endpoint: string, options: RequestInit = {}): Promise<T> {
  const headers = new Headers(options.headers || {});
  if (!headers.has('Content-Type') && options.body && typeof options.body === 'string') {
    headers.set('Content-Type', 'application/json');
  }

  // Attach session API Key if configured
  const sessionKey = getActiveApiKey();
  if (sessionKey && !headers.has('X-API-Key') && !headers.has('Authorization')) {
    headers.set('X-API-Key', sessionKey);
  }

  const res = await fetch(`${BASE_URL}${endpoint}`, {
    ...options,
    headers,
  });

  if (!res.ok) {
    let errorMsg = `HTTP ${res.status}: ${res.statusText}`;
    try {
      const errJson = await res.json();
      if (errJson.error) errorMsg = errJson.error;
    } catch {
      // Keep default
    }
    throw new Error(errorMsg);
  }

  if (res.status === 204) {
    return undefined as unknown as T;
  }

  return res.json();
}

export async function fetchLiveStats(): Promise<LiveStats> {
  return apiRequest<LiveStats>('/api/v1/metrics/live');
}

export async function fetchKillSwitch(): Promise<KillSwitchState> {
  return apiRequest<KillSwitchState>('/api/v1/killswitch');
}

export async function setKillSwitch(active: boolean, reason: string): Promise<KillSwitchState> {
  return apiRequest<KillSwitchState>('/api/v1/killswitch', {
    method: 'POST',
    body: JSON.stringify({
      global_active: active,
      reason,
      activated_by: 'web-dashboard-operator',
    }),
  });
}

export async function fetchUpstreams(): Promise<Upstream[]> {
  return apiRequest<Upstream[]>('/api/v1/upstreams');
}

export async function createUpstream(u: Partial<Upstream>): Promise<Upstream> {
  return apiRequest<Upstream>('/api/v1/upstreams', {
    method: 'POST',
    body: JSON.stringify(u),
  });
}

export async function deleteUpstream(id: string): Promise<void> {
  return apiRequest<void>(`/api/v1/upstreams/${id}`, {
    method: 'DELETE',
  });
}

export async function fetchExperiments(upstreamId?: string): Promise<Experiment[]> {
  const url = upstreamId
    ? `/api/v1/experiments?upstream_id=${upstreamId}`
    : `/api/v1/experiments`;
  return apiRequest<Experiment[]>(url);
}

export async function createExperiment(exp: Partial<Experiment>): Promise<Experiment> {
  return apiRequest<Experiment>('/api/v1/experiments', {
    method: 'POST',
    body: JSON.stringify(exp),
  });
}

export async function updateExperimentStatus(id: string, status: ExperimentStatus): Promise<void> {
  return apiRequest<void>(`/api/v1/experiments/${id}/status`, {
    method: 'POST',
    body: JSON.stringify({ status }),
  });
}

export async function deleteExperiment(id: string): Promise<void> {
  return apiRequest<void>(`/api/v1/experiments/${id}`, {
    method: 'DELETE',
  });
}

export async function createFaultRule(expId: string, rule: Partial<FaultRule>): Promise<FaultRule> {
  return apiRequest<FaultRule>(`/api/v1/experiments/${expId}/rules`, {
    method: 'POST',
    body: JSON.stringify(rule),
  });
}

export async function deleteFaultRule(ruleId: string): Promise<void> {
  return apiRequest<void>(`/api/v1/rules/${ruleId}`, {
    method: 'DELETE',
  });
}

export async function fetchAuditLogs(limit = 50, offset = 0): Promise<AuditLog[]> {
  return apiRequest<AuditLog[]>(`/api/v1/audit-logs?limit=${limit}&offset=${offset}`);
}

export async function fetchAPIKeys(): Promise<APIKey[]> {
  return apiRequest<APIKey[]>('/api/v1/api-keys');
}

export async function createAPIKey(
  name: string,
  role: Role
): Promise<{ api_key: APIKey; secret_key: string }> {
  return apiRequest<{ api_key: APIKey; secret_key: string }>('/api/v1/api-keys', {
    method: 'POST',
    body: JSON.stringify({ name, role }),
  });
}

export async function revokeAPIKey(id: string): Promise<void> {
  return apiRequest<void>(`/api/v1/api-keys/${id}`, {
    method: 'DELETE',
  });
}

export async function dispatchTestStudio(payload: {
  upstream_slug: string;
  method: string;
  path: string;
  headers: Record<string, string>;
  payload: string;
}): Promise<TestStudioResult> {
  return apiRequest<TestStudioResult>('/api/v1/test-studio/dispatch', {
    method: 'POST',
    body: JSON.stringify(payload),
  });
}
