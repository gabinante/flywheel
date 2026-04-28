import { useNavigate } from 'react-router-dom';

export default function DashboardPage() {
  const navigate = useNavigate();

  return (
    <div className="p-6 max-w-7xl mx-auto">
      <h1 className="text-2xl font-bold text-white mb-6">Admin Dashboard</h1>

      <div className="grid grid-cols-1 md:grid-cols-2 gap-6">
        <button
          onClick={() => navigate('/agencies')}
          className="bg-gray-900/50 border border-gray-800 rounded-xl p-6 text-left hover:bg-gray-800/50 transition-colors group"
        >
          <h2 className="text-lg font-semibold text-white group-hover:text-blue-400 transition-colors">
            Agency Management
          </h2>
          <p className="text-sm text-gray-400 mt-2">
            View and manage affiliate agencies, their tiers, merchants, and residual history.
          </p>
        </button>
      </div>
    </div>
  );
}
