import { Outlet } from 'react-router-dom'

import { LeftSidebar } from '@/components/left-sidebar'
import { SidebarProvider } from '@/contexts/sidebar-provider'

export function AppShell() {
  return (
    <SidebarProvider>
      <div className="flex h-screen overflow-hidden">
        <LeftSidebar />
        <main className="flex-1 overflow-y-auto">
          <div className="p-4">
            <Outlet />
          </div>
        </main>
      </div>
    </SidebarProvider>
  )
}
