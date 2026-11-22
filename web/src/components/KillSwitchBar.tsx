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
      <div className="bg-rose-950/80 border-b border-rose-500/50 px-6 py-3 shadow-lg">
        <div className="max-w-7xl mx-auto flex flex-wrap items-center justify-between gap-4">
          <div className="flex items-center gap-3">
            <div className="p-2 bg-rose-600 rounded-lg text-white pulse-emergency shadow-lg shadow-rose-600/50">
              <ShieldAlert className="w-6 h-6" />
            </div>
            <div>
              <div className="flex items-center gap-2">
                <span className="font-extrabold text-rose-200 tracking-wide uppercase text-sm">
                  EMERGENCY KILL SWITCH ACTIVE
                </span>
                <span className="bg-rose-500/30 text-rose-300 text-xs px-2 py-0.5 rounded-full border border-rose-500/40">
                  ALL CHAOS BYPASSED
                </span>
              </div>
              <p className="text-xs text-rose-300/80 mt-0.5">
                Proxy is in 100% Passthrough mode. Reason: <span className="text-white italic">"{killSwitch.reason}"</span> by <span className="font-semibold text-rose-200">{killSwitch.activated_by}</span>
              </p>
            </div>
          </div>

          <button
            onClick={() => handleAction(false)}
            disabled={loading}
            className="bg-emerald-600 hover:bg-emerald-500 text-white font-semibold text-xs px-4 py-2 rounded-lg flex items-center gap-2 border border-emerald-400 shadow-md shadow-emerald-950 transition-all cursor-pointer"
          >
            <ShieldCheck className="w-4 h-4" />
            {loading ? 'Deactivating...' : 'DEACTIVATE KILL SWITCH (RESUME CHAOS)'}
          </button>
        </div>
      </div>
    );
  }

  return (
    <div className="bg-[#0b101d] border-b border-slate-800/80 px-6 py-2.5">
      <div className="max-w-7xl mx-auto flex items-center justify-between gap-4">
        <div className="flex items-center gap-2.5 text-xs text-slate-400">
          <span className="inline-block w-2.5 h-2.5 rounded-full bg-emerald-500 shadow-sm shadow-emerald-500/50 animate-pulse" />
          <span className="font-medium text-slate-300">Proxy Ingress Active</span>
          <span className="text-slate-600">|</span>
          <span className="text-slate-400">Kill Switch Ready</span>
        </div>

        {showConfirm ? (
          <div className="flex items-center gap-2 bg-rose-950/60 p-1.5 px-3 rounded-lg border border-rose-700/60">
            <input
              type="text"
              placeholder="Reason for emergency stop (e.g. Incident #1042)..."
              value={reason}
              onChange={(e) => setReason(e.target.value)}
              className="bg-black/60 border border-rose-500/40 rounded px-2.5 py-1 text-xs text-white placeholder-slate-500 w-72 focus:outline-none focus:border-rose-400"
            />
            <button
              onClick={() => handleAction(true)}
              disabled={loading}
              className="bg-rose-600 hover:bg-rose-500 text-white font-bold text-xs px-3 py-1 rounded border border-rose-400 cursor-pointer"
            >
              CONFIRM EMERGENCY STOP
            </button>
            <button
              onClick={() => setShowConfirm(false)}
              className="text-slate-400 hover:text-white text-xs px-2 cursor-pointer"
            >
              Cancel
            </button>
          </div>
        ) : (
          <button
            onClick={() => setShowConfirm(true)}
            className="group text-xs text-rose-400/90 hover:text-rose-200 bg-rose-950/40 hover:bg-rose-900/50 px-3 py-1.5 rounded-lg border border-rose-800/50 flex items-center gap-2 transition-all cursor-pointer"
          >
            <Power className="w-3.5 h-3.5 text-rose-500 group-hover:scale-110 transition-transform" />
            <span className="font-semibold tracking-wide">EMERGENCY KILL SWITCH</span>
          </button>
        )}
      </div>
    </div>
  );
};
