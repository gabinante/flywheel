import { useParams } from 'react-router-dom';
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query';
import { useState } from 'react';
import { api } from '../lib/api';

interface MerchantDetail {
  id: string;
  businessName: string;
  contactEmail: string;
  contactPhone: string | null;
  website: string | null;
  status: string;
  platformFeePercentage: string;
  platformFlatFeeCents: number;
  ghlLocationId: string | null;
  ghlCompanyId: string | null;
  createdAt: string;
  application: { status: string } | null;
  processingCap: { dailyCapCents: number; monthlyCapCents: number; currentDailyCents: number; currentMonthlyCents: number } | null;
  merchantReserve: { balanceCents: number } | null;
  settlementProfile: { settlementDelayDays: number; reservePercentage: string } | null;
}

const statusOptions = ['PENDING', 'ACTIVE', 'SUSPENDED', 'DEACTIVATED'];

export function MerchantDetailPage() {
  const { id } = useParams<{ id: string }>();
  const queryClient = useQueryClient();
  const [editFees, setEditFees] = useState(false);
  const [feePercentage, setFeePercentage] = useState('');
  const [flatFee, setFlatFee] = useState('');

  const { data: merchant, isLoading } = useQuery({
    queryKey: ['merchant', id],
    queryFn: () => api.get<MerchantDetail>(`/merchants/${id}`),
  });

  const updateMutation = useMutation({
    mutationFn: (body: Record<string, unknown>) => api.patch(`/merchants/${id}`, body),
    onSuccess: () => queryClient.invalidateQueries({ queryKey: ['merchant', id] }),
  });

  if (isLoading || !merchant) {
    return <div className="flex items-center justify-center h-40"><div className="animate-spin rounded-full h-8 w-8 border-b-2 border-primary-600" /></div>;
  }

  const formatCents = (cents: number) =>
    new Intl.NumberFormat('en-US', { style: 'currency', currency: 'USD' }).format(cents / 100);

  return (
    <div>
      <div className="flex items-center justify-between mb-6">
        <div>
          <h1 className="text-2xl font-bold text-gray-900">{merchant.businessName}</h1>
          <p className="text-gray-500">{merchant.contactEmail}</p>
        </div>
        <select
          value={merchant.status}
          onChange={e => updateMutation.mutate({ status: e.target.value })}
          className="px-3 py-2 border border-gray-300 rounded-lg text-sm"
        >
          {statusOptions.map(s => <option key={s} value={s}>{s}</option>)}
        </select>
      </div>

      <div className="grid grid-cols-1 lg:grid-cols-2 gap-6">
        {/* Details */}
        <div className="bg-white rounded-xl shadow-sm border border-gray-200 p-5">
          <h2 className="font-semibold text-gray-900 mb-4">Details</h2>
          <dl className="space-y-3 text-sm">
            <div className="flex justify-between"><dt className="text-gray-500">ID</dt><dd className="font-mono text-xs">{merchant.id}</dd></div>
            <div className="flex justify-between"><dt className="text-gray-500">Phone</dt><dd>{merchant.contactPhone || '-'}</dd></div>
            <div className="flex justify-between"><dt className="text-gray-500">Website</dt><dd>{merchant.website || '-'}</dd></div>
            <div className="flex justify-between"><dt className="text-gray-500">Created</dt><dd>{new Date(merchant.createdAt).toLocaleDateString()}</dd></div>
            <div className="flex justify-between"><dt className="text-gray-500">Application</dt><dd>{merchant.application?.status || '-'}</dd></div>
          </dl>
        </div>

        {/* GHL Integration */}
        <div className="bg-white rounded-xl shadow-sm border border-gray-200 p-5">
          <h2 className="font-semibold text-gray-900 mb-4">GHL Integration</h2>
          <dl className="space-y-3 text-sm">
            <div className="flex justify-between"><dt className="text-gray-500">Location ID</dt><dd className="font-mono text-xs">{merchant.ghlLocationId || 'Not connected'}</dd></div>
            <div className="flex justify-between"><dt className="text-gray-500">Company ID</dt><dd className="font-mono text-xs">{merchant.ghlCompanyId || '-'}</dd></div>
          </dl>
        </div>

        {/* Fee Configuration */}
        <div className="bg-white rounded-xl shadow-sm border border-gray-200 p-5">
          <div className="flex items-center justify-between mb-4">
            <h2 className="font-semibold text-gray-900">Fee Configuration</h2>
            <button onClick={() => { setEditFees(!editFees); setFeePercentage(merchant.platformFeePercentage); setFlatFee(String(merchant.platformFlatFeeCents)); }} className="text-xs text-primary-600 hover:text-primary-700">
              {editFees ? 'Cancel' : 'Edit'}
            </button>
          </div>
          {editFees ? (
            <div className="space-y-3">
              <div>
                <label className="text-sm text-gray-500">Percentage</label>
                <input type="number" step="0.001" value={feePercentage} onChange={e => setFeePercentage(e.target.value)} className="w-full px-3 py-1.5 border rounded-lg text-sm" />
              </div>
              <div>
                <label className="text-sm text-gray-500">Flat Fee (cents)</label>
                <input type="number" value={flatFee} onChange={e => setFlatFee(e.target.value)} className="w-full px-3 py-1.5 border rounded-lg text-sm" />
              </div>
              <button onClick={() => { updateMutation.mutate({ platformFeePercentage: Number(feePercentage), platformFlatFeeCents: Number(flatFee) }); setEditFees(false); }} className="bg-primary-600 text-white px-4 py-1.5 rounded-lg text-sm">
                Save
              </button>
            </div>
          ) : (
            <dl className="space-y-3 text-sm">
              <div className="flex justify-between"><dt className="text-gray-500">Percentage</dt><dd>{merchant.platformFeePercentage}%</dd></div>
              <div className="flex justify-between"><dt className="text-gray-500">Flat Fee</dt><dd>{formatCents(merchant.platformFlatFeeCents)}</dd></div>
            </dl>
          )}
        </div>

        {/* Processing & Reserve */}
        <div className="bg-white rounded-xl shadow-sm border border-gray-200 p-5">
          <h2 className="font-semibold text-gray-900 mb-4">Processing & Reserve</h2>
          <dl className="space-y-3 text-sm">
            <div className="flex justify-between"><dt className="text-gray-500">Reserve Balance</dt><dd>{merchant.merchantReserve ? formatCents(merchant.merchantReserve.balanceCents) : '-'}</dd></div>
            {merchant.processingCap && (
              <>
                <div className="flex justify-between"><dt className="text-gray-500">Daily Cap</dt><dd>{formatCents(merchant.processingCap.dailyCapCents)}</dd></div>
                <div className="flex justify-between"><dt className="text-gray-500">Daily Used</dt><dd>{formatCents(merchant.processingCap.currentDailyCents)}</dd></div>
                <div className="flex justify-between"><dt className="text-gray-500">Monthly Cap</dt><dd>{formatCents(merchant.processingCap.monthlyCapCents)}</dd></div>
                <div className="flex justify-between"><dt className="text-gray-500">Monthly Used</dt><dd>{formatCents(merchant.processingCap.currentMonthlyCents)}</dd></div>
              </>
            )}
          </dl>
        </div>
      </div>
    </div>
  );
}
