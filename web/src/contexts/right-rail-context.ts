import { createContext, type ReactNode } from 'react'

export type RightRailContextValue = {
  /** Whether the right-rail panel is currently open */
  isOpen: boolean
  isOverlay: boolean
  /** Toggle the right-rail open/closed */
  toggle: () => void
  /** Explicitly set the right-rail open state */
  setOpen: (open: boolean) => void
  /** Content to render inside the right-rail */
  children: ReactNode
  /** Whether any content has been provided to the right-rail */
  hasContent: boolean

}

export const RightRailContext = createContext<RightRailContextValue | null>(null)
