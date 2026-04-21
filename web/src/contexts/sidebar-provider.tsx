import { type ReactNode, useCallback, useEffect, useMemo, useState } from 'react'

import { SidebarContext } from '@/contexts/sidebar-context'
import { useLocalStorage } from '@/hooks/use-local-storage'

const STORAGE_KEY = 'flywheel:sidebar-expanded'
const COLLAPSE_BREAKPOINT = 1024

export function SidebarProvider({ children }: { children: ReactNode }) {
  const [persisted, setPersisted] = useLocalStorage(STORAGE_KEY, true)
  const [isNarrow, setIsNarrow] = useState(false)

  useEffect(() => {
    const mql = window.matchMedia(`(max-width: ${COLLAPSE_BREAKPOINT - 1}px)`)
    const handler = (e: MediaQueryListEvent | MediaQueryList) => {
      setIsNarrow(e.matches)
    }
    handler(mql)
    mql.addEventListener('change', handler)
    return () => mql.removeEventListener('change', handler)
  }, [])

  const isExpanded = persisted && !isNarrow

  const toggle = useCallback(() => {
    setPersisted((prev) => !prev)
  }, [setPersisted])

  const setExpanded = useCallback(
    (expanded: boolean) => {
      setPersisted(expanded)
    },
    [setPersisted],
  )

  const value = useMemo(
    () => ({ isExpanded, toggle, setExpanded }),
    [isExpanded, toggle, setExpanded],
  )

  return <SidebarContext value={value}>{children}</SidebarContext>
}
