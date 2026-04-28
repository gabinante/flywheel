import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query';
import { useState } from 'react';
import { api } from '../lib/api';

interface WebhookDelivery {
  id: string;
  url: string;
  statusCode: number | null;
  success: boolean;
  attemptNumber: number;
  createdAt: string;
  webhookEvent: { eventType: string };
}

export function WebhooksPage() {
  const queryClient = useQueryClient();
  const [webhookUrl, setWebhookUrl] = useState('');
  const [loaded, setLoaded] = useState(false);

  useQuery({
    queryKey: ['merchant-webhook-config'],
    queryFn: async () => {
      const data = await api.get<{ webhookUrl: string | null }>('/webhooks/config');
      if (!loaded) {
        setWebhookUrl(data.webhookUrl || '');
        setLoaded(true);
      }
      return data;
    },
  });

  const { data: deliveries } = useQuery({
    queryKey: ['merchant-webhook-deliveries'],
    queryFn: () => api.get<{ deliveries: WebhookDelivery[]; total: number }>('/webhooks/deliveries?limit=20'),
  });

  const updateUrl = useMutation({
    mutationFn: (url: string) => api.put('/webhooks/config', { webhookUrl: url || null }),
    onSuccess: () => queryClient.invalidateQueries({ queryKey: ['merchant-webhook-config'] }),
  });

  return (
    <div>
      <h1 className="text-2xl font-bold text-gray-900 mb-6">Webhooks</h1>

      {/* Config */}
      <div className="bg-white rounded-xl shadow-sm border border-gray-200 p-5 mb-6">
        <h2 className="font-semibold text-gray-900 mb-3">Webhook URL</h2>
        <div className="flex gap-2">
          <input
            type="url"
            value={webhookUrl}
            onChange={e => setWebhookUrl(e.target.value)}
            placeholder="https://your-site.com/webhook"
            className="flex-1 px-3 py-2 border border-gray-300 rounded-lg text-sm focus:ring-2 focus:ring-primary-500"
          />
          <button
            onClick={() => updateUrl.mutate(webhookUrl)}
            className="bg-primary-600 hover:bg-primary-700 text-white px-4 py-2 rounded-lg text-sm font-medium"
          >
            Save
          </button>
        </div>
      </div>

      {/* Recent deliveries */}
      <div className="bg-white rounded-xl shadow-sm border border-gray-200 overflow-hidden">
        <div className="px-4 py-3 border-b border-gray-200">
          <h2 className="font-semibold text-gray-900">Recent Deliveries</h2>
        </div>
        <table className="w-full">
          <thead>
            <tr className="bg-gray-50 border-b border-gray-200">
              <th className="text-left px-4 py-3 text-xs font-medium text-gray-500 uppercase">Time</th>
              <th className="text-left px-4 py-3 text-xs font-medium text-gray-500 uppercase">Event</th>
              <th className="text-left px-4 py-3 text-xs font-medium text-gray-500 uppercase">Status</th>
              <th className="text-left px-4 py-3 text-xs font-medium text-gray-500 uppercase">Attempt</th>
            </tr>
          </thead>
          <tbody>
            {!deliveries?.deliveries.length ? (
              <tr><td colSpan={4} className="text-center py-8 text-gray-500">No deliveries yet</td></tr>
            ) : (
              deliveries.deliveries.map((d) => (
                <tr key={d.id} className="border-b border-gray-100 hover:bg-gray-50">
                  <td className="px-4 py-3 text-sm text-gray-500">{new Date(d.createdAt).toLocaleString()}</td>
                  <td className="px-4 py-3 text-sm font-medium">{d.webhookEvent.eventType}</td>
                  <td className="px-4 py-3">
                    <span className={`inline-block px-2 py-0.5 rounded-full text-xs font-medium ${
                      d.success ? 'bg-green-100 text-green-700' : 'bg-red-100 text-red-700'
                    }`}>
                      {d.success ? `${d.statusCode} OK` : `${d.statusCode || 'Error'}`}
                    </span>
                  </td>
                  <td className="px-4 py-3 text-sm text-gray-500">#{d.attemptNumber}</td>
                </tr>
              ))
            )}
          </tbody>
        </table>
      </div>
    </div>
  );
}
