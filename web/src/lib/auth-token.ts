const STORAGE_KEY = 'flywheel_jwt'

export function getStoredToken(): string | null {
  try {
    return sessionStorage.getItem(STORAGE_KEY)
  } catch {
    return null
  }
}

export function setStoredToken(token: string): void {
  try {
    sessionStorage.setItem(STORAGE_KEY, token)
  } catch {
    // ignore quota / private mode
  }
}

export function clearStoredToken(): void {
  try {
    sessionStorage.removeItem(STORAGE_KEY)
  } catch {
    // ignore
  }
}

/** Parse #token=... from the URL, persist, and strip the hash (fragment is not sent to the server). */
export function consumeOAuthHashToken(): void {
  if (typeof window === 'undefined') return
  const hash = window.location.hash
  if (!hash || hash.length < 2) return
  const params = new URLSearchParams(hash.slice(1))
  const token = params.get('token')
  if (!token) return
  setStoredToken(token)
  const { pathname, search } = window.location
  window.history.replaceState(null, '', pathname + search)
}
