const API_BASE = '/api/v1/merchant';

function getToken(): string | null {
  return localStorage.getItem('merchant_token');
}

export function setToken(token: string) {
  localStorage.setItem('merchant_token', token);
}

export function clearToken() {
  localStorage.removeItem('merchant_token');
  localStorage.removeItem('merchant_refresh_token');
}

function isEmbedded(): boolean {
  return sessionStorage.getItem('ghl_embedded') === 'true';
}

async function apiFetch<T>(path: string, options: RequestInit = {}): Promise<T> {
  const token = getToken();
  const headers: Record<string, string> = {
    'Content-Type': 'application/json',
    ...(token && { Authorization: `Bearer ${token}` }),
    ...(options.headers as Record<string, string>),
  };

  const response = await fetch(`${API_BASE}${path}`, { ...options, headers });

  if (response.status === 401) {
    clearToken();

    if (isEmbedded()) {
      // In GHL iframe - tell the parent to re-trigger SSO
      // rather than showing our login page inside their UI
      try {
        window.parent.postMessage({ type: 'ghp:session-expired' }, '*');
      } catch {
        // ignore cross-origin
      }
      throw new Error('Session expired. Please reopen GoHighPayment from the GHL sidebar.');
    }

    window.location.href = '/login';
    throw new Error('Unauthorized');
  }

  if (!response.ok) {
    const error = await response.json().catch(() => ({ error: 'Request failed' }));
    throw new Error(error.error || `HTTP ${response.status}`);
  }

  return response.json();
}

export const api = {
  get: <T>(path: string) => apiFetch<T>(path),
  post: <T>(path: string, body?: unknown) =>
    apiFetch<T>(path, { method: 'POST', body: body ? JSON.stringify(body) : undefined }),
  patch: <T>(path: string, body: unknown) =>
    apiFetch<T>(path, { method: 'PATCH', body: JSON.stringify(body) }),
  put: <T>(path: string, body: unknown) =>
    apiFetch<T>(path, { method: 'PUT', body: JSON.stringify(body) }),
};

export async function merchantLogin(email: string, password: string) {
  const res = await fetch('/api/v1/merchant/auth/login', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ email, password }),
  });

  if (!res.ok) {
    const err = await res.json();
    throw new Error(err.error || 'Login failed');
  }

  const data = await res.json();
  setToken(data.token);
  if (data.refreshToken) {
    localStorage.setItem('merchant_refresh_token', data.refreshToken);
  }
  return data;
}
