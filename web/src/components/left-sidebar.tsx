import {
  Activity,
  ChevronLeft,
  ChevronRight,
  ClipboardCheck,
  GitPullRequest,
  LayoutDashboard,
  LogOut,
  Building2,
  Settings,
  TerminalSquare,
  Ticket,
  Workflow,
} from 'lucide-react'
import { Link, useLocation } from 'react-router-dom'

import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { ProjectSwitcher } from '@/components/project-switcher'
import { useAuth } from '@/contexts/use-auth'
import { useProjectEscalations } from '@/hooks/use-project-escalations'
import { useProjectPaths } from '@/hooks/use-project-paths'
import { useSidebar } from '@/contexts/use-sidebar'
import { cn } from '@/lib/utils'

const EXPANDED_WIDTH = 'w-60' // 240px
const COLLAPSED_WIDTH = 'w-14' // 56px

type NavItem = {
  label: string
  icon: React.ComponentType<{ className?: string }>
  href: string
  /** Match pattern: if location starts with this, the item is active */
  match?: string
  badgeCount?: number
  children?: NavItem[]
}

function formatBadgeCount(count: number): string {
  return count > 9 ? '9+' : String(count)
}

function NavSection({
  title,
  items,
  expanded,
  currentPath,
}: {
  title: string
  items: NavItem[]
  expanded: boolean
  currentPath: string
}) {
  return (
    <div className="flex flex-col gap-0.5">
      {expanded && (
        <span className="px-3 pb-1 text-[10px] font-semibold uppercase tracking-widest text-muted-foreground">
          {title}
        </span>
      )}
      {items.map((item) => {
        const active = item.match
          ? currentPath.startsWith(item.match)
          : currentPath === item.href
        return (
          <div key={item.href} className="flex flex-col gap-0.5">
            <Link
              to={item.href}
              className={cn(
                'group relative flex items-center gap-2.5 rounded-lg px-3 py-2 text-sm font-medium transition-colors',
                active
                  ? 'bg-sidebar-accent text-sidebar-primary'
                  : 'text-sidebar-foreground/70 hover:bg-sidebar-accent/50 hover:text-sidebar-foreground',
                !expanded && 'justify-center px-0',
              )}
              title={expanded ? undefined : item.label}
            >
              <item.icon className="size-4 shrink-0" />
              {expanded && <span className="truncate">{item.label}</span>}
              {item.badgeCount && item.badgeCount > 0 ? (
                expanded ? (
                  <Badge className="ml-auto border-red-500/30 bg-red-500/15 text-red-400">
                    {formatBadgeCount(item.badgeCount)}
                  </Badge>
                ) : (
                  <span className="absolute right-1.5 top-1.5 flex min-w-4 items-center justify-center rounded-full border border-red-500/30 bg-red-500/90 px-1 text-[10px] font-semibold leading-4 text-white shadow-sm">
                    {formatBadgeCount(item.badgeCount)}
                  </span>
                )
              ) : null}
            </Link>
            {expanded && item.children && item.children.length > 0 ? (
              <div className="ml-5 flex flex-col gap-0.5 border-l border-sidebar-border/70 pl-2">
                {item.children.map((child) => {
                  const childActive = child.match
                    ? currentPath.startsWith(child.match)
                    : currentPath === child.href
                  return (
                    <Link
                      key={child.href}
                      to={child.href}
                      className={cn(
                        'rounded-md px-2 py-1.5 text-xs font-medium transition-colors',
                        childActive
                          ? 'bg-sidebar-accent/80 text-sidebar-primary'
                          : 'text-sidebar-foreground/55 hover:bg-sidebar-accent/40 hover:text-sidebar-foreground',
                      )}
                    >
                      {child.label}
                    </Link>
                  )
                })}
              </div>
            ) : null}
          </div>
        )
      })}
    </div>
  )
}

