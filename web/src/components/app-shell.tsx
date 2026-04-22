import { Outlet } from 'react-router-dom'

import { DispatchStatusIndicator } from '@/components/dispatch-status-indicator'
import { LeftSidebar } from '@/components/left-sidebar'
import { RightRail } from '@/components/right-rail'
import { RightRailWidgets } from '@/components/right-rail-widgets'
import { RightRailProvider } from '@/contexts/right-rail-provider'
import { SidebarProvider } from '@/contexts/sidebar-provider'
import { useAuth } from '@/contexts/use-auth'

export function AppShell() {
  const { token } = useAuth()

  return (
    <SidebarProvider>
      <RightRailProvider railContent={token ? <RightRailWidgets /> : undefined}>
        <div className="flex h-screen overflow-hidden">
          <LeftSidebar />
          <div className="flex flex-1 flex-col overflow-hidden">
            {token && (
              <div className="flex h-10 shrink-0 items-center justify-end gap-2 border-b border-border bg-background/80 px-4 backdrop-blur">
                <DispatchStatusIndicator />
              </div>
            )}
            <main className="flex-1 overflow-y-auto">
              <div className="p-4">
                <Outlet />
              </div>
            </main>
          </div>
          <RightRail />
        </div>
      </RightRailProvider>
    </SidebarProvider>
  )
}
