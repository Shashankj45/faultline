import React from 'react';
import {
  Activity,
  Flame,
  Clock,
  Server,
  AlertOctagon,
  ArrowUpRight,
  TrendingUp,
  Shield,
  Cpu,
  Radio,
  Layers,
  Zap,
} from 'lucide-react';
import { LiveStats, Upstream, Experiment } from '../types';

interface Props {
  stats: LiveStats | null;
  upstreams: Upstream[];
  experiments: Experiment[];
  onNavigate: (tab: 'upstreams' | 'experiments' | 'test-studio' | 'security') => void;
}

export const OverviewTab: React.FC<Props> = ({ stats, upstreams, experiments, onNavigate }) => {
  const activeExps = experiments.filter((e) => e.status === 'running');
  const healthyUpstreams = upstreams.filter((u) => u.health?.healthy !== false);

  return (
    <div className="space-y-6">
      {/* High-Density Engine State Banner */}
      <div className="bg-[#0e1526] border border-slate-800 rounded p-3 text-xs font-mono">
        <div className="flex flex-wrap items-center justify-between gap-3 text-slate-400">
          <div className="flex items-center gap-2">
            <span className="w-2 h-2 rounded-full bg-emerald-400" />
            <span className="text-white font-semibold">Engine Core:</span>
            <span className="text-slate-300">Reverse Proxy Active</span>
          </div>

          <div className="flex flex-wrap items-center gap-4 text-[11px]">
            <span className="flex items-center gap-1.5">
              <Shield className="w-3.5 h-3.5 text-rose-400" />
              SSRF Defense: <strong className="text-slate-200">SafeDialer Hook</strong>
            </span>
            <span className="text-slate-700">|</span>
            <span className="flex items-center gap-1.5">
              <Cpu className="w-3.5 h-3.5 text-emerald-400" />
              Buffer Pool: <strong className="text-slate-200">32KB sync.Pool</strong>
            </span>
            <span className="text-slate-700">|</span>
            <span className="flex items-center gap-1.5">
              <Radio className="w-3.5 h-3.5 text-sky-400" />
              Distributed Tracing: <strong className="text-slate-200">W3C OpenTelemetry</strong>
            </span>
            <span className="text-slate-700">|</span>
            <span className="flex items-center gap-1.5">
              <Layers className="w-3.5 h-3.5 text-amber-400" />
              Storage: <strong className="text-slate-200">Dual Engine + Hot Cache</strong>
            </span>
            <span className="text-slate-700">|</span>
            <button
              onClick={() => onNavigate('security')}
              className="text-sky-400 hover:text-sky-300 font-semibold cursor-pointer underline"
            >
              Security Specs →
            </button>
          </div>
        </div>
      </div>

      {/* Metrics Grid */}
      <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-4 gap-4">
        {/* Card 1: Throughput */}
        <div className="fl-card p-4">
          <div className="flex items-center justify-between text-slate-400 text-xs font-mono mb-2">
            <span>Throughput (RPS)</span>
            <Activity className="w-4 h-4 text-sky-400" />
          </div>
          <div className="flex items-baseline gap-2">
            <span className="text-2xl font-bold text-white font-mono">
              {stats?.requests_per_sec.toFixed(1) || '0.0'}
            </span>
            <span className="text-xs text-slate-400 font-mono">req/sec</span>
          </div>
          <div className="mt-2.5 flex items-center justify-between text-[11px] text-slate-400 font-mono border-t border-slate-800/80 pt-2">
            <span>Total Requests:</span>
            <strong className="text-slate-200">{stats?.total_requests || 0}</strong>
          </div>
        </div>

        {/* Card 2: Latency Spectrum */}
        <div className="fl-card p-4">
          <div className="flex items-center justify-between text-slate-400 text-xs font-mono mb-2">
            <span>Latency Spectrum</span>
            <Clock className="w-4 h-4 text-amber-400" />
          </div>
          <div className="flex items-baseline gap-2">
            <span className="text-2xl font-bold text-amber-400 font-mono">
              {Math.round(stats?.p95_latency_ms || 0)}
            </span>
            <span className="text-xs text-slate-400 font-mono">ms (p95)</span>
          </div>
          <div className="mt-2.5 flex items-center justify-between text-[11px] text-slate-400 font-mono border-t border-slate-800/80 pt-2">
            <span>p50: <strong className="text-slate-200">{Math.round(stats?.p50_latency_ms || 0)}ms</strong></span>
            <span>p99: <strong className="text-rose-400">{Math.round(stats?.p99_latency_ms || 0)}ms</strong></span>
          </div>
        </div>

        {/* Card 3: Error Rate */}
        <div className="fl-card p-4">
          <div className="flex items-center justify-between text-slate-400 text-xs font-mono mb-2">
            <span>Failure Rate</span>
            <AlertOctagon className="w-4 h-4 text-rose-400" />
          </div>
          <div className="flex items-baseline gap-2">
            <span className="text-2xl font-bold text-rose-400 font-mono">
              {stats?.error_rate_percent.toFixed(1) || '0.0'}%
            </span>
            <span className="text-xs text-slate-400 font-mono">5xx / Aborts</span>
          </div>
          <div className="mt-2.5 flex items-center justify-between text-[11px] text-slate-400 font-mono border-t border-slate-800/80 pt-2">
            <span>Chaos Faults:</span>
            <strong className="text-rose-400">{stats?.total_faults || 0}</strong>
          </div>
        </div>

        {/* Card 4: Active Chaos */}
        <div className="fl-card p-4">
          <div className="flex items-center justify-between text-slate-400 text-xs font-mono mb-2">
            <span>Chaos Experiments</span>
            <Flame className="w-4 h-4 text-rose-500" />
          </div>
          <div className="flex items-baseline gap-2">
            <span className="text-2xl font-bold text-white font-mono">
              {activeExps.length}
            </span>
            <span className="text-xs text-slate-400 font-mono">/ {experiments.length} Active</span>
          </div>
          <div className="mt-2.5 flex items-center justify-between text-[11px] text-slate-400 font-mono border-t border-slate-800/80 pt-2">
            <span>Upstreams:</span>
            <strong className="text-emerald-400">{healthyUpstreams.length}/{upstreams.length} Healthy</strong>
          </div>
        </div>
      </div>

      {/* Traffic & Injected Chaos Activity Timeline */}
      <div className="fl-card p-5">
        <div className="flex items-center justify-between mb-4 border-b border-slate-800 pb-3">
          <div>
            <h3 className="text-xs font-bold text-white uppercase font-mono tracking-wider flex items-center gap-2">
              <Activity className="w-3.5 h-3.5 text-sky-400" />
              Ingress Traffic & Chaos Injection Timeline (60s Window)
            </h3>
            <p className="text-[11px] text-slate-400 font-mono mt-0.5">
              Live sliding-window throughput and simulated fault counts per second.
            </p>
          </div>
          <div className="flex items-center gap-4 text-xs font-mono">
            <span className="flex items-center gap-1.5 text-sky-400">
              <span className="w-2.5 h-2.5 rounded-sm bg-sky-500 inline-block" /> Requests
            </span>
            <span className="flex items-center gap-1.5 text-rose-400">
              <span className="w-2.5 h-2.5 rounded-sm bg-rose-500 inline-block" /> Injected Faults
            </span>
          </div>
        </div>

        {/* Timeline Visualization */}
        <div className="h-36 flex items-end gap-2 pt-4">
          {stats?.recent_history?.map((point, idx) => {
            const maxReq = Math.max(...(stats?.recent_history?.map((p) => p.request_count) || [1]), 10);
            const reqHeight = Math.max((point.request_count / maxReq) * 100, 6);
            const faultHeight = point.request_count > 0 ? (point.injected_count / point.request_count) * reqHeight : 0;

            return (
              <div key={idx} className="flex-1 flex flex-col items-center gap-1 h-full justify-end group relative">
                <div className="opacity-0 group-hover:opacity-100 transition-opacity absolute bottom-full mb-1.5 bg-[#090d15] border border-slate-700 text-xs p-2 rounded pointer-events-none z-10 w-36 font-mono text-[11px]">
                  <div className="text-slate-300 font-semibold">{point.time_iso}</div>
                  <div className="text-sky-400">Reqs: {point.request_count}</div>
                  <div className="text-rose-400">Faults: {point.injected_count}</div>
                  <div className="text-amber-400">Avg: {Math.round(point.avg_latency_ms)}ms</div>
                </div>

                <div className="w-full bg-slate-800 rounded-sm relative overflow-hidden flex flex-col justify-end" style={{ height: `${reqHeight}%` }}>
                  <div className="w-full bg-sky-600 h-full" />
                  {point.injected_count > 0 && (
                    <div
                      className="w-full bg-rose-600 absolute bottom-0 left-0"
                      style={{ height: `${faultHeight}%` }}
                    />
                  )}
                </div>
                <span className="text-[10px] text-slate-500 font-mono">{point.time_iso}</span>
              </div>
            );
          })}
        </div>
      </div>

      {/* Two Column Grid: Upstream Health & Injected Fault Types */}
      <div className="grid grid-cols-1 lg:grid-cols-2 gap-4">
        {/* Upstreams Health Radar */}
        <div className="fl-card p-4">
          <div className="flex items-center justify-between mb-3 border-b border-slate-800 pb-2.5">
            <h3 className="text-xs font-bold text-white uppercase font-mono tracking-wider flex items-center gap-2">
              <Server className="w-3.5 h-3.5 text-emerald-400" />
              Registered Upstream Radar
            </h3>
            <span className="text-xs text-emerald-400 font-mono">
              {healthyUpstreams.length}/{upstreams.length} Healthy
            </span>
          </div>

          <div className="space-y-2">
            {upstreams.map((u) => {
              const isHealthy = u.health?.healthy !== false;
              return (
                <div
                  key={u.id}
                  className="p-2.5 bg-[#0a0f1d] rounded border border-slate-800/80 flex items-center justify-between"
                >
                  <div className="flex items-center gap-2.5">
                    <span
                      className={`w-2 h-2 rounded-full ${
                        isHealthy ? 'bg-emerald-500' : 'bg-rose-500'
                      }`}
                    />
                    <div>
                      <div className="font-semibold text-xs text-white flex items-center gap-2">
                        {u.name}
                        <code className="text-[10px] bg-slate-800 text-slate-300 px-1 py-0.5 rounded font-mono">
                          /proxy/{u.slug}
                        </code>
                      </div>
                      <div className="text-[11px] text-slate-500 font-mono mt-0.5">
                        {u.target_url}
                      </div>
                    </div>
                  </div>

                  <div className="text-right font-mono text-xs">
                    <div className="text-slate-300">{u.health?.latency_ms ?? 0}ms</div>
                    <div className={`text-[10px] ${isHealthy ? 'text-emerald-400' : 'text-rose-400'}`}>
                      {isHealthy ? 'HEALTHY' : 'UNHEALTHY'}
                    </div>
                  </div>
                </div>
              );
            })}
          </div>
        </div>

        {/* Fault Injection Distribution */}
        <div className="fl-card p-4">
          <div className="flex items-center justify-between mb-3 border-b border-slate-800 pb-2.5">
            <h3 className="text-xs font-bold text-white uppercase font-mono tracking-wider flex items-center gap-2">
              <Flame className="w-3.5 h-3.5 text-rose-500" />
              Injected Chaos Distribution
            </h3>
            <span className="text-xs text-slate-400 font-mono">Telemetry Counters</span>
          </div>

          {stats?.fault_breakdown && Object.keys(stats.fault_breakdown).length > 0 ? (
            <div className="space-y-2.5">
              {Object.entries(stats.fault_breakdown).map(([fault, count]) => (
                <div key={fault} className="space-y-1">
                  <div className="flex justify-between text-xs font-mono">
                    <span className="text-slate-300 uppercase">{fault.replace('_', ' ')}</span>
                    <span className="text-rose-400 font-semibold">{count} injected</span>
                  </div>
                  <div className="w-full bg-slate-800 h-1.5 rounded-sm overflow-hidden">
                    <div
                      className="bg-rose-600 h-full rounded-sm"
                      style={{ width: `${Math.min((count / (stats.total_faults || 1)) * 100, 100)}%` }}
                    />
                  </div>
                </div>
              ))}
            </div>
          ) : (
            <div className="py-8 text-center text-slate-500 text-xs font-mono">
              No chaos faults recorded yet. Dispatch a test request in the Test Studio to simulate failures.
            </div>
          )}

          <div className="mt-4 pt-3 border-t border-slate-800 flex justify-between items-center">
            <span className="text-xs text-slate-400 font-mono">Simulate failures live:</span>
            <button
              onClick={() => onNavigate('test-studio')}
              className="fl-btn-primary text-xs"
            >
              Open Test Studio
            </button>
          </div>
        </div>
      </div>
    </div>
  );
};
