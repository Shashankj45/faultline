import React, { useState, useEffect } from 'react';
import {
  Shield,
  Key,
  Lock,
  CheckCircle,
  Eye,
  EyeOff,
  Server,
  Layers,
  Cpu,
  Radio,
  FileText,
  AlertTriangle,
  Plus,
  Trash2,
  Copy,
  Check,
} from 'lucide-react';
import { APIKey, Role } from '../types';
import {
  fetchAPIKeys,
  createAPIKey,
  revokeAPIKey,
  getActiveApiKey,
  setActiveApiKey,
} from '../api';

interface Props {
  onKeyChange?: () => void;
}

export const SecurityTab: React.FC<Props> = ({ onKeyChange }) => {
  const [keys, setKeys] = useState<APIKey[]>([]);
  const [loading, setLoading] = useState(false);
  const [sessionKey, setSessionKey] = useState<string>('');
  const [showKeyText, setShowKeyText] = useState(false);
  const [showCreateSection, setShowCreateSection] = useState(false);

  // Key creation state
  const [newKeyName, setNewKeyName] = useState('');
  const [newKeyRole, setNewKeyRole] = useState<Role>('operator');
  const [createdSecret, setCreatedSecret] = useState<string | null>(null);
  const [copied, setCopied] = useState(false);
  const [errorMsg, setErrorMsg] = useState('');

  const loadKeys = async () => {
    setLoading(true);
    try {
      const data = await fetchAPIKeys();
      setKeys(data || []);
    } catch {
      // Ignored in viewer mode
    } finally {
      setLoading(false);
    }
  };

  useEffect(() => {
    setSessionKey(getActiveApiKey());
    loadKeys();
  }, []);

  const handleSaveSessionKey = (k: string) => {
    setActiveApiKey(k);
    setSessionKey(k);
    if (onKeyChange) onKeyChange();
  };

  const handleClearSessionKey = () => {
    setActiveApiKey('');
    setSessionKey('');
    if (onKeyChange) onKeyChange();
  };

  const handleCreateKey = async (e: React.FormEvent) => {
    e.preventDefault();
    setErrorMsg('');
    try {
      const res = await createAPIKey(newKeyName.trim() || 'Operator Key', newKeyRole);
      setCreatedSecret(res.secret_key);
      setNewKeyName('');
      loadKeys();
    } catch (err: any) {
      setErrorMsg(err.message || 'Failed to create key');
    }
  };

  const handleRevoke = async (id: string, name: string) => {
    if (!confirm(`Revoke key "${name}"? Client requests with this key will be denied.`)) return;
    try {
      await revokeAPIKey(id);
      loadKeys();
    } catch (err: any) {
      alert(err.message || 'Failed to revoke');
    }
  };

  const copyToClipboard = (text: string) => {
    navigator.clipboard.writeText(text);
    setCopied(true);
    setTimeout(() => setCopied(false), 2000);
  };

  return (
    <div className="space-y-6">
      {/* Header */}
      <div>
        <h2 className="text-base font-bold text-white flex items-center gap-2">
          <Shield className="w-4 h-4 text-sky-400" />
          Security Architecture & Credential Protection
        </h2>
        <p className="text-xs text-slate-400 font-mono mt-0.5">
          FaultLine core security guarantees: Zero plaintext secret persistence, constant-time verification, and SSRF SafeDialer defense.
        </p>
      </div>

      {/* Security Guarantees Panel: How API Keys Are Kept Safe */}
      <div className="fl-card p-4 border-slate-800 bg-[#0d1424]">
        <div className="flex items-center gap-2 text-xs font-semibold text-white uppercase font-mono tracking-wider mb-3">
          <Lock className="w-4 h-4 text-emerald-400" />
          API Key Protection Mechanisms
        </div>

        <div className="grid grid-cols-1 md:grid-cols-3 gap-3 text-xs font-mono">
          <div className="p-3 bg-[#080d1a] border border-slate-800 rounded">
            <div className="text-emerald-400 font-semibold mb-1 flex items-center gap-1.5">
              <CheckCircle className="w-3.5 h-3.5" />
              SHA-256 One-Way Digest
            </div>
            <p className="text-[11px] text-slate-400 leading-relaxed">
              Raw API keys (<code className="text-slate-300">fl_live_...</code>) are hashed with SHA-256 upon generation. The database only ever stores the 64-character hex hash, never the secret key.
            </p>
          </div>

          <div className="p-3 bg-[#080d1a] border border-slate-800 rounded">
            <div className="text-sky-400 font-semibold mb-1 flex items-center gap-1.5">
              <CheckCircle className="w-3.5 h-3.5" />
              Constant-Time Verification
            </div>
            <p className="text-[11px] text-slate-400 leading-relaxed">
              Incoming headers are verified via <code className="text-slate-300">crypto/subtle.ConstantTimeCompare</code> to prevent timing attack side-channels from uncovering valid tokens.
            </p>
          </div>

          <div className="p-3 bg-[#080d1a] border border-slate-800 rounded">
            <div className="text-amber-400 font-semibold mb-1 flex items-center gap-1.5">
              <CheckCircle className="w-3.5 h-3.5" />
              Automatic Log Redaction
            </div>
            <p className="text-[11px] text-slate-400 leading-relaxed">
              The tamper-evident audit logger scans all event JSON payloads and deeply redacts <code className="text-slate-300">token</code>, <code className="text-slate-300">key_hash</code>, and <code className="text-slate-300">authorization</code> values.
            </p>
          </div>
        </div>
      </div>

      {/* Active Session API Key Configuration */}
      <div className="fl-card p-4">
        <div className="flex flex-col md:flex-row md:items-center justify-between gap-4">
          <div>
            <div className="flex items-center gap-2">
              <span className="text-xs font-semibold text-white font-mono">Active Client Session API Key</span>
              {sessionKey ? (
                <span className="fl-badge bg-emerald-950 text-emerald-400 border-emerald-800">
                  AUTHENTICATED
                </span>
              ) : (
                <span className="fl-badge bg-slate-800 text-slate-400 border-slate-700">
                  BOOTSTRAP MODE (NO KEY)
                </span>
              )}
            </div>
            <p className="text-[11px] text-slate-400 font-mono mt-1">
              If an API key is configured on the proxy, provide it here to sign control plane requests via <code className="text-slate-300">X-API-Key</code>.
            </p>
          </div>

          <div className="flex items-center gap-2">
            <div className="relative">
              <input
                type={showKeyText ? 'text' : 'password'}
                placeholder="fl_live_..."
                value={sessionKey}
                onChange={(e) => setSessionKey(e.target.value)}
                className="fl-input font-mono w-60 text-xs pr-8"
              />
              <button
                type="button"
                onClick={() => setShowKeyText(!showKeyText)}
                className="absolute right-2 top-2 text-slate-500 hover:text-slate-300 text-xs"
              >
                {showKeyText ? <EyeOff className="w-3.5 h-3.5" /> : <Eye className="w-3.5 h-3.5" />}
              </button>
            </div>
            <button
              onClick={() => handleSaveSessionKey(sessionKey)}
              className="fl-btn-secondary text-xs"
            >
              Save Key
            </button>
            {sessionKey && (
              <button
                onClick={handleClearSessionKey}
                className="text-xs text-slate-400 hover:text-rose-400 font-mono px-2 transition-colors"
              >
                Clear
              </button>
            )}
          </div>
        </div>
      </div>

      {/* Registered Keys Table (Collapsible) */}
      <div className="fl-card overflow-hidden">
        <div className="p-3 bg-[#0e1526] border-b border-slate-800 flex items-center justify-between">
          <div className="flex items-center gap-2 text-xs font-mono">
            <Key className="w-4 h-4 text-slate-400" />
            <span className="font-semibold text-white">Registered API Keys</span>
            <span className="text-slate-400">({keys.length})</span>
          </div>

          <button
            onClick={() => setShowCreateSection(!showCreateSection)}
            className="text-xs text-sky-400 hover:text-sky-300 font-mono flex items-center gap-1 cursor-pointer"
          >
            <Plus className="w-3.5 h-3.5" />
            {showCreateSection ? 'Close Key Provisioning' : 'Provision Key'}
          </button>
        </div>

        {/* Optional Provision Key Drawer */}
        {showCreateSection && (
          <div className="p-4 bg-[#080d19] border-b border-slate-800 space-y-3">
            {createdSecret ? (
              <div className="p-3 bg-emerald-950/70 border border-emerald-800 rounded space-y-2">
                <div className="text-xs text-emerald-300 font-semibold flex items-center gap-1.5 font-mono">
                  <Check className="w-3.5 h-3.5" /> Key Created — Store Securely:
                </div>
                <div className="flex items-center gap-2">
                  <input
                    type="text"
                    readOnly
                    value={createdSecret}
                    className="fl-input font-mono w-full text-xs bg-black text-rose-300 select-all"
                  />
                  <button
                    onClick={() => copyToClipboard(createdSecret)}
                    className="fl-btn-secondary text-xs shrink-0"
                  >
                    {copied ? <Check className="w-3.5 h-3.5 text-emerald-400" /> : <Copy className="w-3.5 h-3.5" />}
                  </button>
                  <button
                    onClick={() => {
                      handleSaveSessionKey(createdSecret);
                      setCreatedSecret(null);
                      setShowCreateSection(false);
                    }}
                    className="fl-btn-primary text-xs shrink-0"
                  >
                    Use in Session
                  </button>
                </div>
              </div>
            ) : (
              <form onSubmit={handleCreateKey} className="flex flex-wrap items-center gap-3">
                {errorMsg && <span className="text-xs text-rose-400 font-mono">{errorMsg}</span>}
                <input
                  type="text"
                  placeholder="Key description / service name"
                  value={newKeyName}
                  onChange={(e) => setNewKeyName(e.target.value)}
                  className="fl-input text-xs w-64"
                />
                <select
                  value={newKeyRole}
                  onChange={(e) => setNewKeyRole(e.target.value as Role)}
                  className="fl-input text-xs"
                >
                  <option value="operator">Operator (Trigger Chaos, KillSwitch)</option>
                  <option value="admin">Admin (Full Control Plane Access)</option>
                  <option value="viewer">Viewer (Read-Only Telemetry)</option>
                </select>
                <button type="submit" className="fl-btn-primary text-xs">
                  Create Key
                </button>
              </form>
            )}
          </div>
        )}

        {keys.length === 0 ? (
          <div className="p-6 text-center text-xs text-slate-500 font-mono">
            No API keys stored. Control plane operates in bootstrap mode.
          </div>
        ) : (
          <div className="overflow-x-auto">
            <table className="w-full text-left text-xs font-mono">
              <thead className="bg-[#0a0f1d] border-b border-slate-800 text-slate-400">
                <tr>
                  <th className="p-2.5">Name</th>
                  <th className="p-2.5">Key Prefix</th>
                  <th className="p-2.5">Role</th>
                  <th className="p-2.5">Status</th>
                  <th className="p-2.5">Created</th>
                  <th className="p-2.5">Last Used</th>
                  <th className="p-2.5 text-right">Actions</th>
                </tr>
              </thead>
              <tbody className="divide-y divide-slate-800/60">
                {keys.map((k) => (
                  <tr key={k.id} className="hover:bg-slate-900/40">
                    <td className="p-2.5 font-medium text-white">{k.name}</td>
                    <td className="p-2.5 text-sky-400">{k.key_prefix}****</td>
                    <td className="p-2.5">
                      <span className="fl-badge bg-slate-800 text-slate-300 border-slate-700 uppercase">
                        {k.role}
                      </span>
                    </td>
                    <td className="p-2.5">
                      {k.is_active ? (
                        <span className="text-emerald-400">ACTIVE</span>
                      ) : (
                        <span className="text-rose-400">REVOKED</span>
                      )}
                    </td>
                    <td className="p-2.5 text-slate-400 whitespace-nowrap">
                      {new Date(k.created_at).toLocaleDateString()}
                    </td>
                    <td className="p-2.5 text-slate-400 whitespace-nowrap">
                      {k.last_used_at ? new Date(k.last_used_at).toLocaleDateString() : 'Never'}
                    </td>
                    <td className="p-2.5 text-right">
                      <button
                        onClick={() => handleRevoke(k.id, k.name)}
                        className="text-xs text-rose-400 hover:text-rose-300 p-1"
                        title="Revoke Key"
                      >
                        <Trash2 className="w-3.5 h-3.5" />
                      </button>
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        )}
      </div>

      {/* Important Project Information: Systems Engineering Breakdown */}
      <div className="space-y-3">
        <h3 className="text-xs font-bold text-white uppercase font-mono tracking-wider flex items-center gap-2">
          <Server className="w-3.5 h-3.5 text-slate-400" />
          Systems Architecture & Security Specifications
        </h3>

        <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-3 gap-3">
          {/* Spec 1: SSRF Protection */}
          <div className="fl-card p-3.5 space-y-1.5">
            <div className="flex items-center gap-2 text-rose-400 text-xs font-semibold font-mono">
              <Lock className="w-3.5 h-3.5" />
              SSRF & Rebinding Defense
            </div>
            <p className="text-[11px] text-slate-300 leading-relaxed font-mono">
              Validated at the socket layer via <code className="text-rose-300">net.Dialer.Control</code> hook. Re-resolves host IP at connect time to block DNS rebinding TOCTOU attacks.
            </p>
            <ul className="text-[10px] text-slate-400 font-mono space-y-0.5 list-disc pl-4 pt-1">
              <li>Blocks cloud metadata: 169.254.169.254</li>
              <li>Blocks link-local: 169.254.0.0/16</li>
              <li>Loopback & private networks blocked by default</li>
            </ul>
          </div>

          {/* Spec 2: OpenTelemetry Tracing */}
          <div className="fl-card p-3.5 space-y-1.5">
            <div className="flex items-center gap-2 text-sky-400 text-xs font-semibold font-mono">
              <Radio className="w-3.5 h-3.5" />
              OpenTelemetry W3C Tracing
            </div>
            <p className="text-[11px] text-slate-300 leading-relaxed font-mono">
              Every proxy hop creates an OpenTelemetry span with W3C <code className="text-sky-300">traceparent</code> propagation forwarded downstream to upstreams.
            </p>
            <ul className="text-[10px] text-slate-400 font-mono space-y-0.5 list-disc pl-4 pt-1">
              <li>Span: FaultLine.ProxyIngress</li>
              <li>Semantic conventions for method, route, & status</li>
              <li>Chaos faults recorded as span events & attributes</li>
            </ul>
          </div>

          {/* Spec 3: Buffer Pool */}
          <div className="fl-card p-3.5 space-y-1.5">
            <div className="flex items-center gap-2 text-emerald-400 text-xs font-semibold font-mono">
              <Cpu className="w-3.5 h-3.5" />
              32KB Buffer Pool (sync.Pool)
            </div>
            <p className="text-[11px] text-slate-300 leading-relaxed font-mono">
              Integrated with <code className="text-emerald-300">httputil.BufferPool</code>. Reuses fixed 32KB byte slices during streaming to eliminate GC pressure.
            </p>
            <ul className="text-[10px] text-slate-400 font-mono space-y-0.5 list-disc pl-4 pt-1">
              <li>Zero-alloc body pipe between client and upstream</li>
              <li>Token-bucket throttled streams for bandwidth limits</li>
              <li>Emergency bypass short-circuit in &lt;1 microsecond</li>
            </ul>
          </div>

          {/* Spec 4: Dual Storage */}
          <div className="fl-card p-3.5 space-y-1.5">
            <div className="flex items-center gap-2 text-amber-400 text-xs font-semibold font-mono">
              <Layers className="w-3.5 h-3.5" />
              Dual Storage & Hot Cache
            </div>
            <p className="text-[11px] text-slate-300 leading-relaxed font-mono">
              Hot-path proxy routing checks an in-memory RWMutex cache. Persistence uses SQLite (WAL mode) or PostgreSQL.
            </p>
            <ul className="text-[10px] text-slate-400 font-mono space-y-0.5 list-disc pl-4 pt-1">
              <li>O(1) in-memory rule and kill switch checks</li>
              <li>Atomic cache reloads on rule creation or update</li>
              <li>SQLite WAL single-writer thread safety</li>
            </ul>
          </div>

          {/* Spec 5: Dual Routing */}
          <div className="fl-card p-3.5 space-y-1.5">
            <div className="flex items-center gap-2 text-indigo-400 text-xs font-semibold font-mono">
              <Server className="w-3.5 h-3.5" />
              Dual Routing Ingress
            </div>
            <p className="text-[11px] text-slate-300 leading-relaxed font-mono">
              Supports path-based prefix routing and header-based transparent routing for existing microservice meshes.
            </p>
            <ul className="text-[10px] text-slate-400 font-mono space-y-0.5 list-disc pl-4 pt-1">
              <li>Path route: /proxy/&#123;slug&#125;/*</li>
              <li>Header route: X-FaultLine-Upstream: slug</li>
              <li>Automatic Location header rewriting on redirects</li>
            </ul>
          </div>

          {/* Spec 6: Prometheus Scraper */}
          <div className="fl-card p-3.5 space-y-1.5">
            <div className="flex items-center gap-2 text-purple-400 text-xs font-semibold font-mono">
              <FileText className="w-3.5 h-3.5" />
              Prometheus Scraper & Stats
            </div>
            <p className="text-[11px] text-slate-300 leading-relaxed font-mono">
              Standard Prometheus exporter running at <code className="text-purple-300">/metrics</code> with custom sliding-window quantiles for p50, p95, and p99.
            </p>
            <ul className="text-[10px] text-slate-400 font-mono space-y-0.5 list-disc pl-4 pt-1">
              <li>faultline_http_requests_total</li>
              <li>faultline_faults_injected_total</li>
              <li>faultline_killswitch_active</li>
            </ul>
          </div>
        </div>
      </div>
    </div>
  );
};
