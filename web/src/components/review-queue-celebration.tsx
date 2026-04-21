import { motion } from 'framer-motion'
import { PartyPopper, Sparkles } from 'lucide-react'

import { Button } from '@/components/ui/button'

/** Green-themed confetti colors matching the app palette. */
const CONFETTI_COLORS = [
  'oklch(0.7 0.18 145)',   // emerald
  'oklch(0.75 0.15 155)',  // light green
  'oklch(0.65 0.2 140)',   // deep green
  'oklch(0.8 0.12 160)',   // mint
  'oklch(0.72 0.16 135)',  // forest
  'oklch(0.85 0.1 165)',   // pale green
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
    <motion.div
      initial={{ opacity: 0, scale: 0.95 }}
      animate={{ opacity: 1, scale: 1 }}
      transition={{ type: 'spring', stiffness: 300, damping: 25 }}
      className="relative mt-2 overflow-hidden rounded-xl border border-emerald-500/20 bg-gradient-to-br from-emerald-950/40 via-emerald-900/20 to-green-950/30 p-10 text-center backdrop-blur-md"
      role="status"
    >
      {/* Confetti particles */}
      <div
        className="pointer-events-none absolute inset-0 overflow-hidden"
        aria-hidden
      >
        {Array.from({ length: 28 }, (_, i) => (
          <motion.span
            key={i}
            className="absolute size-2 rounded-full"
            style={{
              left: `${(i * 37 + i * 7) % 96}%`,
              backgroundColor: CONFETTI_COLORS[i % CONFETTI_COLORS.length],
            }}
            initial={{ y: -10, opacity: 0, rotate: 0, scale: 0.5 }}
            animate={{
              y: 350,
              opacity: [0, 1, 1, 0],
              rotate: 720 + (i % 3) * 180,
              scale: [0.5, 1, 0.8, 0.3],
            }}
            transition={{
              duration: 1.4 + (i % 5) * 0.15,
              delay: i * 0.04,
              ease: 'easeIn',
            }}
          />
        ))}
      </div>

      {/* Radial glow behind icon */}
      <div className="pointer-events-none absolute inset-0 flex items-center justify-center" aria-hidden>
        <motion.div
          className="size-32 rounded-full bg-emerald-500/10 blur-3xl"
          initial={{ scale: 0 }}
          animate={{ scale: [0, 1.2, 1] }}
          transition={{ duration: 1, ease: 'easeOut' }}
        />
      </div>

      {/* Icon */}
      <motion.div
        initial={{ scale: 0, rotate: -20 }}
        animate={{ scale: 1, rotate: 0 }}
        transition={{
          type: 'spring',
          stiffness: 300,
          damping: 15,
          delay: 0.1,
        }}
        className="relative mx-auto"
      >
        <PartyPopper
          className="mx-auto size-14 text-emerald-400"
          strokeWidth={1.25}
        />
      </motion.div>

      {/* Heading */}
      <motion.h2
        initial={{ opacity: 0, y: 10 }}
        animate={{ opacity: 1, y: 0 }}
        transition={{ delay: 0.2, duration: 0.4 }}
        className="mt-5 text-xl font-semibold tracking-tight text-foreground"
      >
        You&apos;re all caught up!
      </motion.h2>

      {extraLead ? (
        <motion.p
          initial={{ opacity: 0 }}
          animate={{ opacity: 1 }}
          transition={{ delay: 0.3 }}
          className="mx-auto mt-3 max-w-sm text-sm text-emerald-300/70"
        >
          {extraLead}
        </motion.p>
      ) : null}

      <motion.p
        initial={{ opacity: 0 }}
        animate={{ opacity: 1 }}
        transition={{ delay: 0.35 }}
        className="mx-auto mt-2 max-w-sm text-sm text-muted-foreground"
      >
        Nothing left in the review queue for this project.
      </motion.p>

      {/* Dismiss button with sparkle animation */}
      <motion.div
        initial={{ opacity: 0, y: 10 }}
        animate={{ opacity: 1, y: 0 }}
        transition={{ delay: 0.5, duration: 0.3 }}
        className="mt-8 flex justify-center"
      >
        <Button
          type="button"
          variant="outline"
          size="sm"
          onClick={onDismiss}
          className="border-emerald-500/30 bg-emerald-500/10 text-emerald-400 hover:bg-emerald-500/20"
        >
          <Sparkles className="mr-1.5 size-3.5" />
          Nice
        </Button>
      </motion.div>
    </motion.div>
  )
}
