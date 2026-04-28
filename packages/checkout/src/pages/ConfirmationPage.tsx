import { useState, useEffect } from 'react';
import { useParams } from 'react-router-dom';
import { motion } from 'framer-motion';

interface SessionResult {
  id: string;
  amountCents: number;
  currency: string;
  description?: string;
  status: string;
  transactionId?: string;
}

export function ConfirmationPage() {
  const { sessionId } = useParams<{ sessionId: string }>();
  const [session, setSession] = useState<SessionResult | null>(null);
  const [loading, setLoading] = useState(true);

  useEffect(() => {
    async function fetchSession() {
      try {
        const res = await fetch(`/api/v1/checkout/session/${sessionId}`);
        if (res.ok) {
          setSession(await res.json());
        }
      } finally {
        setLoading(false);
      }
    }
    fetchSession();
  }, [sessionId]);

  const formatAmount = (cents: number) =>
    new Intl.NumberFormat('en-US', { style: 'currency', currency: 'USD' }).format(cents / 100);

  if (loading) {
    return (
      <div className="min-h-screen flex items-center justify-center">
        <div className="animate-spin rounded-full h-8 w-8 border-b-2 border-primary-600" />
      </div>
    );
  }

  if (!session) {
    return (
      <div className="min-h-screen flex items-center justify-center bg-gray-50">
        <div className="text-gray-500">Session not found</div>
      </div>
    );
  }

  return (
    <div className="min-h-screen bg-gradient-to-br from-blue-50 to-slate-100 flex items-center justify-center p-4">
      <motion.div
        initial={{ opacity: 0, scale: 0.95 }}
        animate={{ opacity: 1, scale: 1 }}
        className="bg-white rounded-2xl shadow-xl w-full max-w-md overflow-hidden text-center"
      >
        <div className="p-8">
          <motion.div
            initial={{ scale: 0 }}
            animate={{ scale: 1 }}
            transition={{ delay: 0.2, type: 'spring', stiffness: 200 }}
            className="w-16 h-16 bg-green-100 rounded-full flex items-center justify-center mx-auto mb-4"
          >
            <svg className="w-8 h-8 text-green-600" fill="none" viewBox="0 0 24 24" stroke="currentColor">
              <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M5 13l4 4L19 7" />
            </svg>
          </motion.div>

          <h1 className="text-2xl font-bold text-gray-900 mb-2">Payment Successful</h1>
          <p className="text-gray-500 mb-6">
            {session.status === 'COMPLETED' ? 'Your payment has been processed.' : 'Your payment is being processed.'}
          </p>

          <div className="bg-gray-50 rounded-xl p-4 space-y-3 text-left">
            <div className="flex justify-between">
              <span className="text-gray-500 text-sm">Amount</span>
              <span className="font-semibold">{formatAmount(session.amountCents)}</span>
            </div>
            {session.description && (
              <div className="flex justify-between">
                <span className="text-gray-500 text-sm">Description</span>
                <span className="text-sm">{session.description}</span>
              </div>
            )}
            {session.transactionId && (
              <div className="flex justify-between">
                <span className="text-gray-500 text-sm">Reference</span>
                <span className="text-xs font-mono text-gray-600">{session.transactionId}</span>
              </div>
            )}
            <div className="flex justify-between">
              <span className="text-gray-500 text-sm">Status</span>
              <span className="text-sm font-medium text-green-600">
                {session.status === 'COMPLETED' ? 'Complete' : 'Processing'}
              </span>
            </div>
          </div>
        </div>

        <div className="px-6 py-3 bg-gray-50 border-t border-gray-100">
          <p className="text-xs text-gray-400">Secured by GoHighPayment</p>
        </div>
      </motion.div>
    </div>
  );
}
