import { Routes, Route, Navigate } from 'react-router-dom';
import { Layout } from './components/Layout';
import { LoginPage } from './pages/LoginPage';
import { DashboardPage } from './pages/DashboardPage';
import { MerchantsPage } from './pages/MerchantsPage';
import { MerchantDetailPage } from './pages/MerchantDetailPage';
import { TransactionsPage } from './pages/TransactionsPage';
import { ChargebacksPage } from './pages/ChargebacksPage';
import { StaffPage } from './pages/StaffPage';
import { AuditLogPage } from './pages/AuditLogPage';
import { AnalyticsPage } from './pages/AnalyticsPage';

function isAuthenticated() {
  return !!localStorage.getItem('admin_token');
}

function ProtectedRoute({ children }: { children: React.ReactNode }) {
  if (!isAuthenticated()) return <Navigate to="/login" replace />;
  return <>{children}</>;
}

export default function App() {
  return (
    <Routes>
      <Route path="/login" element={<LoginPage />} />
      <Route
        path="/*"
        element={
          <ProtectedRoute>
            <Layout>
              <Routes>
                <Route path="/" element={<DashboardPage />} />
                <Route path="/merchants" element={<MerchantsPage />} />
                <Route path="/merchants/:id" element={<MerchantDetailPage />} />
                <Route path="/transactions" element={<TransactionsPage />} />
                <Route path="/chargebacks" element={<ChargebacksPage />} />
                <Route path="/analytics" element={<AnalyticsPage />} />
                <Route path="/staff" element={<StaffPage />} />
                <Route path="/audit-log" element={<AuditLogPage />} />
              </Routes>
            </Layout>
          </ProtectedRoute>
        }
      />
    </Routes>
  );
}
