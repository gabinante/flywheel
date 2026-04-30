import { NavLink, Outlet } from "react-router-dom";

interface NavItemProps {
  to: string;
  label: string;
}

function NavItem({ to, label }: NavItemProps) {
  return (
    <NavLink
      to={to}
      className={({ isActive }) =>
        [
          "block px-3 py-2 rounded-md text-sm font-medium transition-colors",
          isActive
            ? "bg-emerald-600/20 text-emerald-400"
            : "text-gray-400 hover:text-gray-200 hover:bg-white/5",
        ].join(" ")
      }
    >
      {label}
    </NavLink>
  );
}

export function Layout() {
  return (
    <div className="flex min-h-screen">
      {/* Sidebar */}
      <aside className="w-56 shrink-0 bg-[#161b22] border-r border-white/10 flex flex-col">
        {/* Brand */}
        <div className="px-4 py-5 border-b border-white/10">
          <span className="text-lg font-semibold text-white tracking-tight">
            Shamroq
          </span>
          <span className="ml-2 text-xs text-emerald-400 font-medium">
            Admin
          </span>
        </div>

        {/* Navigation */}
        <nav className="flex-1 px-3 py-4 space-y-1">
          <NavItem to="/agencies" label="Agencies" />
          {/* Future nav items: Merchants, Residuals, Revenue, Chargebacks */}
        </nav>

        {/* Footer */}
        <div className="px-4 py-3 border-t border-white/10">
          <p className="text-xs text-gray-600">Operator Dashboard</p>
        </div>
      </aside>

      {/* Main content */}
      <main className="flex-1 overflow-auto bg-[#0d1117]">
        <Outlet />
      </main>
    </div>
  );
}
