import { createContext, type ReactNode } from 'react'

export type RightRailContextValue = {
  /** Whether the right-rail panel is currently open */
  isOpen: boolean
  /** Toggle the right-rail open/closed */
  toggle: () => void
  /** Explicitly set the right-rail open state */
  setOpen: (open: boolean) => void
  /** Content to render inside the right-rail */
  children: ReactNode
  /** Whether any content has been provided to the right-rail */
  hasContent: boolean
  /** Replace the default right-rail content for the current route. */
  setRailContent: (content: ReactNode) => void
  /** Restore the default right-rail content. */
  clearRailContent: () => void
}

export const RightRailContext = createContext<RightRailContextValue | null>(null)
