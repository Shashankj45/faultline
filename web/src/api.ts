import {
  Upstream,
  Experiment,
  FaultRule,
  KillSwitchState,
  AuditLog,
  LiveStats,
  TestStudioResult,
  ExperimentStatus,
} from './types';

const BASE_URL = ''; // Relative path works seamlessly through Vite proxy or Go direct serve

export async function fetchLiveStats(): Promise<LiveStats> {
  const res = await fetch(`${BASE_URL}/api/v1/metrics/live`);
  if (!res.ok) throw new Error('Failed to fetch live metrics');
  return res.json();
}

export async function fetchKillSwitch(): Promise<KillSwitchState> {
  const res = await fetch(`${BASE_URL}/api/v1/killswitch`);
  if (!res.ok) throw new Error('Failed to fetch kill switch state');
  return res.json();
}

export async function setKillSwitch(active: boolean, reason: string): Promise<KillSwitchState> {
  const res = await fetch(`${BASE_URL}/api/v1/killswitch`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({
      global_active: active,
      reason,
      activated_by: 'web-dashboard-operator',
    }),
  });
  if (!res.ok) throw new Error('Failed to toggle kill switch');
  return res.json();
}

export async function fetchUpstreams(): Promise<Upstream[]> {
  const res = await fetch(`${BASE_URL}/api/v1/upstreams`);
  if (!res.ok) throw new Error('Failed to fetch upstreams');
  return res.json();
}

export async function createUpstream(u: Partial<Upstream>): Promise<Upstream> {
  const res = await fetch(`${BASE_URL}/api/v1/upstreams`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(u),
  });
  if (!res.ok) {
    const err = await res.json().catch(() => ({}));
    throw new Error(err.error || 'Failed to create upstream');
  }
  return res.json();
}

export async function deleteUpstream(id: string): Promise<void> {
  const res = await fetch(`${BASE_URL}/api/v1/upstreams/${id}`, {
    method: 'DELETE',
  });
  if (!res.ok) throw new Error('Failed to delete upstream');
}

export async function fetchExperiments(upstreamId?: string): Promise<Experiment[]> {
  const url = upstreamId
    ? `${BASE_URL}/api/v1/experiments?upstream_id=${upstreamId}`
    : `${BASE_URL}/api/v1/experiments`;
  const res = await fetch(url);
  if (!res.ok) throw new Error('Failed to fetch experiments');
  return res.json();
}

export async function createExperiment(exp: Partial<Experiment>): Promise<Experiment> {
  const res = await fetch(`${BASE_URL}/api/v1/experiments`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(exp),
  });
  if (!res.ok) {
    const err = await res.json().catch(() => ({}));
    throw new Error(err.error || 'Failed to create experiment');
  }
  return res.json();
}

export async function updateExperimentStatus(id: string, status: ExperimentStatus): Promise<void> {
  const res = await fetch(`${BASE_URL}/api/v1/experiments/${id}/status`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ status }),
  });
  if (!res.ok) throw new Error('Failed to update experiment status');
}

export async function deleteExperiment(id: string): Promise<void> {
  const res = await fetch(`${BASE_URL}/api/v1/experiments/${id}`, {
    method: 'DELETE',
  });
  if (!res.ok) throw new Error('Failed to delete experiment');
}

export async function createFaultRule(expId: string, rule: Partial<FaultRule>): Promise<FaultRule> {
  const res = await fetch(`${BASE_URL}/api/v1/experiments/${expId}/rules`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(rule),
  });
  if (!res.ok) throw new Error('Failed to create fault rule');
  return res.json();
}

export async function deleteFaultRule(ruleId: string): Promise<void> {
  const res = await fetch(`${BASE_URL}/api/v1/rules/${ruleId}`, {
    method: 'DELETE',
  });
  if (!res.ok) throw new Error('Failed to delete fault rule');
}

export async function fetchAuditLogs(limit = 50, offset = 0): Promise<AuditLog[]> {
  const res = await fetch(`${BASE_URL}/api/v1/audit-logs?limit=${limit}&offset=${offset}`);
  if (!res.ok) throw new Error('Failed to fetch audit logs');
  return res.json();
}

export async function dispatchTestStudio(payload: {
  upstream_slug: string;
  method: string;
  path: string;
  headers: Record<string, string>;
  payload: string;
}): Promise<TestStudioResult> {
  const res = await fetch(`${BASE_URL}/api/v1/test-studio/dispatch`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(payload),
  });
  if (!res.ok) throw new Error('Failed to execute simulated request');
  return res.json();
}
