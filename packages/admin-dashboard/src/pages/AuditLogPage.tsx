import { useState } from 'react';
import { useQuery } from '@tanstack/react-query';
import { api } from '../lib/api';

interface AuditEntry {
  id: string;
  actorType: string;
  actorId: string;
  action: string;
  resource: string;
  resourceId: string | null;
  details: Record<string, unknown> | null;
  ipAddress: string | null;
  createdAt: string;
}

export function AuditLogPage() {
  const [page] = useState(1);

  const { data, isLoading } = useQuery({
    queryKey: ['admin-audit-log', page],
    queryFn: () => api.get<{ logs: AuditEntry[]; total: number }>(`/audit-log?page=${page}&limit=50`),
  });

  return (
    <div>
      <h1 className="text-2xl font-bold text-gray-900 mb-6">Audit Log</h1>

      <div className="bg-white rounded-xl shadow-sm border border-gray-200 overflow-hidden">
        <table className="w-full">
          <thead>
            <tr className="bg-gray-50 border-b border-gray-200">
              <th className="text-left px-4 py-3 text-xs font-medium text-gray-500 uppercase">Time</th>
              <th className="text-left px-4 py-3 text-xs font-medium text-gray-500 uppercase">Actor</th>
              <th className="text-left px-4 py-3 text-xs font-medium text-gray-500 uppercase">Action</th>
              <th className="text-left px-4 py-3 text-xs font-medium text-gray-500 uppercase">Resource</th>
              <th className="text-left px-4 py-3 text-xs font-medium text-gray-500 uppercase">IP</th>
            </tr>
          </thead>
          <tbody>
            {isLoading ? (
              <tr><td colSpan={5} className="text-center py-8 text-gray-500">Loading...</td></tr>
            ) : (
              data?.logs.map((entry) => (
                <tr key={entry.id} className="border-b border-gray-100 hover:bg-gray-50">
                  <td className="px-4 py-3 text-sm text-gray-500">{new Date(entry.createdAt).toLocaleString()}</td>
                  <td className="px-4 py-3 text-sm">
                    <span className="font-mono text-xs">{entry.actorType}:{entry.actorId.slice(0, 8)}</span>
                  </td>
                  <td className="px-4 py-3 text-sm font-medium">{entry.action}</td>
                  <td className="px-4 py-3 text-sm text-gray-600">{entry.resource}{entry.resourceId ? `:${entry.resourceId.slice(0, 8)}` : ''}</td>
                  <td className="px-4 py-3 text-xs font-mono text-gray-500">{entry.ipAddress || '-'}</td>
                </tr>
              ))
            )}
          </tbody>
        </table>
      </div>
    </div>
  );
}
