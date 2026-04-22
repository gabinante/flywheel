import { useCallback, useMemo, useState, type ReactNode } from 'react'

import { AuthContext } from '@/contexts/auth-context'
import {
  clearStoredToken,
  consumeOAuthHashToken,
  getStoredToken,
} from '@/lib/auth-token'
import { createFlywheelClient } from '@/lib/api/client'

function readInitialToken(): string | null {
  if (typeof window === 'undefined') return null
  consumeOAuthHashToken()
  return getStoredToken()
}

export function AuthProvider({ children }: { children: ReactNode }) {
  const [token, setToken] = useState<string | null>(readInitialToken)

  const refreshTokenFromStorage = useCallback(() => {
    setToken(getStoredToken())
  }, [])

  const signOut = useCallback(() => {
    clearStoredToken()
    setToken(null)
  }, [])

  const client = useMemo(() => createFlywheelClient(() => getStoredToken()), [])

  const value = useMemo(
    () => ({
      token,
      client,
      signOut,
      refreshTokenFromStorage,
    }),
    [token, client, signOut, refreshTokenFromStorage],
  )

  return <AuthContext.Provider value={value}>{children}</AuthContext.Provider>
}
