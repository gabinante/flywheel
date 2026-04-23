import { ChevronRight } from 'lucide-react'
import { Link } from 'react-router-dom'

import { useRouteBreadcrumbs } from '@/hooks/use-route-breadcrumbs'
import { cn } from '@/lib/utils'

/**
 * Contextual breadcrumb trail for the app header.
 * Reads the current route and renders clickable segments.
 */
export function HeaderBreadcrumbs({ className }: { className?: string }) {
  const crumbs = useRouteBreadcrumbs()

  if (crumbs.length === 0) return null

  return (
    <nav aria-label="Breadcrumb" className={cn('flex items-center gap-1 text-sm', className)}>
      {crumbs.map((crumb, i) => {
        const isLast = i === crumbs.length - 1
        return (
          <span key={`${crumb.label}-${i}`} className="flex items-center gap-1">
            {i > 0 && (
              <ChevronRight className="size-3.5 shrink-0 text-muted-foreground/50" />
            )}
            {crumb.href && !isLast ? (
              <Link
                to={crumb.href}
                className="text-muted-foreground transition-colors duration-150 hover:text-foreground"
              >
                {crumb.label}
              </Link>
            ) : (
              <span
                className={cn(
                  'truncate',
                  isLast
                    ? 'max-w-[200px] font-medium text-foreground'
                    : 'text-muted-foreground',
                )}
              >
                {crumb.label}
              </span>
            )}
          </span>
        )
      })}
    </nav>
  )
}
