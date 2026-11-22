import React from 'react';
import {
  Activity,
  Flame,
  Zap,
  Clock,
  Server,
  AlertOctagon,
  ArrowUpRight,
  TrendingUp,
} from 'lucide-react';
import { LiveStats, Upstream, Experiment } from '../types';

interface Props {
  stats: LiveStats | null;
  upstreams: Upstream[];
  experiments: Experiment[];
  onNavigate: (tab: 'upstreams' | 'experiments' | 'test-studio') => void;
}

export const OverviewTab: React.FC<Props> = ({ stats, upstreams, experiments, onNavigate }) => {
  const activeExps = experiments.filter((e) => e.status === 'running');
  const healthyUpstreams = upstreams.filter((u) => u.health?.healthy !== false);

  return (
    <div className="space-y-6">
      {/* Metrics Grid */}
      <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-4 gap-4">
        {/* Card 1: Throughput */}
        <div className="fl-card p-5 relative overflow-hidden group hover:border-slate-700 transition-all">
          <div className="flex items-center justify-between text-slate-400 text-xs font-semibold tracking-wider uppercase mb-2">
            <span>Throughput</span>
            <Activity className="w-4 h-4 text-sky-400" />
          </div>
          <div className="flex items-baseline gap-2">
            <span className="text-3xl font-extrabold text-white font-mono">
              {stats?.requests_per_sec.toFixed(1) || '0.0'}
            </span>
            <span className="text-xs text-slate-400 font-medium">req/sec</span>
          </div>
          <div className="mt-3 flex items-center gap-1.5 text-xs text-slate-500 font-mono">
            <span>Total:</span>
            <span className="text-slate-300 font-semibold">{stats?.total_requests || 0}</span>
            <span className="text-slate-600">|</span>
            <span className="text-emerald-400 flex items-center">
              <TrendingUp className="w-3 h-3 mr-0.5" /> 60s window
            </span>
          </div>
        </div>

        {/* Card 2: Latency Spectrum */}
        <div className="fl-card p-5 relative overflow-hidden group hover:border-slate-700 transition-all">
          <div className="flex items-center justify-between text-slate-400 text-xs font-semibold tracking-wider uppercase mb-2">
            <span>Latency (p95 / p99)</span>
            <Clock className="w-4 h-4 text-amber-400" />
          </div>
          <div className="flex items-baseline gap-2">
            <span className="text-3xl font-extrabold text-amber-400 font-mono">
              {Math.round(stats?.p95_latency_ms || 0)}
            </span>
            <span className="text-xs text-slate-400 font-medium">ms (p95)</span>
          </div>
          <div className="mt-3 flex items-center justify-between text-xs text-slate-400 font-mono">
            <span>p50: <strong className="text-slate-200">{Math.round(stats?.p50_latency_ms || 0)}ms</strong></span>
            <span>p99: <strong className="text-rose-400">{Math.round(stats?.p99_latency_ms || 0)}ms</strong></span>
          </div>
        </div>

        {/* Card 3: Error Rate */}
        <div className="fl-card p-5 relative overflow-hidden group hover:border-slate-700 transition-all">
          <div className="flex items-center justify-between text-slate-400 text-xs font-semibold tracking-wider uppercase mb-2">
            <span>Error Rate</span>
            <AlertOctagon className="w-4 h-4 text-rose-400" />
          </div>
          <div className="flex items-baseline gap-2">
            <span className="text-3xl font-extrabold text-rose-400 font-mono">
              {stats?.error_rate_percent.toFixed(1) || '0.0'}%
            </span>
            <span className="text-xs text-slate-400 font-medium">5xx / Failures</span>
          </div>
          <div className="mt-3 flex items-center gap-2 text-xs text-slate-400">
            <div className="w-full bg-slate-800 h-1.5 rounded-full overflow-hidden">
              <div
                className="bg-rose-500 h-full rounded-full transition-all duration-500"
                style={{ width: `${Math.min(stats?.error_rate_percent || 0, 100)}%` }}
              />
            </div>
          </div>
        </div>

        {/* Card 4: Active Chaos */}
        <div className="fl-card p-5 relative overflow-hidden group hover:border-slate-700 transition-all">
          <div className="flex items-center justify-between text-slate-400 text-xs font-semibold tracking-wider uppercase mb-2">
            <span>Active Chaos Experiments</span>
            <Flame className="w-4 h-4 text-rose-500" />
          </div>
          <div className="flex items-baseline gap-2">
            <span className="text-3xl font-extrabold text-white font-mono">
              {activeExps.length}
            </span>
            <span className="text-xs text-slate-400 font-medium">/ {experiments.length} Total</span>
          </div>
          <div className="mt-3 flex items-center justify-between text-xs text-slate-400 font-mono">
            <span>Injected: <strong className="text-rose-400">{stats?.total_faults || 0}</strong></span>
            <button
              onClick={() => onNavigate('experiments')}
              className="text-rose-400 hover:text-rose-300 flex items-center gap-0.5 cursor-pointer"
            >
              Manage <ArrowUpRight className="w-3 h-3" />
            </button>
          </div>
        </div>
      </div>

      {/* Traffic & Injected Chaos Activity Timeline */}
      <div className="fl-card p-6">
        <div className="flex items-center justify-between mb-4">
          <div>
            <h3 className="text-base font-bold text-white flex items-center gap-2">
              <Zap className="w-4 h-4 text-rose-500" />
              Live Ingress & Chaos Timeline (Last 60 Seconds)
            </h3>
            <p className="text-xs text-slate-400 mt-0.5">
              Real-time request volume, injected fault frequency, and average request latency.
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

        {/* Timeline Chart Visualization */}
        <div className="h-44 flex items-end gap-3 pt-6 border-b border-slate-800">
          {stats?.recent_history?.map((point, idx) => {
            const maxReq = Math.max(...(stats?.recent_history?.map((p) => p.request_count) || [1]), 10);
            const reqHeight = Math.max((point.request_count / maxReq) * 100, 8);
            const faultHeight = point.request_count > 0 ? (point.injected_count / point.request_count) * reqHeight : 0;

            return (
              <div key={idx} className="flex-1 flex flex-col items-center gap-1 h-full justify-end group relative">
                {/* Tooltip */}
                <div className="opacity-0 group-hover:opacity-100 transition-opacity absolute bottom-full mb-2 bg-slate-900 border border-slate-700 text-xs p-2 rounded shadow-xl pointer-events-none z-10 w-36 font-mono text-[11px]">
                  <div className="text-slate-300 font-bold">{point.time_iso}</div>
                  <div className="text-sky-400">Reqs: {point.request_count}</div>
                  <div className="text-rose-400">Faults: {point.injected_count}</div>
                  <div className="text-amber-400">Avg: {Math.round(point.avg_latency_ms)}ms</div>
                </div>

                {/* Bars */}
                <div className="w-full max-w-[40px] bg-slate-800/80 rounded-t relative overflow-hidden flex flex-col justify-end" style={{ height: `${reqHeight}%` }}>
                  <div className="w-full bg-sky-500/80 h-full" />
                  {point.injected_count > 0 && (
                    <div
                      className="w-full bg-rose-500 absolute bottom-0 left-0"
                      style={{ height: `${faultHeight}%` }}
                    />
                  )}
                </div>
                <span className="text-[10px] text-slate-500 font-mono mt-1">{point.time_iso}</span>
              </div>
            );
          })}
        </div>
      </div>

      {/* Two Column Grid: Upstream Health & Injected Fault Types */}
      <div className="grid grid-cols-1 lg:grid-cols-2 gap-6">
        {/* Upstreams Health Radar */}
        <div className="fl-card p-6">
          <div className="flex items-center justify-between mb-4">
            <h3 className="text-sm font-bold text-white flex items-center gap-2">
              <Server className="w-4 h-4 text-emerald-400" />
              Registered Upstream Radar
            </h3>
            <span className="text-xs text-emerald-400 font-mono">
              {healthyUpstreams.length}/{upstreams.length} Healthy
            </span>
          </div>

          <div className="space-y-3">
            {upstreams.map((u) => {
              const isHealthy = u.health?.healthy !== false;
              return (
                <div
                  key={u.id}
                  className="p-3 bg-slate-900/60 rounded-lg border border-slate-800 flex items-center justify-between"
                >
                  <div className="flex items-center gap-3">
                    <span
                      className={`w-2.5 h-2.5 rounded-full ${
                        isHealthy ? 'bg-emerald-500 shadow-sm shadow-emerald-500/50' : 'bg-rose-500 animate-pulse'
                      }`}
                    />
                    <div>
                      <div className="font-semibold text-xs text-white flex items-center gap-2">
                        {u.name}
                        <code className="text-[10px] bg-slate-800 text-slate-400 px-1.5 py-0.5 rounded">
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
        <div className="fl-card p-6">
          <div className="flex items-center justify-between mb-4">
            <h3 className="text-sm font-bold text-white flex items-center gap-2">
              <Flame className="w-4 h-4 text-rose-500" />
              Injected Fault Distribution
            </h3>
            <span className="text-xs text-slate-400 font-mono">Real-Time Counters</span>
          </div>

          {stats?.fault_breakdown && Object.keys(stats.fault_breakdown).length > 0 ? (
            <div className="space-y-3">
              {Object.entries(stats.fault_breakdown).map(([fault, count]) => (
                <div key={fault} className="space-y-1">
                  <div className="flex justify-between text-xs font-mono">
                    <span className="text-slate-300 font-semibold uppercase">{fault.replace('_', ' ')}</span>
                    <span className="text-rose-400 font-bold">{count} injected</span>
                  </div>
                  <div className="w-full bg-slate-800 h-2 rounded-full overflow-hidden">
                    <div
                      className="bg-rose-500 h-full rounded-full"
                      style={{ width: `${Math.min((count / (stats.total_faults || 1)) * 100, 100)}%` }}
                    />
                  </div>
                </div>
              ))}
            </div>
          ) : (
            <div className="py-12 text-center text-slate-500 text-xs">
              No chaos faults recorded yet. Dispatch a test request in the Test Studio to simulate failures.
            </div>
          )}

          <div className="mt-6 pt-4 border-t border-slate-800 flex justify-between items-center">
            <span className="text-xs text-slate-400">Test resilience immediately:</span>
            <button
              onClick={() => onNavigate('test-studio')}
              className="fl-btn-primary text-xs py-1.5"
            >
              Open Chaos Test Studio
            </button>
          </div>
        </div>
      </div>
    </div>
  );
};
