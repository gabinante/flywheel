import { type ReactNode, useCallback, useEffect, useMemo, useState } from 'react'

import { RightRailContext } from '@/contexts/right-rail-context'
import { useLocalStorage } from '@/hooks/use-local-storage'

const STORAGE_KEY = 'warrant:right-rail-open'
const COLLAPSE_BREAKPOINT = 1024 // lg breakpoint in px

export function RightRailProvider({
  children,
  railContent,
}: {
  children: ReactNode
  railContent?: ReactNode
}) {
  const [persisted, setPersisted] = useLocalStorage(STORAGE_KEY, true)
  const [isNarrow, setIsNarrow] = useState(false)
  const [narrowOpen, setNarrowOpen] = useState(false)

  // Watch for viewport width changes to auto-collapse on narrow viewports
  useEffect(() => {
    const mql = window.matchMedia(`(max-width: ${COLLAPSE_BREAKPOINT - 1}px)`)
    const handler = (e: MediaQueryListEvent | MediaQueryList) => {
      setIsNarrow(e.matches)
    }
    handler(mql)
    mql.addEventListener('change', handler)
    return () => mql.removeEventListener('change', handler)
  }, [])

  // Narrow screens start closed but remain accessible through the same toggle.
  const isOpen = isNarrow ? narrowOpen : persisted

  const toggle = useCallback(() => {
    if (isNarrow) setNarrowOpen(prev => !prev)
    else setPersisted((prev) => !prev)
  }, [isNarrow, setPersisted])

  const setOpen = useCallback(
    (open: boolean) => {
      if (isNarrow) setNarrowOpen(open)
      else setPersisted(open)
    },
    [isNarrow, setPersisted],
  )

  const hasContent = railContent != null

  const value = useMemo(
    () => ({
      isOpen: isOpen && hasContent,
      isOverlay: isNarrow,
      toggle,
      setOpen,
      children: railContent,
      hasContent,
    }),
    [
      hasContent,
      railContent,
      isOpen,
      isNarrow,
      setOpen,
      toggle,
    ],
  )

  return <RightRailContext value={value}>{children}</RightRailContext>
}
