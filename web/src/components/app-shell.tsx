import { useCallback, useEffect, useRef, useState } from 'react'
import { Link, Outlet } from 'react-router-dom'

import {
  Keyboard,
  LogOut,
  PanelRightClose,
  PanelRightOpen,
} from 'lucide-react'

import { DispatchStatusIndicator } from '@/components/dispatch-status-indicator'
import { FlywheelLogo } from '@/components/flywheel-logo'
import { HeaderBreadcrumbs } from '@/components/header-breadcrumbs'
import { KeyboardShortcutHelp } from '@/components/keyboard-shortcut-help'
import { LeftSidebar } from '@/components/left-sidebar'
import { RightRail } from '@/components/right-rail'
import { RightRailWidgets } from '@/components/right-rail-widgets'
import { Button } from '@/components/ui/button'
import { RightRailProvider } from '@/contexts/right-rail-provider'
import { SidebarProvider } from '@/contexts/sidebar-provider'
import { useAuth } from '@/contexts/use-auth'
import { useRightRail } from '@/contexts/use-right-rail'
import { cn } from '@/lib/utils'

/** Threshold in pixels before the header intensifies its blur */
const SCROLL_THRESHOLD = 8

/**
 * Enhanced right-rail toggle with label and keyboard shortcut hint.
 * More discoverable than a bare icon button.
 */
function EnhancedRightRailToggle() {
  const { isOpen, toggle, hasContent } = useRightRail()

  // Keyboard shortcut: ] to toggle panel
  useEffect(() => {
    if (!hasContent) return
    function onKey(e: KeyboardEvent) {
      if (
        e.key === ']' &&
        !e.ctrlKey &&
        !e.metaKey &&
        !(e.target instanceof HTMLInputElement) &&
        !(e.target instanceof HTMLTextAreaElement)
      ) {
        e.preventDefault()
        toggle()
      }
    }
    window.addEventListener('keydown', onKey)
    return () => window.removeEventListener('keydown', onKey)
  }, [hasContent, toggle])

  if (!hasContent) return null

  return (
    <Button
      type="button"
      variant="ghost"
      size="sm"
      onClick={toggle}
      aria-label={isOpen ? 'Close panel' : 'Open panel'}
      className="gap-1.5 text-muted-foreground transition-colors duration-150 hover:text-foreground"
    >
      {isOpen ? (
        <PanelRightClose className="size-4" />
      ) : (
        <PanelRightOpen className="size-4" />
      )}
      <span className="hidden text-xs sm:inline">Panel</span>
      <kbd className="hidden rounded border border-white/10 bg-white/5 px-1 py-0.5 font-mono text-[10px] text-muted-foreground/70 sm:inline">
        ]
      </kbd>
    </Button>
  )
}

/**
 * Inner shell rendered within RightRailProvider so it can access useRightRail.
 */
function AppShellInner() {
  const { token, signOut } = useAuth()

  // Track scroll position for blur-on-scroll effect
  const [scrolled, setScrolled] = useState(false)
  const mainRef = useRef<HTMLDivElement>(null)

  // Keyboard shortcuts help overlay
  const [showShortcuts, setShowShortcuts] = useState(false)

  const handleScroll = useCallback(() => {
    if (mainRef.current) {
      setScrolled(mainRef.current.scrollTop > SCROLL_THRESHOLD)
    }
  }, [])

  useEffect(() => {
    const el = mainRef.current
    if (!el) return
    el.addEventListener('scroll', handleScroll, { passive: true })
    return () => el.removeEventListener('scroll', handleScroll)
  }, [handleScroll])

  // Global keyboard shortcut for ? help
  useEffect(() => {
    function onKey(e: KeyboardEvent) {
      if (
        e.key === '?' &&
        !e.ctrlKey &&
        !e.metaKey &&
        !(e.target instanceof HTMLInputElement) &&
        !(e.target instanceof HTMLTextAreaElement)
      ) {
        e.preventDefault()
        setShowShortcuts((prev) => !prev)
      }
    }
    window.addEventListener('keydown', onKey)
    return () => window.removeEventListener('keydown', onKey)
  }, [])

  return (
    <>
      <div className="flex h-screen overflow-hidden">
        <LeftSidebar />
        <div className="flex flex-1 flex-col overflow-hidden">
          {/* ─── Glassmorphic sticky header ─── */}
          <header
            className={cn(
              'sticky top-0 z-40 border-b border-white/[0.06] transition-all duration-300',
              scrolled
                ? 'bg-background/70 shadow-[0_1px_3px_0_rgba(0,0,0,0.3)] backdrop-blur-xl backdrop-saturate-150'
                : 'bg-background/40 backdrop-blur-md',
            )}
          >
            <div className="flex h-14 items-center justify-between gap-4 px-4">
              {/* ─── Left: Logo + breadcrumbs ─── */}
              <div className="flex min-w-0 items-center gap-3">
                <Link
                  to="/"
                  className="shrink-0 transition-opacity duration-150 hover:opacity-80"
                  aria-label="Flywheel home"
                >
                  <FlywheelLogo />
                </Link>

                {/* Separator + breadcrumbs */}
                {token && (
                  <>
                    <div className="h-5 w-px shrink-0 bg-white/10" aria-hidden />
                    <HeaderBreadcrumbs className="min-w-0" />
                  </>
                )}
              </div>

              {/* ─── Right: actions ─── */}
              <div className="flex shrink-0 items-center gap-1.5">
                {/* Dispatch status indicator */}
                {token && <DispatchStatusIndicator />}

                {/* Keyboard shortcut hint */}
                <Button
                  type="button"
                  variant="ghost"
                  size="icon-sm"
                  onClick={() => setShowShortcuts(true)}
                  aria-label="Keyboard shortcuts"
                  className="text-muted-foreground/60 transition-colors duration-150 hover:text-foreground"
                >
                  <Keyboard className="size-4" />
                </Button>

                {/* Right rail toggle (more discoverable) */}
                <EnhancedRightRailToggle />

                {/* Separator */}
                <div className="mx-1 h-5 w-px bg-white/10" aria-hidden />

                {/* Auth buttons */}
                {token ? (
                  <Button
                    type="button"
                    variant="ghost"
                    size="sm"
                    onClick={signOut}
                    className="gap-1.5 text-muted-foreground transition-colors duration-150 hover:text-destructive"
                  >
                    <LogOut className="size-3.5" />
                    <span className="hidden sm:inline">Sign out</span>
                  </Button>
                ) : (
                  <Button
                    asChild
                    size="sm"
                    className="bg-primary/90 backdrop-blur-sm transition-all duration-200 hover:bg-primary"
                  >
                    <a href="/auth/login" className="gap-1.5">
                      Sign in
                    </a>
                  </Button>
                )}
              </div>
            </div>
          </header>

          {/* ─── Main content area ─── */}
          <main ref={mainRef} className="flex-1 overflow-y-auto" id="main-scroll">
            <div className="w-full p-4">
              <Outlet />
            </div>
          </main>
        </div>
        <RightRail />
      </div>

      {/* Keyboard shortcuts overlay */}
      <KeyboardShortcutHelp
        open={showShortcuts}
        onClose={() => setShowShortcuts(false)}
      />
    </>
  )
}

export function AppShell() {
  const { token } = useAuth()

  return (
    <SidebarProvider>
      <RightRailProvider railContent={token ? <RightRailWidgets /> : undefined}>
        <AppShellInner />
      </RightRailProvider>
    </SidebarProvider>
  )
}
