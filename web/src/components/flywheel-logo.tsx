import { cn } from '@/lib/utils'

type FlywheelLogoProps = {
  className?: string
  showWordmark?: boolean
}

/**
 * Flywheel logo: a stylized spinning gear/flywheel icon + optional wordmark.
 * Uses current theme accent color.
 */
export function FlywheelLogo({ className, showWordmark = true }: FlywheelLogoProps) {
  return (
    <span className={cn('inline-flex items-center gap-2', className)}>
      {/* Flywheel icon — concentric arcs suggesting rotation/momentum */}
      <svg
        viewBox="0 0 24 24"
        fill="none"
        className="size-6 shrink-0"
        aria-hidden="true"
      >
        {/* Outer ring */}
        <circle
          cx="12"
          cy="12"
          r="10"
          stroke="currentColor"
          strokeWidth="1.5"
          className="text-primary/40"
        />
        {/* Three momentum arcs — staggered around the wheel */}
        <path
          d="M12 2 A10 10 0 0 1 20.66 7"
          stroke="url(#fw-grad-1)"
          strokeWidth="2.5"
          strokeLinecap="round"
          className="animate-[spin_8s_linear_infinite] origin-center"
        />
        <path
          d="M20.66 17 A10 10 0 0 1 7 20.66"
          stroke="url(#fw-grad-2)"
          strokeWidth="2.5"
          strokeLinecap="round"
          className="animate-[spin_8s_linear_infinite] origin-center"
        />
        <path
          d="M3.34 7 A10 10 0 0 1 12 2"
          stroke="url(#fw-grad-3)"
          strokeWidth="2.5"
          strokeLinecap="round"
          className="animate-[spin_12s_linear_infinite_reverse] origin-center"
          style={{ animationDelay: '-2s' }}
        />
        {/* Center hub */}
        <circle cx="12" cy="12" r="3" className="fill-primary" />
        <circle cx="12" cy="12" r="1.5" className="fill-background" />
        {/* Gradient defs */}
        <defs>
          <linearGradient id="fw-grad-1" x1="0" y1="0" x2="1" y2="1">
            <stop offset="0%" className="[stop-color:oklch(0.72_0.19_145)]" />
            <stop offset="100%" className="[stop-color:oklch(0.55_0.16_145)]" />
          </linearGradient>
          <linearGradient id="fw-grad-2" x1="1" y1="0" x2="0" y2="1">
            <stop offset="0%" className="[stop-color:oklch(0.65_0.17_145)]" />
            <stop offset="100%" className="[stop-color:oklch(0.50_0.14_145)]" />
          </linearGradient>
          <linearGradient id="fw-grad-3" x1="0" y1="1" x2="1" y2="0">
            <stop offset="0%" className="[stop-color:oklch(0.60_0.15_145)]" />
            <stop offset="100%" className="[stop-color:oklch(0.72_0.19_145)]" />
          </linearGradient>
        </defs>
      </svg>
      {showWordmark && (
        <span className="text-base font-semibold tracking-tight text-foreground">
          Flywheel
        </span>
      )}
    </span>
  )
}
