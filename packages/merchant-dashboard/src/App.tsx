import { Routes, Route } from 'react-router-dom';

function DashboardPage() {
  return <div>Merchant Dashboard</div>;
}

export default function App() {
  return (
    <Routes>
      <Route path="/" element={<DashboardPage />} />
    </Routes>
  );
}
