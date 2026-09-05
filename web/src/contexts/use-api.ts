import { useContext } from 'react'

import { APIContext } from '@/contexts/api-context'

export function useAPI() {
  const ctx = useContext(APIContext)
  if (!ctx) throw new Error('useAPI must be used within APIProvider')
  return ctx
}
