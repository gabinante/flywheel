import { cn } from '@/lib/utils'

type ProgressProps = {
  value: number
  max?: number
  className?: string
  barClassName?: string
}

function Progress({ value, max = 100, className, barClassName }: ProgressProps) {
  const pct = max > 0 ? Math.min(100, Math.max(0, (value / max) * 100)) : 0

  return (
    <div
      role="progressbar"
      aria-valuenow={value}
      aria-valuemin={0}
      aria-valuemax={max}
      className={cn(
        'h-1.5 w-full overflow-hidden rounded-full bg-white/10',
        className,
      )}
    >
      <div
        className={cn(
          'h-full rounded-full bg-primary/70 transition-all duration-500 ease-out',
          barClassName,
        )}
        style={{ width: `${pct}%` }}
      />
    </div>
  )
}

export { Progress }
