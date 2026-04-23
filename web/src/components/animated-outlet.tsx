import { AnimatePresence, type Variants, motion } from 'framer-motion'
import { useLocation, useOutlet } from 'react-router-dom'
import * as React from 'react'

const ease = [0.25, 0.1, 0.25, 1] as const

const pageVariants: Variants = {
  initial: {
    opacity: 0,
    y: 8,
  },
  enter: {
    opacity: 1,
    y: 0,
    transition: {
      duration: 0.25,
      ease,
    },
  },
  exit: {
    opacity: 0,
    y: -8,
    transition: {
      duration: 0.15,
      ease,
    },
  },
}

/**
 * Replaces <Outlet /> with an animated version that cross-fades
 * between route changes using framer-motion's AnimatePresence.
 */
export function AnimatedOutlet() {
  const location = useLocation()
  const outlet = useOutlet()

  return (
    <AnimatePresence mode="wait">
      <motion.div
        key={location.pathname}
        variants={pageVariants}
        initial="initial"
        animate="enter"
        exit="exit"
      >
        {/* Freeze the outlet so the exiting component doesn't re-render with new route data */}
        <FrozenOutlet>{outlet}</FrozenOutlet>
      </motion.div>
    </AnimatePresence>
  )
}

/**
 * Freezes the outlet content during exit animation to prevent
 * the stale route from re-rendering while fading out.
 */
function FrozenOutlet({ children }: { children: React.ReactNode }) {
  const frozen = React.useRef(children)
  // Update the frozen ref only when new content arrives (not on unmount)
  React.useEffect(() => {
    if (children) {
      frozen.current = children
    }
  }, [children])
  return <>{children ?? frozen.current}</>
}
