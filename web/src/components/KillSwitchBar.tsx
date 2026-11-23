import React, { useState } from 'react';
import { ShieldAlert, ShieldCheck, AlertTriangle, Power } from 'lucide-react';
import { KillSwitchState } from '../types';

interface Props {
  killSwitch: KillSwitchState;
  onToggle: (active: boolean, reason: string) => Promise<void>;
}

export const KillSwitchBar: React.FC<Props> = ({ killSwitch, onToggle }) => {
  const [showConfirm, setShowConfirm] = useState(false);
  const [reason, setReason] = useState('');
  const [loading, setLoading] = useState(false);

  const handleAction = async (targetActive: boolean) => {
    try {
      setLoading(true);
      await onToggle(targetActive, reason || 'Operator triggered emergency override via Web Console');
      setShowConfirm(false);
      setReason('');
    } finally {
      setLoading(false);
    }
  };

  if (killSwitch.global_active) {
    return (
      <div className="bg-[#450a0a] border-b border-rose-600 px-6 py-2.5">
        <div className="max-w-7xl mx-auto flex flex-wrap items-center justify-between gap-4">
          <div className="flex items-center gap-3">
            <div className="p-1.5 bg-rose-700 rounded text-white border border-rose-500">
              <ShieldAlert className="w-4 h-4" />
            </div>
            <div>
              <div className="flex items-center gap-2">
                <span className="font-bold text-rose-100 text-xs tracking-wider uppercase font-mono">
                  EMERGENCY KILL SWITCH ACTIVE
                </span>
                <span className="fl-badge bg-rose-900 text-rose-200 border-rose-500">
                  ALL CHAOS BYPASSED (100% PASSTHROUGH)
                </span>
              </div>
              <p className="text-[11px] text-rose-200/90 font-mono mt-0.5">
                Reason: "{killSwitch.reason}" • Triggered by: <span className="font-semibold">{killSwitch.activated_by}</span>
              </p>
            </div>
          </div>

          <button
            onClick={() => handleAction(false)}
            disabled={loading}
            className="fl-btn-primary bg-emerald-700 hover:bg-emerald-600 border-emerald-500 text-xs"
          >
            <ShieldCheck className="w-3.5 h-3.5" />
            {loading ? 'Deactivating...' : 'Resume Chaos Engine'}
          </button>
        </div>
      </div>
    );
  }

  return (
    <div className="bg-[#0b101c] border-b border-slate-800/80 px-6 py-2">
      <div className="max-w-7xl mx-auto flex items-center justify-between gap-4">
        <div className="flex items-center gap-2.5 text-xs text-slate-400 font-mono">
          <span className="inline-block w-2 h-2 rounded-full bg-emerald-500" />
          <span className="text-slate-300">Proxy Ingress Active</span>
          <span className="text-slate-700">|</span>
          <span className="text-slate-400">Chaos Rules Armed</span>
        </div>

        {showConfirm ? (
          <div className="flex items-center gap-2 bg-[#1b1f2e] p-1.5 px-3 rounded border border-rose-700">
            <input
              type="text"
              placeholder="Reason for emergency stop (e.g. Incident #1042)..."
              value={reason}
              onChange={(e) => setReason(e.target.value)}
              className="fl-input w-72 text-xs font-mono"
            />
            <button
              onClick={() => handleAction(true)}
              disabled={loading}
              className="fl-btn-danger text-xs"
            >
              <Power className="w-3.5 h-3.5" />
              {loading ? 'Disarming...' : 'Confirm Disarm'}
            </button>
            <button
              onClick={() => {
                setShowConfirm(false);
                setReason('');
              }}
              className="text-xs text-slate-400 hover:text-white px-2 py-1 font-mono transition-colors"
            >
              Cancel
            </button>
          </div>
        ) : (
          <button
            onClick={() => setShowConfirm(true)}
            className="fl-btn-danger text-[11px] py-1 px-3"
            title="Immediately short-circuits all fault injection and enables 100% passthrough"
          >
            <AlertTriangle className="w-3.5 h-3.5" />
            Emergency Kill Switch
          </button>
        )}
      </div>
    </div>
  );
};
