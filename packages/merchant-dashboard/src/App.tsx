import { Routes, Route, Navigate, useSearchParams } from 'react-router-dom';
import { useEffect, useState } from 'react';
import { Layout } from './components/Layout';
import { LoginPage } from './pages/LoginPage';
import { DashboardPage } from './pages/DashboardPage';
import { TransactionsPage } from './pages/TransactionsPage';
import { SettingsPage } from './pages/SettingsPage';
import { WebhooksPage } from './pages/WebhooksPage';
import { OnboardingPage } from './pages/OnboardingPage';
import { setToken } from './lib/api';

function isAuthenticated() {
  return !!localStorage.getItem('merchant_token');
}

function ProtectedRoute({ children }: { children: React.ReactNode }) {
  if (!isAuthenticated()) return <Navigate to="/login" replace />;
  return <>{children}</>;
}

/**
 * Handles the OAuth callback redirect (after GHL OAuth install flow).
 */
function AuthCallback() {
  const [searchParams] = useSearchParams();
  const token = searchParams.get('token');

  useEffect(() => {
    if (token) {
      setToken(token);
      window.location.href = '/';
    }
  }, [token]);

  return (
    <div className="flex items-center justify-center h-screen">
      <div className="animate-spin rounded-full h-8 w-8 border-b-2 border-primary-600" />
    </div>
  );
}

/**
 * Handles GHL SSO entry.
 */
function GhlSsoEntry() {
  const [searchParams] = useSearchParams();
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    const ssoToken = searchParams.get('ssoToken') || searchParams.get('token');

    if (!ssoToken) {
      setError('No SSO token provided');
      return;
    }

    async function validateSso(token: string) {
      try {
        const res = await fetch('/api/v1/ghl/sso', {
          method: 'POST',
          headers: { 'Content-Type': 'application/json' },
          body: JSON.stringify({ ssoToken: token }),
        });

        if (!res.ok) {
          const err = await res.json().catch(() => ({ error: 'SSO failed' }));
          setError(err.error || 'SSO validation failed');
          return;
        }

        const data = await res.json();
        setToken(data.token);

        // Mark as GHL-embedded so Layout knows to use iframe mode
        sessionStorage.setItem('ghl_embedded', 'true');

        // Navigate to dashboard (stays in iframe)
        window.location.href = '/';
      } catch {
        setError('Failed to connect to server');
      }
    }

    validateSso(ssoToken);
  }, [searchParams]);

  if (error) {
    return (
      <div className="flex items-center justify-center h-screen bg-gray-50">
        <div className="bg-white rounded-xl shadow-lg p-6 max-w-sm text-center">
          <div className="text-red-500 font-semibold mb-2">SSO Error</div>
          <p className="text-gray-600 text-sm">{error}</p>
          <p className="text-gray-400 text-xs mt-3">
            Try reinstalling GoHighPayment from the GHL marketplace.
          </p>
        </div>
      </div>
    );
  }

  return (
    <div className="flex items-center justify-center h-screen bg-gray-50">
      <div className="text-center">
        <div className="animate-spin rounded-full h-8 w-8 border-b-2 border-primary-600 mx-auto mb-3" />
        <p className="text-sm text-gray-500">Connecting to GoHighPayment...</p>
      </div>
    </div>
  );
}

export default function App() {
  return (
    <Routes>
      <Route path="/login" element={<LoginPage />} />
      <Route path="/auth/callback" element={<AuthCallback />} />
      <Route path="/ghl/sso" element={<GhlSsoEntry />} />
      <Route path="/onboarding" element={<ProtectedRoute><OnboardingPage /></ProtectedRoute>} />
      <Route
        path="/*"
        element={
          <ProtectedRoute>
            <Layout>
              <Routes>
                <Route path="/" element={<DashboardPage />} />
                <Route path="/transactions" element={<TransactionsPage />} />
                <Route path="/webhooks" element={<WebhooksPage />} />
                <Route path="/settings" element={<SettingsPage />} />
              </Routes>
            </Layout>
          </ProtectedRoute>
        }
      />
    </Routes>
  );
}
