import { createContext } from 'react'

export type SidebarContextValue = {
  /** Whether the sidebar is currently expanded */
  isExpanded: boolean
  /** Toggle the sidebar expanded/collapsed */
  toggle: () => void
  /** Explicitly set the sidebar expanded state */
  setExpanded: (expanded: boolean) => void
}

export const SidebarContext = createContext<SidebarContextValue | null>(null)
