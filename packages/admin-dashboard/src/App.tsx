import { Routes, Route } from 'react-router-dom';
import Layout from './components/Layout';
import DashboardPage from './pages/DashboardPage';
import AgenciesPage from './pages/AgenciesPage';
import AgencyDetailPage from './pages/AgencyDetailPage';

export default function App() {
  return (
    <Routes>
      <Route element={<Layout />}>
        <Route path="/" element={<DashboardPage />} />
        <Route path="/agencies" element={<AgenciesPage />} />
        <Route path="/agencies/:id" element={<AgencyDetailPage />} />
      </Route>
    </Routes>
  );
}
