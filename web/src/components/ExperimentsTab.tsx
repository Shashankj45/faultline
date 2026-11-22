import React, { useState } from 'react';
import {
  Flame,
  Plus,
  Play,
  Pause,
  StopCircle,
  Trash2,
  Clock,
  AlertTriangle,
  Zap,
  Sliders,
  Filter,
  CheckCircle2,
} from 'lucide-react';
import { Experiment, Upstream, FaultRule, FaultType, ExperimentStatus } from '../types';
import {
  createExperiment,
  updateExperimentStatus,
  deleteExperiment,
  createFaultRule,
  deleteFaultRule,
} from '../api';

interface Props {
  experiments: Experiment[];
  upstreams: Upstream[];
  onRefresh: () => void;
}

export const ExperimentsTab: React.FC<Props> = ({ experiments, upstreams, onRefresh }) => {
  const [showExpModal, setShowExpModal] = useState(false);
  const [showRuleModal, setShowRuleModal] = useState(false);
  const [selectedExpId, setSelectedExpId] = useState<string>('');
  const [loading, setLoading] = useState(false);

  // New Exp Form
  const [expName, setExpName] = useState('');
  const [expDesc, setExpDesc] = useState('');
  const [expUpstreamId, setExpUpstreamId] = useState('');
  const [expMaxDuration, setExpMaxDuration] = useState(3600);

  // New Rule Form
  const [ruleName, setRuleName] = useState('');
  const [faultType, setFaultType] = useState<FaultType>('latency');
  const [probability, setProbability] = useState(1.0);
  const [priority, setPriority] = useState(10);
  const [matchMethods, setMatchMethods] = useState<string[]>(['POST']);
  const [matchPath, setMatchPath] = useState('');

  // Latency Config
  const [latencyDist, setLatencyDist] = useState<'fixed' | 'uniform' | 'gaussian' | 'ramp'>('uniform');
  const [fixedLatencyMs, setFixedLatencyMs] = useState(200);
  const [jitterMinMs, setJitterMinMs] = useState(100);
  const [jitterMaxMs, setJitterMaxMs] = useState(400);

  // HTTP Error Config
  const [errStatusCode, setErrStatusCode] = useState(503);
  const [errBody, setErrBody] = useState('{"error":"Simulated Service Unavailable","code":"CHAOS_DOWN"}');
  const [errPreUpstream, setErrPreUpstream] = useState(true);

  // Connection Drop Config
  const [dropType, setDropType] = useState<'tcp_reset' | 'empty_eof' | 'partial_body' | 'malformed_bytes'>('tcp_reset');

  // Timeout Config
  const [timeoutType, setTimeoutType] = useState<'gateway_timeout' | 'client_stall' | 'slow_write'>('gateway_timeout');
  const [timeoutDelayMs, setTimeoutDelayMs] = useState(2500);

  // Bandwidth Config
  const [rateBytesPerSec, setRateBytesPerSec] = useState(65536); // 64 KB/s

  // Burst Config
  const [burstDurationMs, setBurstDurationMs] = useState(3000);
  const [burstIntervalMs, setBurstIntervalMs] = useState(15000);

  const handleCreateExperiment = async (e: React.FormEvent) => {
    e.preventDefault();
    setLoading(true);
    try {
      await createExperiment({
        name: expName,
        description: expDesc,
        upstream_id: expUpstreamId || upstreams[0]?.id,
        max_duration_seconds: expMaxDuration,
        status: 'draft',
      });
      setShowExpModal(false);
      setExpName('');
      setExpDesc('');
      onRefresh();
    } catch (err: any) {
      alert(err.message || 'Failed to create experiment');
    } finally {
      setLoading(false);
    }
  };

  const handleStatusChange = async (id: string, status: ExperimentStatus) => {
    try {
      await updateExperimentStatus(id, status);
      onRefresh();
    } catch (err: any) {
      alert(err.message || 'Failed to change experiment status');
    }
  };

  const handleDeleteExp = async (id: string, name: string) => {
    if (!confirm(`Delete experiment "${name}"?`)) return;
    try {
      await deleteExperiment(id);
      onRefresh();
    } catch (err: any) {
      alert(err.message || 'Failed to delete experiment');
    }
  };

  const handleCreateRule = async (e: React.FormEvent) => {
    e.preventDefault();
    if (!selectedExpId) return;

    let config: any = {};
    if (faultType === 'latency') {
      config = {
        distribution: latencyDist,
        fixed_latency_ms: Number(fixedLatencyMs),
        jitter_min_ms: Number(jitterMinMs),
        jitter_max_ms: Number(jitterMaxMs),
      };
    } else if (faultType === 'http_error') {
      config = {
        status_code: Number(errStatusCode),
        response_body: errBody,
        pre_upstream: errPreUpstream,
      };
    } else if (faultType === 'connection_drop') {
      config = {
        drop_type: dropType,
      };
    } else if (faultType === 'timeout') {
      config = {
        timeout_type: timeoutType,
        delay_ms: Number(timeoutDelayMs),
      };
    } else if (faultType === 'bandwidth_limit') {
      config = {
        direction: 'downstream',
        rate_bytes_per_sec: Number(rateBytesPerSec),
      };
    } else if (faultType === 'burst') {
      config = {
        burst_duration_ms: Number(burstDurationMs),
        burst_interval_ms: Number(burstIntervalMs),
        burst_status_code: 503,
      };
    }

    setLoading(true);
    try {
      await createFaultRule(selectedExpId, {
        name: ruleName,
        fault_type: faultType,
        probability: Number(probability),
        priority: Number(priority),
        match_methods: matchMethods,
        match_path_pattern: matchPath,
        config,
        is_enabled: true,
      });

      setShowRuleModal(false);
      setRuleName('');
      setMatchPath('');
      onRefresh();
    } catch (err: any) {
      alert(err.message || 'Failed to create rule');
    } finally {
      setLoading(false);
    }
  };

  const handleDeleteRule = async (ruleId: string) => {
    if (!confirm('Delete this fault rule?')) return;
    try {
      await deleteFaultRule(ruleId);
      onRefresh();
    } catch (err: any) {
      alert(err.message || 'Failed to delete rule');
    }
  };

  const getStatusBadge = (status: ExperimentStatus) => {
    switch (status) {
      case 'running':
        return <span className="fl-badge bg-rose-950 text-rose-400 border border-rose-800 animate-pulse">Running</span>;
      case 'paused':
        return <span className="fl-badge bg-amber-950 text-amber-400 border border-amber-800">Paused</span>;
      case 'completed':
        return <span className="fl-badge bg-emerald-950 text-emerald-400 border border-emerald-800">Completed</span>;
      case 'aborted':
        return <span className="fl-badge bg-slate-900 text-slate-400 border border-slate-800">Aborted</span>;
      default:
        return <span className="fl-badge bg-slate-900 text-slate-300 border border-slate-700">Draft</span>;
    }
  };

  const getFaultTypeBadge = (ft: FaultType) => {
    const colors: Record<FaultType, string> = {
      latency: 'bg-amber-950 text-amber-400 border-amber-800',
      http_error: 'bg-rose-950 text-rose-400 border-rose-800',
      connection_drop: 'bg-purple-950 text-purple-400 border-purple-800',
      timeout: 'bg-red-950 text-red-400 border-red-800',
      bandwidth_limit: 'bg-sky-950 text-sky-400 border-sky-800',
      packet_loss: 'bg-orange-950 text-orange-400 border-orange-800',
      burst: 'bg-pink-950 text-pink-400 border-pink-800',
    };
    return (
      <span className={`fl-badge border uppercase ${colors[ft] || 'bg-slate-900 text-slate-300'}`}>
        {ft.replace('_', ' ')}
      </span>
    );
  };

  return (
    <div className="space-y-6">
      {/* Header */}
      <div className="flex flex-wrap items-center justify-between gap-4">
        <div>
          <h2 className="text-xl font-bold text-white flex items-center gap-2">
            <Flame className="w-5 h-5 text-rose-500" />
            Chaos Experiments & Injection Rules
          </h2>
          <p className="text-xs text-slate-400 mt-1">
            Programmatic failure injection policies targeting registered upstream routes.
          </p>
        </div>

        <button
          onClick={() => setShowExpModal(true)}
          className="fl-btn-primary text-xs"
        >
          <Plus className="w-4 h-4" />
          Create Experiment
        </button>
      </div>

      {/* Experiments List */}
      <div className="space-y-5">
        {experiments.map((exp) => {
          const upstream = upstreams.find((u) => u.id === exp.upstream_id);
          const isRunning = exp.status === 'running';

          return (
            <div key={exp.id} className="fl-card overflow-hidden">
              {/* Exp Header */}
              <div className="p-5 border-b border-slate-800/80 flex flex-wrap items-center justify-between gap-4 bg-slate-900/40">
                <div className="space-y-1">
                  <div className="flex items-center gap-2.5">
                    <h3 className="font-bold text-base text-white">{exp.name}</h3>
                    {getStatusBadge(exp.status)}
                    <span className="text-xs text-slate-500 font-mono">
                      Target: <strong className="text-slate-300">{upstream?.name || exp.upstream_id}</strong>
                    </span>
                  </div>
                  {exp.description && (
                    <p className="text-xs text-slate-400">{exp.description}</p>
                  )}
                </div>

                {/* Lifecycle Controls */}
                <div className="flex items-center gap-2">
                  {exp.status !== 'running' ? (
                    <button
                      onClick={() => handleStatusChange(exp.id, 'running')}
                      className="bg-rose-600 hover:bg-rose-500 text-white text-xs font-semibold px-3 py-1.5 rounded-lg flex items-center gap-1.5 cursor-pointer shadow-md shadow-rose-950"
                    >
                      <Play className="w-3.5 h-3.5 fill-current" /> Start Experiment
                    </button>
                  ) : (
                    <button
                      onClick={() => handleStatusChange(exp.id, 'paused')}
                      className="bg-amber-600 hover:bg-amber-500 text-white text-xs font-semibold px-3 py-1.5 rounded-lg flex items-center gap-1.5 cursor-pointer"
                    >
                      <Pause className="w-3.5 h-3.5 fill-current" /> Pause
                    </button>
                  )}

                  {exp.status === 'running' && (
                    <button
                      onClick={() => handleStatusChange(exp.id, 'completed')}
                      className="bg-slate-800 hover:bg-slate-700 text-slate-300 text-xs px-2.5 py-1.5 rounded-lg flex items-center gap-1 cursor-pointer"
                      title="Mark as Completed"
                    >
                      <StopCircle className="w-3.5 h-3.5" /> Stop
                    </button>
                  )}

                  <button
                    onClick={() => {
                      setSelectedExpId(exp.id);
                      setShowRuleModal(true);
                    }}
                    className="fl-btn-secondary text-xs py-1.5"
                  >
                    <Plus className="w-3.5 h-3.5" /> Add Rule
                  </button>

                  <button
                    onClick={() => handleDeleteExp(exp.id, exp.name)}
                    className="text-slate-500 hover:text-rose-400 p-1.5 ml-1 transition-colors cursor-pointer"
                    title="Delete Experiment"
                  >
                    <Trash2 className="w-4 h-4" />
                  </button>
                </div>
              </div>

              {/* Rules Table */}
              <div className="p-5">
                <div className="text-xs font-bold text-slate-400 uppercase tracking-wider mb-3 flex items-center justify-between">
                  <span>Injected Fault Rules ({exp.rules?.length || 0})</span>
                  <span className="text-[11px] text-slate-500 font-normal">
                    Rules evaluate by Priority in descending order
                  </span>
                </div>

                {exp.rules && exp.rules.length > 0 ? (
                  <div className="overflow-x-auto">
                    <table className="w-full text-left text-xs font-mono">
                      <thead>
                        <tr className="text-slate-500 border-b border-slate-800 pb-2">
                          <th className="pb-2">Rule Name</th>
                          <th className="pb-2">Fault Type</th>
                          <th className="pb-2">Probability</th>
                          <th className="pb-2">Scope / Pattern</th>
                          <th className="pb-2">Parameters</th>
                          <th className="pb-2 text-right">Action</th>
                        </tr>
                      </thead>
                      <tbody className="divide-y divide-slate-800/60">
                        {exp.rules.map((rule) => {
                          const probPct = Math.round(rule.probability * 100);
                          return (
                            <tr key={rule.id} className="hover:bg-slate-900/40">
                              <td className="py-2.5 font-bold text-slate-200">
                                {rule.name}
                              </td>
                              <td className="py-2.5">
                                {getFaultTypeBadge(rule.fault_type)}
                              </td>
                              <td className="py-2.5">
                                <span className="text-amber-400 font-bold">{probPct}%</span>
                              </td>
                              <td className="py-2.5 text-slate-400">
                                {rule.match_methods?.length ? (
                                  <span className="text-sky-400 mr-1.5">[{rule.match_methods.join(',')}]</span>
                                ) : null}
                                <span>{rule.match_path_pattern || '/*'}</span>
                              </td>
                              <td className="py-2.5 text-slate-400 max-w-xs truncate">
                                {typeof rule.config === 'object'
                                  ? JSON.stringify(rule.config)
                                  : String(rule.config)}
                              </td>
                              <td className="py-2.5 text-right">
                                <button
                                  onClick={() => handleDeleteRule(rule.id)}
                                  className="text-slate-600 hover:text-rose-400 p-1 transition-colors cursor-pointer"
                                  title="Delete Rule"
                                >
                                  <Trash2 className="w-3.5 h-3.5" />
                                </button>
                              </td>
                            </tr>
                          );
                        })}
                      </tbody>
                    </table>
                  </div>
                ) : (
                  <div className="py-8 text-center text-xs text-slate-500">
                    No rules attached to this experiment. Click "Add Rule" to configure latency or error injection.
                  </div>
                )}
              </div>
            </div>
          );
        })}
      </div>

      {/* New Experiment Modal */}
      {showExpModal && (
        <div className="fixed inset-0 bg-black/80 backdrop-blur-sm z-50 flex items-center justify-center p-4">
          <div className="fl-card w-full max-w-lg p-6 relative border-slate-700 shadow-2xl">
            <h3 className="text-base font-bold text-white mb-2 flex items-center gap-2">
              <Flame className="w-5 h-5 text-rose-500" />
              Create Chaos Experiment
            </h3>
            <p className="text-xs text-slate-400 mb-5">
              Experiments contain one or more failure injection rules scoped to an upstream service.
            </p>

            <form onSubmit={handleCreateExperiment} className="space-y-4">
              <div>
                <label className="block text-xs font-semibold text-slate-300 mb-1">Experiment Name</label>
                <input
                  type="text"
                  required
                  placeholder="e.g. Black Friday Stripe Gateway Downtime"
                  value={expName}
                  onChange={(e) => setExpName(e.target.value)}
                  className="fl-input w-full"
                />
              </div>

              <div>
                <label className="block text-xs font-semibold text-slate-300 mb-1">Target Upstream Service</label>
                <select
                  value={expUpstreamId}
                  onChange={(e) => setExpUpstreamId(e.target.value)}
                  className="fl-input w-full font-mono text-xs"
                >
                  {upstreams.map((u) => (
                    <option key={u.id} value={u.id}>
                      {u.name} (/proxy/{u.slug})
                    </option>
                  ))}
                </select>
              </div>

              <div>
                <label className="block text-xs font-semibold text-slate-300 mb-1">Description / Hypothesis</label>
                <textarea
                  rows={2}
                  placeholder="e.g. Verify that client falls back to cached prices when payment upstream returns 503."
                  value={expDesc}
                  onChange={(e) => setExpDesc(e.target.value)}
                  className="fl-input w-full"
                />
              </div>

              <div>
                <label className="block text-xs font-semibold text-slate-300 mb-1">
                  Safety Auto-Stop TTL (seconds)
                </label>
                <input
                  type="number"
                  min="60"
                  max="86400"
                  value={expMaxDuration}
                  onChange={(e) => setExpMaxDuration(Number(e.target.value))}
                  className="fl-input w-full font-mono text-xs"
                />
                <p className="text-[11px] text-slate-500 mt-1">
                  Prevents lingering chaos by automatically aborting the experiment after this time.
                </p>
              </div>

              <div className="pt-2 flex justify-end gap-3">
                <button
                  type="button"
                  onClick={() => setShowExpModal(false)}
                  className="fl-btn-secondary text-xs"
                >
                  Cancel
                </button>
                <button
                  type="submit"
                  disabled={loading}
                  className="fl-btn-primary text-xs"
                >
                  {loading ? 'Creating...' : 'Create Experiment'}
                </button>
              </div>
            </form>
          </div>
        </div>
      )}

      {/* Add Fault Rule Modal */}
      {showRuleModal && (
        <div className="fixed inset-0 bg-black/80 backdrop-blur-sm z-50 flex items-center justify-center p-4">
          <div className="fl-card w-full max-w-xl p-6 relative border-slate-700 shadow-2xl max-h-[90vh] overflow-y-auto">
            <h3 className="text-base font-bold text-white mb-2 flex items-center gap-2">
              <Zap className="w-5 h-5 text-rose-500" />
              Configure Injected Fault Rule
            </h3>
            <p className="text-xs text-slate-400 mb-5">
              Select fault type, failure probability, and request matching criteria.
            </p>

            <form onSubmit={handleCreateRule} className="space-y-4">
              <div>
                <label className="block text-xs font-semibold text-slate-300 mb-1">Rule Name</label>
                <input
                  type="text"
                  required
                  placeholder="e.g. Checkout 500ms Latency Jitter"
                  value={ruleName}
                  onChange={(e) => setRuleName(e.target.value)}
                  className="fl-input w-full"
                />
              </div>

              <div className="grid grid-cols-2 gap-3">
                <div>
                  <label className="block text-xs font-semibold text-slate-300 mb-1">Fault Type</label>
                  <select
                    value={faultType}
                    onChange={(e) => setFaultType(e.target.value as FaultType)}
                    className="fl-input w-full font-mono text-xs"
                  >
                    <option value="latency">Artificial Network Latency</option>
                    <option value="http_error">HTTP Error Injection (5xx/429)</option>
                    <option value="connection_drop">Connection Drop (TCP Reset / EOF)</option>
                    <option value="timeout">Timeout Simulation (Slowloris/Stall)</option>
                    <option value="bandwidth_limit">Bandwidth Throttling</option>
                    <option value="burst">Burst Failure Spike</option>
                  </select>
                </div>

                <div>
                  <label className="block text-xs font-semibold text-slate-300 mb-1">
                    Failure Probability ({Math.round(probability * 100)}%)
                  </label>
                  <input
                    type="range"
                    min="0.01"
                    max="1.0"
                    step="0.01"
                    value={probability}
                    onChange={(e) => setProbability(Number(e.target.value))}
                    className="w-full accent-rose-500 mt-2"
                  />
                </div>
              </div>

              {/* Scoping Filters */}
              <div className="grid grid-cols-2 gap-3">
                <div>
                  <label className="block text-xs font-semibold text-slate-300 mb-1">Path Pattern (Regex/Prefix)</label>
                  <input
                    type="text"
                    placeholder="e.g. ^/v1/charges or /api/orders"
                    value={matchPath}
                    onChange={(e) => setMatchPath(e.target.value)}
                    className="fl-input w-full font-mono text-xs"
                  />
                </div>

                <div>
                  <label className="block text-xs font-semibold text-slate-300 mb-1">HTTP Methods</label>
                  <div className="flex gap-2 mt-1">
                    {['GET', 'POST', 'PUT', 'DELETE'].map((m) => {
                      const selected = matchMethods.includes(m);
                      return (
                        <button
                          key={m}
                          type="button"
                          onClick={() => {
                            if (selected) {
                              setMatchMethods(matchMethods.filter((x) => x !== m));
                            } else {
                              setMatchMethods([...matchMethods, m]);
                            }
                          }}
                          className={`text-xs px-2.5 py-1 rounded font-mono font-bold transition-colors cursor-pointer ${
                            selected
                              ? 'bg-rose-600 text-white'
                              : 'bg-slate-800 text-slate-400 hover:bg-slate-700'
                          }`}
                        >
                          {m}
                        </button>
                      );
                    })}
                  </div>
                </div>
              </div>

              {/* Type-Specific Parameter Panel */}
              <div className="p-4 bg-slate-900/90 rounded-lg border border-slate-800 space-y-3">
                <div className="text-xs font-bold text-rose-400 uppercase tracking-wider">
                  {faultType.replace('_', ' ')} Parameters
                </div>

                {faultType === 'latency' && (
                  <div className="space-y-3">
                    <div className="grid grid-cols-2 gap-3">
                      <div>
                        <label className="block text-[11px] text-slate-400 mb-1">Distribution</label>
                        <select
                          value={latencyDist}
                          onChange={(e) => setLatencyDist(e.target.value as any)}
                          className="fl-input w-full text-xs font-mono"
                        >
                          <option value="uniform">Uniform Jitter (Min-Max)</option>
                          <option value="fixed">Fixed Milliseconds</option>
                          <option value="gaussian">Gaussian / Normal Distribution</option>
                        </select>
                      </div>
                      {latencyDist === 'fixed' ? (
                        <div>
                          <label className="block text-[11px] text-slate-400 mb-1">Fixed Delay (ms)</label>
                          <input
                            type="number"
                            value={fixedLatencyMs}
                            onChange={(e) => setFixedLatencyMs(Number(e.target.value))}
                            className="fl-input w-full font-mono text-xs"
                          />
                        </div>
                      ) : (
                        <div className="flex gap-2">
                          <div>
                            <label className="block text-[11px] text-slate-400 mb-1">Min (ms)</label>
                            <input
                              type="number"
                              value={jitterMinMs}
                              onChange={(e) => setJitterMinMs(Number(e.target.value))}
                              className="fl-input w-full font-mono text-xs"
                            />
                          </div>
                          <div>
                            <label className="block text-[11px] text-slate-400 mb-1">Max (ms)</label>
                            <input
                              type="number"
                              value={jitterMaxMs}
                              onChange={(e) => setJitterMaxMs(Number(e.target.value))}
                              className="fl-input w-full font-mono text-xs"
                            />
                          </div>
                        </div>
                      )}
                    </div>
                  </div>
                )}

                {faultType === 'http_error' && (
                  <div className="space-y-3">
                    <div className="grid grid-cols-2 gap-3">
                      <div>
                        <label className="block text-[11px] text-slate-400 mb-1">HTTP Status Code</label>
                        <select
                          value={errStatusCode}
                          onChange={(e) => setErrStatusCode(Number(e.target.value))}
                          className="fl-input w-full font-mono text-xs"
                        >
                          <option value="500">500 Internal Server Error</option>
                          <option value="502">502 Bad Gateway</option>
                          <option value="503">503 Service Unavailable</option>
                          <option value="504">504 Gateway Timeout</option>
                          <option value="429">429 Too Many Requests</option>
                        </select>
                      </div>
                      <div className="flex items-center pt-5">
                        <label className="flex items-center gap-2 text-xs text-slate-300 cursor-pointer">
                          <input
                            type="checkbox"
                            checked={errPreUpstream}
                            onChange={(e) => setErrPreUpstream(e.target.checked)}
                            className="rounded bg-slate-950 border-slate-700 text-rose-500"
                          />
                          <span>Short-circuit Pre-Upstream</span>
                        </label>
                      </div>
                    </div>
                    <div>
                      <label className="block text-[11px] text-slate-400 mb-1">Response Body (JSON/Text)</label>
                      <textarea
                        rows={2}
                        value={errBody}
                        onChange={(e) => setErrBody(e.target.value)}
                        className="fl-input w-full font-mono text-xs"
                      />
                    </div>
                  </div>
                )}

                {faultType === 'connection_drop' && (
                  <div>
                    <label className="block text-[11px] text-slate-400 mb-1">Drop Technique</label>
                    <select
                      value={dropType}
                      onChange={(e) => setDropType(e.target.value as any)}
                      className="fl-input w-full font-mono text-xs"
                    >
                      <option value="tcp_reset">TCP RST (SetLinger 0 Raw Reset)</option>
                      <option value="empty_eof">Empty EOF (Immediate Silent Socket Close)</option>
                      <option value="partial_body">Partial Body Drop (Mid-Stream Truncation)</option>
                      <option value="malformed_bytes">Malformed Protocol Bytes</option>
                    </select>
                  </div>
                )}

                {faultType === 'timeout' && (
                  <div className="grid grid-cols-2 gap-3">
                    <div>
                      <label className="block text-[11px] text-slate-400 mb-1">Timeout Behavior</label>
                      <select
                        value={timeoutType}
                        onChange={(e) => setTimeoutType(e.target.value as any)}
                        className="fl-input w-full font-mono text-xs"
                      >
                        <option value="gateway_timeout">504 Gateway Timeout After Delay</option>
                        <option value="client_stall">Infinite Client Stall</option>
                        <option value="slow_write">Drip-Feed Slowloris Response</option>
                      </select>
                    </div>
                    <div>
                      <label className="block text-[11px] text-slate-400 mb-1">Delay (ms)</label>
                      <input
                        type="number"
                        value={timeoutDelayMs}
                        onChange={(e) => setTimeoutDelayMs(Number(e.target.value))}
                        className="fl-input w-full font-mono text-xs"
                      />
                    </div>
                  </div>
                )}

                {faultType === 'bandwidth_limit' && (
                  <div>
                    <label className="block text-[11px] text-slate-400 mb-1">Throttled Rate (Bytes / sec)</label>
                    <input
                      type="number"
                      value={rateBytesPerSec}
                      onChange={(e) => setRateBytesPerSec(Number(e.target.value))}
                      className="fl-input w-full font-mono text-xs"
                    />
                    <p className="text-[10px] text-slate-500 mt-1">
                      e.g. 65536 = 64 KB/sec download link throttling
                    </p>
                  </div>
                )}

                {faultType === 'burst' && (
                  <div className="grid grid-cols-2 gap-3">
                    <div>
                      <label className="block text-[11px] text-slate-400 mb-1">Burst Duration (ms)</label>
                      <input
                        type="number"
                        value={burstDurationMs}
                        onChange={(e) => setBurstDurationMs(Number(e.target.value))}
                        className="fl-input w-full font-mono text-xs"
                      />
                    </div>
                    <div>
                      <label className="block text-[11px] text-slate-400 mb-1">Burst Interval Cycle (ms)</label>
                      <input
                        type="number"
                        value={burstIntervalMs}
                        onChange={(e) => setBurstIntervalMs(Number(e.target.value))}
                        className="fl-input w-full font-mono text-xs"
                      />
                    </div>
                  </div>
                )}
              </div>

              <div className="pt-2 flex justify-end gap-3">
                <button
                  type="button"
                  onClick={() => setShowRuleModal(false)}
                  className="fl-btn-secondary text-xs"
                >
                  Cancel
                </button>
                <button
                  type="submit"
                  disabled={loading}
                  className="fl-btn-primary text-xs"
                >
                  {loading ? 'Adding Rule...' : 'Attach Rule to Experiment'}
                </button>
              </div>
            </form>
          </div>
        </div>
      )}
    </div>
  );
};
