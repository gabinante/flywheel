import { Link, Outlet } from 'react-router-dom'

import { RightRail, RightRailToggle } from '@/components/right-rail'
import { RightRailWidgets } from '@/components/right-rail-widgets'
import { Button } from '@/components/ui/button'
import { RightRailProvider } from '@/contexts/right-rail-provider'
import { useAuth } from '@/contexts/use-auth'

export function AppShell() {
  const { token, signOut } = useAuth()

  return (
    <RightRailProvider railContent={token ? <RightRailWidgets /> : undefined}>
      <div className="flex min-h-screen flex-col">
        <header className="border-border border-b bg-background/80 backdrop-blur">
          <div className="flex h-12 items-center justify-between gap-4 px-4">
            <nav className="flex items-center gap-3 text-sm">
              <Link
                to="/"
                className="text-foreground font-medium tracking-tight hover:underline"
              >
                Flywheel
              </Link>
              {token ? (
                <>
                  <Link
                    to="/orgs"
                    className="text-muted-foreground hover:text-foreground"
                  >
                    Organizations
                  </Link>
                </>
              ) : null}
            </nav>
            <div className="flex items-center gap-2">
              <RightRailToggle />
              {token ? (
                <Button type="button" variant="outline" size="sm" onClick={signOut}>
                  Sign out
                </Button>
              ) : (
                <Button asChild size="sm">
                  <a href="/auth/github">Sign in with GitHub</a>
                </Button>
              )}
            </div>
          </div>
        </header>
        <div className="flex flex-1 overflow-hidden">
          <main className="flex-1 overflow-y-auto">
            <div className="mx-auto w-full max-w-4xl p-4">
              <Outlet />
            </div>
          </main>
          <RightRail />
        </div>
      </div>
    </RightRailProvider>
  )
}