export function LeftSidebar() {
  const { isExpanded, toggle } = useSidebar()
  const { token, signOut } = useAuth()
  const location = useLocation()
  const { projectId, base: projectBase } = useProjectPaths()
  const { escalations } = useProjectEscalations(projectId)
  const escalationCount = escalations.length

  const navigateItems: NavItem[] = projectBase
    ? [
        {
          label: 'Command Center',
          icon: LayoutDashboard,
          href: `${projectBase}/command`,
          match: `${projectBase}/command`,
          badgeCount: escalationCount,
        },
        {
          label: 'Tickets',
          icon: Ticket,
          href: `${projectBase}/tickets`,
          match: `${projectBase}/tickets`,
        },
        {
          label: 'Reviews',
          icon: ClipboardCheck,
          href: `${projectBase}/reviews`,
          match: `${projectBase}/reviews`,
        },
        {
          label: 'Code Reviews',
          icon: GitPullRequest,
          href: `${projectBase}/code-reviews`,
          match: `${projectBase}/code-reviews`,
        },
        {
          label: 'Sessions',
          icon: TerminalSquare,
          href: `${projectBase}/sessions`,
          match: `${projectBase}/sessions`,
        },
        {
          label: 'Work Streams',
          icon: Workflow,
          href: `${projectBase}/work-streams`,
          match: `${projectBase}/work-streams`,
        },
      ]
    : []


  const configureItems: NavItem[] = projectBase
    ? [
        {
          label: 'Settings',
          icon: Settings,
          href: `${projectBase}/settings`,
          match: `${projectBase}/settings`,
          children: [
            {
              label: 'Dispatch',
              icon: Settings,
              href: `${projectBase}/settings/dispatch`,
              match: `${projectBase}/settings/dispatch`,
            },
            {
              label: 'Workers and roles',
              icon: Settings,
              href: `${projectBase}/settings/workers`,
              match: `${projectBase}/settings/workers`,
            },
            {
              label: 'Workflow stages',
              icon: Settings,
              href: `${projectBase}/settings/workflow`,
              match: `${projectBase}/settings/workflow`,
            },
          ],
        },
      ]
    : []

  const currentPath = location.pathname

  return (
    <aside
      data-slot="left-sidebar"
      className={cn(
        'flex shrink-0 flex-col border-r border-sidebar-border bg-sidebar/90 backdrop-blur-lg transition-[width] duration-200 ease-in-out',
        isExpanded ? EXPANDED_WIDTH : COLLAPSED_WIDTH,
      )}
    >
      {/* Header */}
      <div
        className={cn(
          'flex h-12 items-center border-b border-sidebar-border',
          isExpanded ? 'justify-between px-3' : 'justify-center',
        )}
      >
        {isExpanded && (
          <Link
            to="/"
            className="flex items-center gap-2 text-sm font-semibold tracking-tight text-sidebar-foreground hover:text-sidebar-primary transition-colors"
          >
            <Activity className="size-4 text-sidebar-primary" />
            Flywheel
          </Link>
        )}
        <Button
          type="button"
          variant="ghost"
          size="icon-xs"
          onClick={toggle}
          aria-label={isExpanded ? 'Collapse sidebar' : 'Expand sidebar'}
          className="text-sidebar-foreground/60 hover:text-sidebar-foreground"
        >
          {isExpanded ? (
            <ChevronLeft className="size-4" />
          ) : (
            <ChevronRight className="size-4" />
          )}
        </Button>
      </div>

      {token && (
        <div className="border-b border-sidebar-border px-2 py-3">
          <ProjectSwitcher expanded={isExpanded} />
        </div>
      )}

      {/* Project context indicator */}
      {projectBase && isExpanded && (
        <div className="border-b border-sidebar-border px-3 py-2">
          <Link
            to={projectBase}
            className="text-xs text-muted-foreground hover:text-sidebar-foreground transition-colors truncate block"
          >
            Project
          </Link>
        </div>
      )}

      {/* Navigation */}
      <nav className="flex flex-1 flex-col gap-4 overflow-y-auto px-2 py-3">
        {token && navigateItems.length > 0 && (
          <NavSection
            title="Navigate"
            items={navigateItems}
            expanded={isExpanded}
            currentPath={currentPath}
          />
        )}
        {token && configureItems.length > 0 && (
          <NavSection
            title="Configure"
            items={configureItems}
            expanded={isExpanded}
            currentPath={currentPath}
          />
        )}
        {token && !projectBase && (
          <NavSection
            title="Navigate"
            items={[
              {
                label: 'Organizations',
                icon: Building2,
                href: '/orgs',
                match: '/orgs',
              },
            ]}
            expanded={isExpanded}
            currentPath={currentPath}
          />
        )}
      </nav>

      {/* Bottom pinned */}
      <div className="flex flex-col gap-1 border-t border-sidebar-border px-2 py-2">
        {token && projectBase && (
          <Link
            to="/orgs"
            className={cn(
              'group flex items-center gap-2.5 rounded-lg px-3 py-2 text-sm font-medium text-sidebar-foreground/70 transition-colors hover:bg-sidebar-accent/50 hover:text-sidebar-foreground',
              !isExpanded && 'justify-center px-0',
            )}
            title={isExpanded ? undefined : 'Organizations'}
          >
            <Building2 className="size-4 shrink-0" />
            {isExpanded && <span className="truncate">All organizations</span>}
          </Link>
        )}
        {token ? (
          <button
            type="button"
            onClick={signOut}
            className={cn(
              'group flex items-center gap-2.5 rounded-lg px-3 py-2 text-sm font-medium text-sidebar-foreground/70 transition-colors hover:bg-sidebar-accent/50 hover:text-sidebar-foreground',
              !isExpanded && 'justify-center px-0',
            )}
            title={isExpanded ? undefined : 'Sign out'}
          >
            <LogOut className="size-4 shrink-0" />
            {isExpanded && <span className="truncate">Sign out</span>}
          </button>
        ) : (
          <Button asChild size="sm" className={cn(!isExpanded && 'px-2')}>
            <a href="/auth/login">
              {isExpanded ? 'Sign in' : 'In'}
            </a>
          </Button>
        )}
      </div>
    </aside>
  )
}
