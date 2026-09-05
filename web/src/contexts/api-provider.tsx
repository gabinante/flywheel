import { useState, type ReactNode } from 'react'

import { APIContext } from '@/contexts/api-context'
import { createFlywheelClient } from '@/lib/api/client'

export function APIProvider({ children }: { children: ReactNode }) {
  const [value] = useState(() => ({ client: createFlywheelClient() }))
  return <APIContext.Provider value={value}>{children}</APIContext.Provider>
}
