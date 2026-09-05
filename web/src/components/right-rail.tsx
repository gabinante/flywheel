import { PanelRightClose, PanelRightOpen } from 'lucide-react'

import { Button } from '@/components/ui/button'
import { useRightRail } from '@/contexts/use-right-rail'
import { cn } from '@/lib/utils'

const RAIL_WIDTH = 'w-80' // 320px

/**
 * Collapsible right-rail panel.
 *
 * Renders its children (provided via RightRailProvider's `railContent` prop)
 * inside a fixed-width sidebar that slides in/out on the right side of the
 * main content area. A toggle button is always visible in the header area
 * when rail content exists.
 */
export function RightRail() {
  const { isOpen, isOverlay, setOpen, children, hasContent } = useRightRail()

  if (!hasContent) return null

  return (
    <>
    {isOverlay && isOpen && <button type="button" aria-label="Close work tray" className="fixed inset-0 z-30 bg-black/50" onClick={() => setOpen(false)} />}
    <aside
      data-slot="right-rail"
      aria-label="Global work"
      aria-hidden={!isOpen}
      inert={!isOpen}
      className={cn(
        'shrink-0 overflow-hidden border-l border-white/10 bg-card backdrop-blur-md transition-[width] duration-200 ease-in-out',
        isOpen ? RAIL_WIDTH : 'w-0',
        isOverlay && 'fixed right-0 top-12 bottom-0 z-40',
      )}
    >
      <div className={cn('h-full overflow-y-auto p-4', RAIL_WIDTH)}>
        {children}
      </div>
    </aside>
    </>
  )
}

/**
 * Toggle button for the right-rail. Place this in the header or wherever
 * a control is needed. Only renders when the rail has content.
 */
export function RightRailToggle() {
  const { isOpen, toggle, hasContent } = useRightRail()

  if (!hasContent) return null

  return (
    <Button
      type="button"
      variant="ghost"
      size="icon-sm"
      onClick={toggle}
      aria-label={isOpen ? 'Close panel' : 'Open panel'}
    >
      {isOpen ? (
        <PanelRightClose className="size-4" />
      ) : (
        <PanelRightOpen className="size-4" />
      )}
    </Button>
  )
}
