import { createContext } from 'react'

import { createFlywheelClient } from '@/lib/api/client'

export type FlywheelClient = ReturnType<typeof createFlywheelClient>

export type AuthContextValue = {
  token: string | null
  client: FlywheelClient
  signOut: () => void
  refreshTokenFromStorage: () => void
}

export const AuthContext = createContext<AuthContextValue | null>(null)
