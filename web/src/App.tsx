import React, { useState, useEffect } from 'react';
import {
  Activity,
  Server,
  Flame,
  Terminal,
  Shield,
  RotateCw,
  ExternalLink,
  Key,
  ShieldAlert,
} from 'lucide-react';
import { LiveStats, Upstream, Experiment, KillSwitchState, AuditLog } from './types';
import {
  fetchLiveStats,
  fetchKillSwitch,
  setKillSwitch,
  fetchUpstreams,
  fetchExperiments,
  fetchAuditLogs,
  getActiveApiKey,
} from './api';
import { KillSwitchBar } from './components/KillSwitchBar';
import { OverviewTab } from './components/OverviewTab';
import { UpstreamsTab } from './components/UpstreamsTab';
import { ExperimentsTab } from './components/ExperimentsTab';
import { TestStudioTab } from './components/TestStudioTab';
import { AuditTab } from './components/AuditTab';
import { SecurityTab } from './components/SecurityTab';

export const App: React.FC = () => {
  const [activeTab, setActiveTab] = useState<
    'overview' | 'upstreams' | 'experiments' | 'test-studio' | 'security' | 'audit'
  >('overview');
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
  const [sessionKey, setSessionKey] = useState<string>(getActiveApiKey());

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
      console.error('Failed to poll FaultLine telemetry:', err);
    }
  };

  useEffect(() => {
    loadData();
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
    setSessionKey(getActiveApiKey());
    setRefreshing(false);
  };

  return (
    <div className="min-h-screen bg-[#0b0f17] text-slate-200 flex flex-col font-sans">
      {/* Top Emergency Kill Switch Bar */}
      <KillSwitchBar killSwitch={killSwitch} onToggle={handleToggleKillSwitch} />

      {/* Main Engineering Header */}
      <header className="bg-[#0e1422] border-b border-slate-800 px-6 py-3 sticky top-0 z-40">
        <div className="max-w-7xl mx-auto flex flex-wrap items-center justify-between gap-4">
          <div className="flex items-center gap-3">
            <div className="w-8 h-8 rounded bg-rose-600 flex items-center justify-center text-white font-mono font-bold text-sm tracking-tighter border border-rose-500">
              FL
            </div>
            <div>
              <div className="flex items-center gap-2">
                <span className="font-bold text-base text-white tracking-tight">FAULTLINE</span>
                <span className="fl-badge bg-[#182238] text-slate-300 border-slate-700">
                  REVERSE PROXY
                </span>
                <span className="text-[10px] bg-slate-800 text-slate-400 px-1 py-0.5 rounded font-mono">
                  v1.2.0
                </span>
              </div>
              <p className="text-[11px] text-slate-400 font-mono mt-0.5">
                Programmable HTTP Chaos Reverse Proxy • SSRF Protected • OTel Enabled
              </p>
            </div>
          </div>

          {/* Session Key Status & Endpoints */}
          <div className="flex items-center gap-3 text-xs font-mono">
            {/* Active Key Indicator */}
            <button
              onClick={() => setActiveTab('security')}
              className="flex items-center gap-1.5 px-2.5 py-1 rounded bg-[#131b2e] border border-slate-700/80 hover:border-slate-600 transition-colors cursor-pointer text-slate-300"
              title="Click to manage API Keys and console credentials"
            >
              <Key className="w-3.5 h-3.5 text-sky-400" />
              <span>
                {sessionKey ? `Key: ${sessionKey.substring(0, 11)}...` : 'Auth: Bootstrap Mode'}
              </span>
            </button>

            <span className="text-slate-700">|</span>

            <a
              href="/metrics"
              target="_blank"
              rel="noreferrer"
              className="text-slate-400 hover:text-white flex items-center gap-1 transition-colors"
            >
              /metrics <ExternalLink className="w-3 h-3" />
            </a>
            <span className="text-slate-700">|</span>
            <a
              href="/api/v1/health"
              target="_blank"
              rel="noreferrer"
              className="text-slate-400 hover:text-white flex items-center gap-1 transition-colors"
            >
              /health <ExternalLink className="w-3 h-3" />
            </a>
            <span className="text-slate-700">|</span>
            <button
              onClick={handleManualRefresh}
              className="p-1 text-slate-400 hover:text-white transition-colors cursor-pointer"
              title="Manual Telemetry Poll"
            >
              <RotateCw className={`w-3.5 h-3.5 ${refreshing ? 'animate-spin' : ''}`} />
            </button>
          </div>
        </div>
      </header>

      {/* Tab Navigation Strip */}
      <nav className="bg-[#0c121e] border-b border-slate-800 px-6">
        <div className="max-w-7xl mx-auto flex items-center gap-1 overflow-x-auto">
          <button
            onClick={() => setActiveTab('overview')}
            className={`py-3 px-3.5 text-xs font-medium flex items-center gap-2 border-b-2 transition-colors cursor-pointer whitespace-nowrap ${
              activeTab === 'overview'
                ? 'border-rose-500 text-white bg-[#131b2e]'
                : 'border-transparent text-slate-400 hover:text-slate-200'
            }`}
          >
            <Activity className="w-3.5 h-3.5 text-sky-400" />
            Overview & Telemetry
          </button>

          <button
            onClick={() => setActiveTab('upstreams')}
            className={`py-3 px-3.5 text-xs font-medium flex items-center gap-2 border-b-2 transition-colors cursor-pointer whitespace-nowrap ${
              activeTab === 'upstreams'
                ? 'border-rose-500 text-white bg-[#131b2e]'
                : 'border-transparent text-slate-400 hover:text-slate-200'
            }`}
          >
            <Server className="w-3.5 h-3.5 text-emerald-400" />
            Upstream Registry ({upstreams.length})
          </button>

          <button
            onClick={() => setActiveTab('experiments')}
            className={`py-3 px-3.5 text-xs font-medium flex items-center gap-2 border-b-2 transition-colors cursor-pointer whitespace-nowrap ${
              activeTab === 'experiments'
                ? 'border-rose-500 text-white bg-[#131b2e]'
                : 'border-transparent text-slate-400 hover:text-slate-200'
            }`}
          >
            <Flame className="w-3.5 h-3.5 text-rose-500" />
            Chaos Experiments ({experiments.length})
          </button>

          <button
            onClick={() => setActiveTab('test-studio')}
            className={`py-3 px-3.5 text-xs font-medium flex items-center gap-2 border-b-2 transition-colors cursor-pointer whitespace-nowrap ${
              activeTab === 'test-studio'
                ? 'border-rose-500 text-white bg-[#131b2e]'
                : 'border-transparent text-slate-400 hover:text-slate-200'
            }`}
          >
            <Terminal className="w-3.5 h-3.5 text-amber-400" />
            Chaos Test Studio
          </button>

          <button
            onClick={() => setActiveTab('security')}
            className={`py-3 px-3.5 text-xs font-medium flex items-center gap-2 border-b-2 transition-colors cursor-pointer whitespace-nowrap ${
              activeTab === 'security'
                ? 'border-rose-500 text-white bg-[#131b2e]'
                : 'border-transparent text-slate-400 hover:text-slate-200'
            }`}
          >
            <Key className="w-3.5 h-3.5 text-sky-400" />
            Security & API Keys
          </button>

          <button
            onClick={() => setActiveTab('audit')}
            className={`py-3 px-3.5 text-xs font-medium flex items-center gap-2 border-b-2 transition-colors cursor-pointer whitespace-nowrap ${
              activeTab === 'audit'
                ? 'border-rose-500 text-white bg-[#131b2e]'
                : 'border-transparent text-slate-400 hover:text-slate-200'
            }`}
          >
            <Shield className="w-3.5 h-3.5 text-slate-400" />
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

        {activeTab === 'security' && (
          <SecurityTab onKeyChange={() => setSessionKey(getActiveApiKey())} />
        )}

        {activeTab === 'audit' && (
          <AuditTab logs={auditLogs} onRefresh={loadData} />
        )}
      </main>

      {/* Footer */}
      <footer className="border-t border-slate-800/80 py-3 px-6 text-center text-xs text-slate-500 font-mono bg-[#090d15]">
        FaultLine Reverse Proxy — Production Systems Engineering Architecture
      </footer>
    </div>
  );
};
