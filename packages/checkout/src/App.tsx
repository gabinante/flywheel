import { Routes, Route } from 'react-router-dom';

function CheckoutPage() {
  return <div>Checkout</div>;
}

export default function App() {
  return (
    <Routes>
      <Route path="/" element={<CheckoutPage />} />
    </Routes>
  );
}
