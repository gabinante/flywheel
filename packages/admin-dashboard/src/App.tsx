import { BrowserRouter, Navigate, Route, Routes } from "react-router-dom";
import { Layout } from "./components/Layout";
import { AgenciesPage } from "./pages/AgenciesPage";
import { AgencyDetailPage } from "./pages/AgencyDetailPage";
import { ChargebacksPage } from "./pages/ChargebacksPage";
import { MerchantDetailPage } from "./pages/MerchantDetailPage";

export default function App() {
  return (
    <BrowserRouter>
      <Routes>
        <Route element={<Layout />}>
          {/* Redirect root to agencies */}
          <Route path="/" element={<Navigate to="/agencies" replace />} />
          <Route path="/agencies" element={<AgenciesPage />} />
          <Route path="/agencies/:id" element={<AgencyDetailPage />} />
          {/* Chargeback risk monitoring */}
          <Route path="/chargebacks" element={<ChargebacksPage />} />
          <Route path="/merchants/:id" element={<MerchantDetailPage />} />
        </Route>
      </Routes>
    </BrowserRouter>
  );
}
