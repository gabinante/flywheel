import React from 'react';
import ReactDOM from 'react-dom/client';
import { BrowserRouter, Routes, Route } from 'react-router-dom';
import { CheckoutPage } from './pages/CheckoutPage';
import { ConfirmationPage } from './pages/ConfirmationPage';
import './index.css';

ReactDOM.createRoot(document.getElementById('root')!).render(
  <React.StrictMode>
    <BrowserRouter>
      <Routes>
        <Route path="/:sessionId" element={<CheckoutPage />} />
        <Route path="/confirmation/:sessionId" element={<ConfirmationPage />} />
      </Routes>
    </BrowserRouter>
  </React.StrictMode>
);
