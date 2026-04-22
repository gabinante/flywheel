import { useContext } from 'react'

import { RightRailContext } from '@/contexts/right-rail-context'

export function useRightRail() {
  const ctx = useContext(RightRailContext)
  if (!ctx) throw new Error('useRightRail must be used within RightRailProvider')
  return ctx
}
