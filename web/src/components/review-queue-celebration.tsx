import { PartyPopper } from 'lucide-react'

import { Button } from '@/components/ui/button'

const CONFETTI_COLORS = [
  'oklch(0.7 0.2 40)',
  'oklch(0.75 0.18 85)',
  'oklch(0.72 0.19 145)',
  'oklch(0.65 0.18 250)',
  'oklch(0.7 0.22 310)',
]

type ReviewQueueCelebrationProps = {
  onDismiss: () => void
  /** Optional line above the standard queue-empty copy (e.g. ticket marked done). */
  extraLead?: string
}

export function ReviewQueueCelebration({
  onDismiss,
  extraLead,
}: ReviewQueueCelebrationProps) {
  return (
    <div
      className="relative mt-2 overflow-hidden rounded-2xl border border-white/10 bg-gradient-to-br from-emerald-950/30 via-amber-950/20 to-orange-950/20 p-10 text-center backdrop-blur-md"
      role="status"
    >
      <div className="pointer-events-none absolute inset-0 overflow-hidden" aria-hidden>
        {Array.from({ length: 22 }, (_, i) => (
          <span
            key={i}
            className="absolute size-2.5 rounded-full opacity-90"
            style={{
              left: `${(i * 37) % 92}%`,
              top: '8%',
              backgroundColor: CONFETTI_COLORS[i % CONFETTI_COLORS.length],
              animation: `reviews-confetti-fall ${1.1 + (i % 6) * 0.12}s ease-in forwards`,
              animationDelay: `${i * 0.045}s`,
            }}
          />
        ))}
      </div>
      <PartyPopper
        className="text-foreground mx-auto size-14"
        strokeWidth={1.25}
        style={{
          animation:
            'reviews-celebrate-pop 0.75s cubic-bezier(0.34, 1.56, 0.64, 1) forwards',
        }}
        aria-hidden
      />
      <h2
        className="text-foreground mt-5 text-xl font-semibold tracking-tight"
        style={{
          animation:
            'reviews-celebrate-pop 0.75s cubic-bezier(0.34, 1.56, 0.64, 1) 0.08s both',
        }}
      >
        You&apos;re all caught up!
      </h2>
      {extraLead ? (
        <p className="text-muted-foreground mx-auto mt-3 max-w-sm text-sm">
          {extraLead}
        </p>
      ) : null}
      <p className="text-muted-foreground mx-auto mt-2 max-w-sm text-sm">
        Nothing left in the review queue for this project.
      </p>
      <div
        className="mt-8 flex justify-center"
        style={{ animation: 'reviews-sparkle 2.5s ease-in-out infinite' }}
      >
        <Button type="button" variant="outline" size="sm" onClick={onDismiss}>
          Nice
        </Button>
      </div>
    </div>
  )
}
