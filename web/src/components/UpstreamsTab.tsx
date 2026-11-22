import React, { useState } from 'react';
import {
  Server,
  Plus,
  Trash2,
  ShieldCheck,
  ShieldAlert,
  Activity,
  AlertTriangle,
  Globe,
  Lock,
} from 'lucide-react';
import { Upstream } from '../types';
import { createUpstream, deleteUpstream } from '../api';

interface Props {
  upstreams: Upstream[];
  onRefresh: () => void;
}

export const UpstreamsTab: React.FC<Props> = ({ upstreams, onRefresh }) => {
  const [showModal, setShowModal] = useState(false);
  const [loading, setLoading] = useState(false);
  const [errorMsg, setErrorMsg] = useState('');

  // Form State
  const [name, setName] = useState('');
  const [slug, setSlug] = useState('');
  const [targetUrl, setTargetUrl] = useState('');
  const [allowLocalhost, setAllowLocalhost] = useState(false);
  const [allowPrivate, setAllowPrivate] = useState(false);
  const [timeoutSec, setTimeoutSec] = useState(30);
  const [healthPath, setHealthPath] = useState('/healthz');

  const handleSubmit = async (e: React.FormEvent) => {
    e.preventDefault();
    setErrorMsg('');
    setLoading(true);

    try {
      await createUpstream({
        name,
        slug: slug.toLowerCase().trim(),
        target_url: targetUrl.trim(),
        allow_localhost: allowLocalhost,
        allow_private_network: allowPrivate,
        timeout_seconds: Number(timeoutSec),
        health_check_path: healthPath,
        is_active: true,
      });

      setShowModal(false);
      setName('');
      setSlug('');
      setTargetUrl('');
      onRefresh();
    } catch (err: any) {
      setErrorMsg(err.message || 'Failed to register upstream');
    } finally {
      setLoading(false);
    }
  };

  const handleDelete = async (id: string, name: string) => {
    if (!confirm(`Are you sure you want to delete upstream "${name}"?`)) return;
    try {
      await deleteUpstream(id);
      onRefresh();
    } catch (err: any) {
      alert(err.message || 'Failed to delete upstream');
    }
  };

  return (
    <div className="space-y-6">
      {/* Header */}
      <div className="flex flex-wrap items-center justify-between gap-4">
        <div>
          <h2 className="text-xl font-bold text-white flex items-center gap-2">
            <Server className="w-5 h-5 text-sky-400" />
            Registered Upstream Services
          </h2>
          <p className="text-xs text-slate-400 mt-1">
            FaultLine forwards traffic exclusively to explicitly registered, SSRF-validated upstreams.
          </p>
        </div>

        <button
          onClick={() => setShowModal(true)}
          className="fl-btn-primary text-xs"
        >
          <Plus className="w-4 h-4" />
          Register Upstream
        </button>
      </div>

      {/* Upstreams Grid */}
      <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-3 gap-5">
        {upstreams.map((u) => {
          const isHealthy = u.health?.healthy !== false;
          return (
            <div
              key={u.id}
              className="fl-card p-5 flex flex-col justify-between hover:border-slate-700 transition-all group"
            >
              <div>
                {/* Header */}
                <div className="flex items-start justify-between gap-2 mb-3">
                  <div className="flex items-center gap-2.5">
                    <span
                      className={`w-3 h-3 rounded-full ${
                        isHealthy
                          ? 'bg-emerald-500 shadow-md shadow-emerald-500/50'
                          : 'bg-rose-500 animate-pulse'
                      }`}
                    />
                    <div>
                      <h3 className="font-bold text-sm text-white">{u.name}</h3>
                      <div className="font-mono text-xs text-rose-400 mt-0.5">
                        /proxy/{u.slug}/*
                      </div>
                    </div>
                  </div>

                  <button
                    onClick={() => handleDelete(u.id, u.name)}
                    className="text-slate-600 hover:text-rose-400 transition-colors p-1"
                    title="Delete Upstream"
                  >
                    <Trash2 className="w-4 h-4" />
                  </button>
                </div>

                {/* Target URL */}
                <div className="bg-slate-900/90 rounded-lg p-2.5 border border-slate-800/80 font-mono text-xs text-slate-300 break-all mb-4">
                  <div className="text-[10px] text-slate-500 uppercase tracking-wider mb-0.5">Target URL</div>
                  {u.target_url}
                </div>

                {/* Security & SSRF Policy Badges */}
                <div className="space-y-1.5 mb-4">
                  <div className="text-[11px] font-semibold text-slate-400 uppercase tracking-wider">
                    SSRF Security Policy
                  </div>
                  <div className="flex flex-wrap gap-1.5">
                    <span className="fl-badge bg-sky-950 text-sky-400 border border-sky-800/50">
                      <Lock className="w-3 h-3 mr-0.5" /> DNS Rebind Protected
                    </span>
                    {u.allow_localhost ? (
                      <span className="fl-badge bg-amber-950 text-amber-400 border border-amber-800/50">
                        Loopback Allowed
                      </span>
                    ) : (
                      <span className="fl-badge bg-slate-900 text-slate-400 border border-slate-800">
                        Loopback Blocked
                      </span>
                    )}
                    {u.allow_private_network ? (
                      <span className="fl-badge bg-amber-950 text-amber-400 border border-amber-800/50">
                        Private RFC1918 Allowed
                      </span>
                    ) : (
                      <span className="fl-badge bg-slate-900 text-slate-400 border border-slate-800">
                        Private RFC1918 Blocked
                      </span>
                    )}
                  </div>
                </div>
              </div>

              {/* Health Telemetry Footer */}
              <div className="pt-3 border-t border-slate-800/80 flex items-center justify-between text-xs font-mono text-slate-400">
                <div className="flex items-center gap-1.5">
                  <Activity className="w-3.5 h-3.5 text-emerald-400" />
                  <span>Probe: {u.health_check_path}</span>
                </div>
                <div className="text-white font-semibold">
                  {u.health?.latency_ms ?? 0}ms
                </div>
              </div>
            </div>
          );
        })}
      </div>

      {/* Register Modal */}
      {showModal && (
        <div className="fixed inset-0 bg-black/80 backdrop-blur-sm z-50 flex items-center justify-center p-4">
          <div className="fl-card w-full max-w-lg p-6 relative border-slate-700 shadow-2xl">
            <h3 className="text-base font-bold text-white mb-2 flex items-center gap-2">
              <Plus className="w-5 h-5 text-rose-500" />
              Register New Upstream Service
            </h3>
            <p className="text-xs text-slate-400 mb-5">
              The reverse proxy will route requests matching <code className="text-rose-400">/proxy/{'{slug}'}/*</code> to this target.
            </p>

            {errorMsg && (
              <div className="mb-4 p-3 bg-rose-950/80 border border-rose-600 rounded-lg text-xs text-rose-200 flex items-center gap-2">
                <AlertTriangle className="w-4 h-4 text-rose-400 shrink-0" />
                <span>{errorMsg}</span>
              </div>
            )}

            <form onSubmit={handleSubmit} className="space-y-4">
              <div>
                <label className="block text-xs font-semibold text-slate-300 mb-1">Service Display Name</label>
                <input
                  type="text"
                  required
                  placeholder="e.g. Stripe Payment Gateway"
                  value={name}
                  onChange={(e) => setName(e.target.value)}
                  className="fl-input w-full"
                />
              </div>

              <div className="grid grid-cols-2 gap-3">
                <div>
                  <label className="block text-xs font-semibold text-slate-300 mb-1">Routing Slug</label>
                  <input
                    type="text"
                    required
                    placeholder="e.g. stripe-gateway"
                    value={slug}
                    onChange={(e) => setSlug(e.target.value)}
                    className="fl-input w-full font-mono text-xs"
                  />
                </div>
                <div>
                  <label className="block text-xs font-semibold text-slate-300 mb-1">Timeout (seconds)</label>
                  <input
                    type="number"
                    min="1"
                    max="300"
                    value={timeoutSec}
                    onChange={(e) => setTimeoutSec(Number(e.target.value))}
                    className="fl-input w-full font-mono text-xs"
                  />
                </div>
              </div>

              <div>
                <label className="block text-xs font-semibold text-slate-300 mb-1">Target Base URL</label>
                <input
                  type="url"
                  required
                  placeholder="e.g. https://api.stripe.com or http://10.0.1.20:8080"
                  value={targetUrl}
                  onChange={(e) => setTargetUrl(e.target.value)}
                  className="fl-input w-full font-mono text-xs"
                />
              </div>

              <div>
                <label className="block text-xs font-semibold text-slate-300 mb-1">Health Check Probe Path</label>
                <input
                  type="text"
                  placeholder="/healthz"
                  value={healthPath}
                  onChange={(e) => setHealthPath(e.target.value)}
                  className="fl-input w-full font-mono text-xs"
                />
              </div>

              {/* SSRF Explicit Override Toggles */}
              <div className="p-3 bg-slate-900/80 rounded-lg border border-slate-800 space-y-2">
                <div className="text-xs font-bold text-amber-400 flex items-center gap-1.5">
                  <ShieldAlert className="w-3.5 h-3.5" />
                  SSRF Protection Policies (Safe by Default)
                </div>

                <label className="flex items-center gap-2 text-xs text-slate-300 cursor-pointer">
                  <input
                    type="checkbox"
                    checked={allowLocalhost}
                    onChange={(e) => setAllowLocalhost(e.target.checked)}
                    className="rounded border-slate-700 bg-slate-950 text-rose-500 focus:ring-0"
                  />
                  <span>Allow Localhost / Loopback (<code className="text-slate-400">127.0.0.1, ::1</code>)</span>
                </label>

                <label className="flex items-center gap-2 text-xs text-slate-300 cursor-pointer">
                  <input
                    type="checkbox"
                    checked={allowPrivate}
                    onChange={(e) => setAllowPrivate(e.target.checked)}
                    className="rounded border-slate-700 bg-slate-950 text-rose-500 focus:ring-0"
                  />
                  <span>Allow Private RFC1918 (<code className="text-slate-400">10.x, 172.16.x, 192.168.x</code>)</span>
                </label>
              </div>

              <div className="pt-2 flex justify-end gap-3">
                <button
                  type="button"
                  onClick={() => setShowModal(false)}
                  className="fl-btn-secondary text-xs"
                >
                  Cancel
                </button>
                <button
                  type="submit"
                  disabled={loading}
                  className="fl-btn-primary text-xs"
                >
                  {loading ? 'Validating Target...' : 'Validate & Register'}
                </button>
              </div>
            </form>
          </div>
        </div>
      )}
    </div>
  );
};
