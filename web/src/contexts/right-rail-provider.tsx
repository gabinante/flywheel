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
  const [overrideContent, setOverrideContent] = useState<ReactNode>(undefined)
  const [hasOverride, setHasOverride] = useState(false)

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

  // Effective open state: persisted preference AND wide viewport
  const isOpen = persisted && !isNarrow

  const toggle = useCallback(() => {
    setPersisted((prev) => !prev)
  }, [setPersisted])

  const setOpen = useCallback(
    (open: boolean) => {
      setPersisted(open)
    },
    [setPersisted],
  )

  const setRailContent = useCallback((content: ReactNode) => {
    setOverrideContent(content)
    setHasOverride(true)
  }, [])

  const clearRailContent = useCallback(() => {
    setOverrideContent(undefined)
    setHasOverride(false)
  }, [])

  const effectiveContent = hasOverride ? overrideContent : railContent
  const hasContent = effectiveContent != null

  const value = useMemo(
    () => ({
      isOpen: isOpen && hasContent,
      toggle,
      setOpen,
      children: effectiveContent,
      hasContent,
      setRailContent,
      clearRailContent,
    }),
    [
      clearRailContent,
      effectiveContent,
      hasContent,
      isOpen,
      setOpen,
      setRailContent,
      toggle,
    ],
  )

  return <RightRailContext value={value}>{children}</RightRailContext>
}
