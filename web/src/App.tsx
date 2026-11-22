import React, { useState, useEffect } from 'react';
import {
  Zap,
  Activity,
  Server,
  Flame,
  Terminal,
  Shield,
  RotateCw,
  ExternalLink,
  Lock,
} from 'lucide-react';
import { LiveStats, Upstream, Experiment, KillSwitchState, AuditLog } from './types';
import {
  fetchLiveStats,
  fetchKillSwitch,
  setKillSwitch,
  fetchUpstreams,
  fetchExperiments,
  fetchAuditLogs,
} from './api';
import { KillSwitchBar } from './components/KillSwitchBar';
import { OverviewTab } from './components/OverviewTab';
import { UpstreamsTab } from './components/UpstreamsTab';
import { ExperimentsTab } from './components/ExperimentsTab';
import { TestStudioTab } from './components/TestStudioTab';
import { AuditTab } from './components/AuditTab';

export const App: React.FC = () => {
  const [activeTab, setActiveTab] = useState<'overview' | 'upstreams' | 'experiments' | 'test-studio' | 'audit'>('overview');
  const [stats, setStats] = useState<LiveStats | null>(null);
  const [killSwitch, setKillSwitchState] = useState<KillSwitchState>({
    global_active: false,
    activated_by: '',
    reason: '',
  });
  const [upstreams, setUpstreams] = useState<Upstream[]>([]);
  const [experiments, setExperiments] = useState<Experiment[]>([]);
  const [auditLogs, setAuditLogs] = useState<AuditLog[]>([]);
  const [refreshing, setRefreshing] = useState(false);

  const loadData = async () => {
    try {
      const [s, ks, ups, exps] = await Promise.all([
        fetchLiveStats().catch(() => null),
        fetchKillSwitch().catch(() => ({ global_active: false, activated_by: '', reason: '' })),
        fetchUpstreams().catch(() => []),
        fetchExperiments().catch(() => []),
      ]);

      if (s) setStats(s);
      if (ks) setKillSwitchState(ks);
      setUpstreams(ups);
      setExperiments(exps);

      if (activeTab === 'audit') {
        const logs = await fetchAuditLogs(50, 0).catch(() => []);
        setAuditLogs(logs);
      }
    } catch (err) {
      console.error('Failed to poll FaultLine data:', err);
    }
  };

  useEffect(() => {
    loadData();
    // Poll telemetry every 3 seconds
    const interval = setInterval(loadData, 3000);
    return () => clearInterval(interval);
  }, [activeTab]);

  const handleToggleKillSwitch = async (active: boolean, reason: string) => {
    const updated = await setKillSwitch(active, reason);
    setKillSwitchState(updated);
    await loadData();
  };

  const handleManualRefresh = async () => {
    setRefreshing(true);
    await loadData();
    setRefreshing(false);
  };

  return (
    <div className="min-h-screen bg-[#070a12] text-slate-100 flex flex-col">
      {/* Top Emergency Kill Switch Bar */}
      <KillSwitchBar killSwitch={killSwitch} onToggle={handleToggleKillSwitch} />

      {/* Main Header */}
      <header className="bg-[#0b101d] border-b border-slate-800/80 px-6 py-4 sticky top-0 z-40 backdrop-blur-md">
        <div className="max-w-7xl mx-auto flex flex-wrap items-center justify-between gap-4">
          <div className="flex items-center gap-3">
            <div className="p-2.5 bg-gradient-to-tr from-rose-600 to-rose-400 rounded-xl text-white shadow-lg shadow-rose-950">
              <Zap className="w-5 h-5 fill-current" />
            </div>
            <div>
              <div className="flex items-center gap-2">
                <span className="font-extrabold text-lg text-white tracking-tight">FAULTLINE</span>
                <span className="fl-badge bg-rose-950/80 text-rose-400 border border-rose-800">
                  REVERSE PROXY
                </span>
                <span className="text-[10px] bg-slate-800 text-slate-400 px-1.5 py-0.5 rounded font-mono">
                  v1.0.0
                </span>
              </div>
              <p className="text-xs text-slate-400">
                Security-First Programmable Chaos Engineering Reverse Proxy
              </p>
            </div>
          </div>

          {/* Quick External Links */}
          <div className="flex items-center gap-3">
            <a
              href="/metrics"
              target="_blank"
              rel="noreferrer"
              className="text-xs text-slate-400 hover:text-white flex items-center gap-1 font-mono transition-colors"
            >
              /metrics <ExternalLink className="w-3 h-3" />
            </a>
            <span className="text-slate-700">|</span>
            <a
              href="/api/v1/health"
              target="_blank"
              rel="noreferrer"
              className="text-xs text-slate-400 hover:text-white flex items-center gap-1 font-mono transition-colors"
            >
              /health <ExternalLink className="w-3 h-3" />
            </a>
            <span className="text-slate-700">|</span>
            <button
              onClick={handleManualRefresh}
              className="p-1.5 text-slate-400 hover:text-white transition-colors cursor-pointer"
              title="Manual Refresh"
            >
              <RotateCw className={`w-4 h-4 ${refreshing ? 'animate-spin' : ''}`} />
            </button>
          </div>
        </div>
      </header>

      {/* Tab Navigation Strip */}
      <nav className="bg-[#090d16] border-b border-slate-800/60 px-6">
        <div className="max-w-7xl mx-auto flex items-center gap-1">
          <button
            onClick={() => setActiveTab('overview')}
            className={`py-3.5 px-4 text-xs font-semibold flex items-center gap-2 border-b-2 transition-all cursor-pointer ${
              activeTab === 'overview'
                ? 'border-rose-500 text-white bg-slate-900/40'
                : 'border-transparent text-slate-400 hover:text-slate-200'
            }`}
          >
            <Activity className="w-4 h-4" />
            Overview & Telemetry
          </button>

          <button
            onClick={() => setActiveTab('upstreams')}
            className={`py-3.5 px-4 text-xs font-semibold flex items-center gap-2 border-b-2 transition-all cursor-pointer ${
              activeTab === 'upstreams'
                ? 'border-rose-500 text-white bg-slate-900/40'
                : 'border-transparent text-slate-400 hover:text-slate-200'
            }`}
          >
            <Server className="w-4 h-4" />
            Upstream Registry ({upstreams.length})
          </button>

          <button
            onClick={() => setActiveTab('experiments')}
            className={`py-3.5 px-4 text-xs font-semibold flex items-center gap-2 border-b-2 transition-all cursor-pointer ${
              activeTab === 'experiments'
                ? 'border-rose-500 text-white bg-slate-900/40'
                : 'border-transparent text-slate-400 hover:text-slate-200'
            }`}
          >
            <Flame className="w-4 h-4" />
            Chaos Experiments ({experiments.length})
          </button>

          <button
            onClick={() => setActiveTab('test-studio')}
            className={`py-3.5 px-4 text-xs font-semibold flex items-center gap-2 border-b-2 transition-all cursor-pointer ${
              activeTab === 'test-studio'
                ? 'border-rose-500 text-white bg-slate-900/40'
                : 'border-transparent text-slate-400 hover:text-slate-200'
            }`}
          >
            <Terminal className="w-4 h-4" />
            Chaos Test Studio
          </button>

          <button
            onClick={() => setActiveTab('audit')}
            className={`py-3.5 px-4 text-xs font-semibold flex items-center gap-2 border-b-2 transition-all cursor-pointer ${
              activeTab === 'audit'
                ? 'border-rose-500 text-white bg-slate-900/40'
                : 'border-transparent text-slate-400 hover:text-slate-200'
            }`}
          >
            <Shield className="w-4 h-4" />
            Audit Ledger
          </button>
        </div>
      </nav>

      {/* Main Content View */}
      <main className="flex-1 max-w-7xl w-full mx-auto p-6">
        {activeTab === 'overview' && (
          <OverviewTab
            stats={stats}
            upstreams={upstreams}
            experiments={experiments}
            onNavigate={(tab) => setActiveTab(tab)}
          />
        )}

        {activeTab === 'upstreams' && (
          <UpstreamsTab upstreams={upstreams} onRefresh={loadData} />
        )}

        {activeTab === 'experiments' && (
          <ExperimentsTab
            experiments={experiments}
            upstreams={upstreams}
            onRefresh={loadData}
          />
        )}

        {activeTab === 'test-studio' && (
          <TestStudioTab upstreams={upstreams} />
        )}

        {activeTab === 'audit' && (
          <AuditTab logs={auditLogs} onRefresh={loadData} />
        )}
      </main>

      {/* Footer */}
      <footer className="border-t border-slate-900 py-4 px-6 text-center text-xs text-slate-500 font-mono">
        FaultLine Reverse Proxy — Production-Minded Chaos Engineering Architecture
      </footer>
    </div>
  );
};
