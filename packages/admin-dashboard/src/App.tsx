import { Routes, Route, Navigate } from "react-router-dom";
import Layout from "./components/Layout";
import ResidualsPage from "./pages/ResidualsPage";

export default function App() {
  return (
    <Layout>
      <Routes>
        <Route path="/residuals" element={<ResidualsPage />} />
        <Route path="*" element={<Navigate to="/residuals" replace />} />
      </Routes>
    </Layout>
  );
}
