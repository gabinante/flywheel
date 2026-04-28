import { type ReactNode } from "react";
import { Link, useLocation } from "react-router-dom";

const navItems = [
  { label: "Residuals", path: "/residuals" },
  // Future nav items: Agencies, Revenue, Chargebacks, etc.
];

interface LayoutProps {
  children: ReactNode;
}

export default function Layout({ children }: LayoutProps) {
  const location = useLocation();

  return (
    <div style={{ display: "flex", minHeight: "100vh", background: "#0a0a0f", color: "#e2e8f0" }}>
      {/* Sidebar */}
      <nav
        style={{
          width: 240,
          background: "linear-gradient(180deg, #111118 0%, #0d0d14 100%)",
          borderRight: "1px solid rgba(255,255,255,0.06)",
          padding: "24px 0",
          flexShrink: 0,
        }}
      >
        <div style={{ padding: "0 20px 24px", borderBottom: "1px solid rgba(255,255,255,0.06)" }}>
          <h1 style={{ fontSize: 16, fontWeight: 700, margin: 0, color: "#fff" }}>
            GoHighPayment
          </h1>
          <span style={{ fontSize: 11, color: "#64748b", marginTop: 2, display: "block" }}>
            Admin Dashboard
          </span>
        </div>

        <div style={{ padding: "16px 12px" }}>
          {navItems.map((item) => {
            const isActive = location.pathname.startsWith(item.path);
            return (
              <Link
                key={item.path}
                to={item.path}
                style={{
                  display: "block",
                  padding: "10px 12px",
                  borderRadius: 8,
                  fontSize: 14,
                  fontWeight: isActive ? 600 : 400,
                  color: isActive ? "#fff" : "#94a3b8",
                  background: isActive ? "rgba(99,102,241,0.15)" : "transparent",
                  textDecoration: "none",
                  marginBottom: 2,
                  transition: "all 0.15s",
                }}
              >
                {item.label}
              </Link>
            );
          })}
        </div>
      </nav>

      {/* Main content */}
      <main style={{ flex: 1, padding: 32, overflow: "auto" }}>
        {children}
      </main>
    </div>
  );
}
