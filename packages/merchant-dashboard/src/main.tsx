import React from 'react';
import ReactDOM from 'react-dom/client';
import { BrowserRouter } from 'react-router-dom';
import { initSentryFrontend } from '@gohighpayment/shared/sentry-init';
import { SentryErrorBoundary } from '@gohighpayment/shared/sentry-error-boundary';
import App from './App';

// Initialize Sentry before rendering
initSentryFrontend({ projectName: 'merchant-dashboard' });

ReactDOM.createRoot(document.getElementById('root')!).render(
  <React.StrictMode>
    <SentryErrorBoundary>
      <BrowserRouter>
        <App />
      </BrowserRouter>
    </SentryErrorBoundary>
  </React.StrictMode>
);
