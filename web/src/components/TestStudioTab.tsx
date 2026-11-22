import React, { useState } from 'react';
import {
  Send,
  Play,
  RotateCw,
  Flame,
  CheckCircle,
  AlertTriangle,
  Clock,
  Layers,
  Sparkles,
  Terminal,
} from 'lucide-react';
import { Upstream, TestStudioResult } from '../types';
import { dispatchTestStudio } from '../api';

interface Props {
  upstreams: Upstream[];
}

export const TestStudioTab: React.FC<Props> = ({ upstreams }) => {
  const [selectedSlug, setSelectedSlug] = useState<string>(upstreams[0]?.slug || 'payments');
  const [method, setMethod] = useState<'GET' | 'POST' | 'PUT' | 'DELETE'>('POST');
  const [path, setPath] = useState<string>('/v1/charges');
  const [payload, setPayload] = useState<string>(
    JSON.stringify({ amount: 149.99, currency: 'USD', customer_id: 'cust_live_881' }, null, 2)
  );
  const [loading, setLoading] = useState(false);
  const [result, setResult] = useState<TestStudioResult | null>(null);

  const handleDispatch = async () => {
    setLoading(true);
    setResult(null);
    try {
      const res = await dispatchTestStudio({
        upstream_slug: selectedSlug,
        method,
        path,
        headers: {
          'Content-Type': 'application/json',
          'X-Client-Version': '2.0.0',
        },
        payload: method === 'GET' ? '' : payload,
      });
      setResult(res);
    } catch (err: any) {
      alert(err.message || 'Dispatch failed');
    } finally {
      setLoading(false);
    }
  };

  const applyPreset = (preset: 'payment' | 'order' | 'inventory') => {
    if (preset === 'payment') {
      setSelectedSlug('payments');
      setMethod('POST');
      setPath('/v1/charges');
      setPayload(JSON.stringify({ amount: 99.50, currency: 'USD' }, null, 2));
    } else if (preset === 'order') {
      setSelectedSlug('orders');
      setMethod('POST');
      setPath('/v1/orders');
      setPayload(JSON.stringify({ item_id: 'sku_ultra_9', quantity: 2 }, null, 2));
    } else if (preset === 'inventory') {
      setSelectedSlug('inventory');
      setMethod('GET');
      setPath('/v1/stock/SKU-TURBO-77');
      setPayload('');
    }
  };

  return (
    <div className="space-y-6">
      {/* Header */}
      <div>
        <h2 className="text-xl font-bold text-white flex items-center gap-2">
          <Terminal className="w-5 h-5 text-rose-500" />
          Chaos Test Studio & Live Dispatcher
        </h2>
        <p className="text-xs text-slate-400 mt-1">
          Send live requests through FaultLine's reverse proxy ingress to observe latency, error injections, and header mutations.
        </p>
      </div>

      {/* Quick Presets */}
      <div className="flex items-center gap-2 flex-wrap">
        <span className="text-xs text-slate-400 flex items-center gap-1 font-semibold">
          <Sparkles className="w-3.5 h-3.5 text-amber-400" /> Quick Presets:
        </span>
        <button
          onClick={() => applyPreset('payment')}
          className="text-xs bg-slate-900 hover:bg-slate-800 text-slate-300 px-3 py-1.5 rounded-lg border border-slate-800 cursor-pointer transition-colors"
        >
          Payment Charge (/proxy/payments/v1/charges)
        </button>
        <button
          onClick={() => applyPreset('order')}
          className="text-xs bg-slate-900 hover:bg-slate-800 text-slate-300 px-3 py-1.5 rounded-lg border border-slate-800 cursor-pointer transition-colors"
        >
          Create Order (/proxy/orders/v1/orders)
        </button>
        <button
          onClick={() => applyPreset('inventory')}
          className="text-xs bg-slate-900 hover:bg-slate-800 text-slate-300 px-3 py-1.5 rounded-lg border border-slate-800 cursor-pointer transition-colors"
        >
          Stock Lookup (/proxy/inventory/v1/stock/...)
        </button>
      </div>

      <div className="grid grid-cols-1 lg:grid-cols-2 gap-6">
        {/* Request Builder */}
        <div className="fl-card p-6 space-y-4">
          <h3 className="text-sm font-bold text-white flex items-center gap-2">
            <Send className="w-4 h-4 text-sky-400" />
            Request Parameters
          </h3>

          <div className="grid grid-cols-3 gap-3">
            <div>
              <label className="block text-xs font-semibold text-slate-400 mb-1">Target Upstream</label>
              <select
                value={selectedSlug}
                onChange={(e) => setSelectedSlug(e.target.value)}
                className="fl-input w-full font-mono text-xs"
              >
                {upstreams.map((u) => (
                  <option key={u.id} value={u.slug}>
                    {u.name} ({u.slug})
                  </option>
                ))}
              </select>
            </div>

            <div>
              <label className="block text-xs font-semibold text-slate-400 mb-1">HTTP Method</label>
              <select
                value={method}
                onChange={(e) => setMethod(e.target.value as any)}
                className="fl-input w-full font-mono text-xs"
              >
                <option value="GET">GET</option>
                <option value="POST">POST</option>
                <option value="PUT">PUT</option>
                <option value="DELETE">DELETE</option>
              </select>
            </div>

            <div>
              <label className="block text-xs font-semibold text-slate-400 mb-1">Relative Path</label>
              <input
                type="text"
                value={path}
                onChange={(e) => setPath(e.target.value)}
                className="fl-input w-full font-mono text-xs"
              />
            </div>
          </div>

          <div>
            <div className="text-[11px] font-mono text-slate-400 mb-1">
              Effective Proxy URL:{' '}
              <span className="text-rose-400 font-bold">
                http://localhost:8080/proxy/{selectedSlug}{path.startsWith('/') ? path : '/' + path}
              </span>
            </div>
          </div>

          {method !== 'GET' && (
            <div>
              <label className="block text-xs font-semibold text-slate-400 mb-1">JSON Payload</label>
              <textarea
                rows={7}
                value={payload}
                onChange={(e) => setPayload(e.target.value)}
                className="fl-input w-full font-mono text-xs"
              />
            </div>
          )}

          <button
            onClick={handleDispatch}
            disabled={loading}
            className="fl-btn-primary w-full justify-center text-xs py-2.5 shadow-lg shadow-rose-950"
          >
            {loading ? (
              <span className="flex items-center gap-2">
                <RotateCw className="w-4 h-4 animate-spin" /> Transmitting Request...
              </span>
            ) : (
              <span className="flex items-center gap-2">
                <Play className="w-4 h-4 fill-current" /> Dispatch Request Through Proxy
              </span>
            )}
          </button>
        </div>

        {/* Live Response Inspector */}
        <div className="fl-card p-6 flex flex-col justify-between">
          <div>
            <div className="flex items-center justify-between mb-4">
              <h3 className="text-sm font-bold text-white flex items-center gap-2">
                <Layers className="w-4 h-4 text-emerald-400" />
                Proxy Response Inspector
              </h3>
              {result && (
                <div className="flex items-center gap-2">
                  <span
                    className={`fl-badge font-mono text-xs ${
                      result.status_code >= 200 && result.status_code < 400
                        ? 'bg-emerald-950 text-emerald-400 border border-emerald-800'
                        : 'bg-rose-950 text-rose-400 border border-rose-800'
                    }`}
                  >
                    HTTP {result.status_code}
                  </span>
                  <span className="fl-badge bg-slate-900 text-amber-400 border border-slate-800 font-mono text-xs">
                    <Clock className="w-3 h-3 mr-1 inline" /> {result.duration_ms}ms
                  </span>
                </div>
              )}
            </div>

            {result ? (
              <div className="space-y-4">
                {/* Chaos Injection Banner */}
                {result.is_injected ? (
                  <div className="p-3 bg-rose-950/70 border border-rose-600/70 rounded-lg flex items-center justify-between">
                    <div className="flex items-center gap-2 text-xs text-rose-200">
                      <Flame className="w-4 h-4 text-rose-500 shrink-0" />
                      <div>
                        <div className="font-bold uppercase tracking-wider text-[11px] text-rose-400">
                          CHAOS FAULT INJECTED
                        </div>
                        <div>
                          Fault Type: <strong className="text-white uppercase">{result.injected_fault || 'Chaos Rule'}</strong>
                        </div>
                      </div>
                    </div>
                    <span className="text-xs bg-rose-500/20 text-rose-300 border border-rose-500/30 px-2 py-0.5 rounded font-mono">
                      X-FaultLine-Injected
                    </span>
                  </div>
                ) : (
                  <div className="p-3 bg-emerald-950/50 border border-emerald-800/60 rounded-lg flex items-center gap-2 text-xs text-emerald-300">
                    <CheckCircle className="w-4 h-4 text-emerald-400 shrink-0" />
                    <span>Clean Passthrough: Upstream responded directly without injected disruption.</span>
                  </div>
                )}

                {/* Response Body */}
                <div>
                  <div className="text-[11px] font-bold text-slate-400 uppercase tracking-wider mb-1">
                    Response Payload
                  </div>
                  <pre className="p-3 bg-slate-950 rounded-lg border border-slate-800 text-xs text-sky-300 font-mono overflow-x-auto max-h-56">
                    {result.response_body || '<empty body>'}
                  </pre>
                </div>

                {/* Response Headers */}
                <div>
                  <div className="text-[11px] font-bold text-slate-400 uppercase tracking-wider mb-1">
                    Response Headers ({Object.keys(result.response_headers).length})
                  </div>
                  <div className="p-2.5 bg-slate-950/80 rounded-lg border border-slate-800/80 text-[11px] font-mono max-h-36 overflow-y-auto divide-y divide-slate-800/40">
                    {Object.entries(result.response_headers).map(([k, v]) => (
                      <div key={k} className="py-1 flex items-baseline justify-between gap-2">
                        <span className="text-slate-400 font-semibold">{k}:</span>
                        <span className="text-slate-200 text-right truncate max-w-xs">{v}</span>
                      </div>
                    ))}
                  </div>
                </div>
              </div>
            ) : (
              <div className="py-24 text-center text-slate-500 text-xs">
                Ready to dispatch. Select an upstream and click "Dispatch Request Through Proxy".
              </div>
            )}
          </div>
        </div>
      </div>
    </div>
  );
};
