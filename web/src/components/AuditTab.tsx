import React from 'react';
import { Shield, RotateCw, User, Calendar, Terminal } from 'lucide-react';
import { AuditLog } from '../types';

interface Props {
  logs: AuditLog[];
  onRefresh: () => void;
}

export const AuditTab: React.FC<Props> = ({ logs, onRefresh }) => {
  const getActionBadge = (action: string) => {
    if (action.includes('KILL_SWITCH')) {
      return <span className="fl-badge bg-rose-950 text-rose-400 border border-rose-800">{action}</span>;
    }
    if (action.includes('CREATE')) {
      return <span className="fl-badge bg-emerald-950 text-emerald-400 border border-emerald-800">{action}</span>;
    }
    if (action.includes('DELETE') || action.includes('REVOKE')) {
      return <span className="fl-badge bg-red-950 text-red-400 border border-red-800">{action}</span>;
    }
    return <span className="fl-badge bg-sky-950 text-sky-400 border border-sky-800">{action}</span>;
  };

  return (
    <div className="space-y-6">
      {/* Header */}
      <div className="flex flex-wrap items-center justify-between gap-4">
        <div>
          <h2 className="text-xl font-bold text-white flex items-center gap-2">
            <Shield className="w-5 h-5 text-sky-400" />
            Tamper-Evident Audit Log Explorer
          </h2>
          <p className="text-xs text-slate-400 mt-1">
            Append-only historical ledger tracking all administrative actions, killswitch triggers, and experiment modifications.
          </p>
        </div>

        <button onClick={onRefresh} className="fl-btn-secondary text-xs">
          <RotateCw className="w-3.5 h-3.5" /> Refresh Log
        </button>
      </div>

      {/* Logs Table */}
      <div className="fl-card overflow-hidden">
        <div className="overflow-x-auto">
          <table className="w-full text-left text-xs font-mono">
            <thead className="bg-slate-900/60 border-b border-slate-800 text-slate-400">
              <tr>
                <th className="p-3.5">Timestamp (UTC)</th>
                <th className="p-3.5">Action</th>
                <th className="p-3.5">Operator</th>
                <th className="p-3.5">Resource Target</th>
                <th className="p-3.5">Client IP</th>
                <th className="p-3.5">Details</th>
              </tr>
            </thead>
            <tbody className="divide-y divide-slate-800/60">
              {logs.map((log) => (
                <tr key={log.id} className="hover:bg-slate-900/40 transition-colors">
                  <td className="p-3.5 text-slate-400 whitespace-nowrap">
                    {new Date(log.timestamp).toLocaleString()}
                  </td>
                  <td className="p-3.5">
                    {getActionBadge(log.action)}
                  </td>
                  <td className="p-3.5 font-bold text-white flex items-center gap-1.5">
                    <User className="w-3 h-3 text-slate-400" />
                    {log.user_id}
                  </td>
                  <td className="p-3.5 text-slate-300">
                    <span className="text-slate-500 uppercase">{log.resource_type}:</span> {log.resource_id}
                  </td>
                  <td className="p-3.5 text-slate-400">
                    {log.client_ip || 'local'}
                  </td>
                  <td className="p-3.5 text-slate-400 max-w-sm truncate text-[11px]">
                    <code className="text-sky-300/80 bg-slate-950 px-2 py-0.5 rounded border border-slate-800/80">
                      {log.details_json}
                    </code>
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>

        {logs.length === 0 && (
          <div className="py-16 text-center text-xs text-slate-500">
            No audit records found.
          </div>
        )}
      </div>
    </div>
  );
};
