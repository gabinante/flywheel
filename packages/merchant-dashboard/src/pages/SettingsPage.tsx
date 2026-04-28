import { useQuery, useMutation } from '@tanstack/react-query';
import { useState } from 'react';
import { api } from '../lib/api';

interface Settings {
  id: string;
  businessName: string;
  contactEmail: string;
  contactPhone: string | null;
  website: string | null;
  status: string;
  processingConfigured: boolean;
  ghlLocationId: string | null;
  webhookUrl: string | null;
  createdAt: string;
}

export function SettingsPage() {
  const { data: settings } = useQuery({
    queryKey: ['merchant-settings'],
    queryFn: () => api.get<Settings>('/settings'),
  });

  const rotateMutation = useMutation({
    mutationFn: (keyType: 'live' | 'test') => api.post<{ key: string }>('/api-keys/rotate', { keyType }),
  });

  const [rotatedKey, setRotatedKey] = useState<{ type: string; key: string } | null>(null);

  const handleRotate = async (type: 'live' | 'test') => {
    const result = await rotateMutation.mutateAsync(type);
    setRotatedKey({ type, key: result.key });
  };

  if (!settings) return null;

  return (
    <div>
      <h1 className="text-2xl font-bold text-gray-900 mb-6">Account Settings</h1>

      <div className="grid grid-cols-1 lg:grid-cols-2 gap-6">
        {/* Account Info */}
        <div className="bg-white rounded-xl shadow-sm border border-gray-200 p-5">
          <h2 className="font-semibold text-gray-900 mb-4">Account Information</h2>
          <dl className="space-y-3 text-sm">
            <div className="flex justify-between"><dt className="text-gray-500">Business Name</dt><dd className="font-medium">{settings.businessName}</dd></div>
            <div className="flex justify-between"><dt className="text-gray-500">Email</dt><dd>{settings.contactEmail}</dd></div>
            <div className="flex justify-between"><dt className="text-gray-500">Phone</dt><dd>{settings.contactPhone || '-'}</dd></div>
            <div className="flex justify-between"><dt className="text-gray-500">Website</dt><dd>{settings.website || '-'}</dd></div>
            <div className="flex justify-between"><dt className="text-gray-500">Status</dt><dd><span className={`px-2 py-0.5 rounded-full text-xs font-medium ${settings.status === 'ACTIVE' ? 'bg-green-100 text-green-700' : 'bg-yellow-100 text-yellow-700'}`}>{settings.status}</span></dd></div>
            <div className="flex justify-between"><dt className="text-gray-500">Member Since</dt><dd>{new Date(settings.createdAt).toLocaleDateString()}</dd></div>
          </dl>
        </div>

        {/* Processing Status */}
        <div className="bg-white rounded-xl shadow-sm border border-gray-200 p-5">
          <h2 className="font-semibold text-gray-900 mb-4">Processing Status</h2>
          <dl className="space-y-3 text-sm">
            <div className="flex justify-between">
              <dt className="text-gray-500">Card Processing (NMI)</dt>
              <dd>{settings.processingConfigured ? (
                <span className="flex items-center gap-1.5"><span className="w-2 h-2 bg-green-500 rounded-full" /><span className="text-green-700 font-medium">Active</span></span>
              ) : (
                <span className="text-yellow-600 font-medium">Not configured</span>
              )}</dd>
            </div>
            <div className="flex justify-between">
              <dt className="text-gray-500">Account Status</dt>
              <dd><span className={`px-2 py-0.5 rounded-full text-xs font-medium ${settings.status === 'ACTIVE' ? 'bg-green-100 text-green-700' : 'bg-yellow-100 text-yellow-700'}`}>{settings.status}</span></dd>
            </div>
          </dl>
        </div>

        {/* GHL Connection */}
        <div className="bg-white rounded-xl shadow-sm border border-gray-200 p-5">
          <h2 className="font-semibold text-gray-900 mb-4">GoHighLevel Connection</h2>
          {settings.ghlLocationId ? (
            <div className="flex items-center gap-2">
              <span className="w-2 h-2 bg-green-500 rounded-full" />
              <span className="text-sm text-green-700 font-medium">Connected</span>
              <span className="text-xs text-gray-500 font-mono ml-2">{settings.ghlLocationId}</span>
            </div>
          ) : (
            <div className="text-sm text-gray-500">Not connected to GoHighLevel</div>
          )}
        </div>

        {/* API Keys */}
        <div className="bg-white rounded-xl shadow-sm border border-gray-200 p-5">
          <h2 className="font-semibold text-gray-900 mb-4">API Keys</h2>
          <div className="space-y-3">
            <div className="flex items-center justify-between">
              <span className="text-sm text-gray-600">Live Key</span>
              <button onClick={() => handleRotate('live')} className="text-xs text-primary-600 hover:text-primary-700 font-medium">Rotate</button>
            </div>
            <div className="flex items-center justify-between">
              <span className="text-sm text-gray-600">Test Key</span>
              <button onClick={() => handleRotate('test')} className="text-xs text-primary-600 hover:text-primary-700 font-medium">Rotate</button>
            </div>
            {rotatedKey && (
              <div className="mt-3 p-3 bg-yellow-50 border border-yellow-200 rounded-lg">
                <p className="text-xs text-yellow-800 font-medium mb-1">New {rotatedKey.type} key (save this now):</p>
                <code className="text-xs font-mono break-all">{rotatedKey.key}</code>
              </div>
            )}
          </div>
        </div>
      </div>
    </div>
  );
}
