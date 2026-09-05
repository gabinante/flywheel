import { createContext } from 'react'

import { createFlywheelClient } from '@/lib/api/client'

export type FlywheelClient = ReturnType<typeof createFlywheelClient>
export type APIContextValue = { client: FlywheelClient }
export const APIContext = createContext<APIContextValue | null>(null)
