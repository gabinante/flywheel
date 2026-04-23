import { type Variants, motion } from 'framer-motion'
import * as React from 'react'

const ease = [0.25, 0.1, 0.25, 1] as const

const containerVariants: Variants = {
  hidden: {},
  visible: {
    transition: {
      staggerChildren: 0.05,
      delayChildren: 0.02,
    },
  },
}

const itemVariants: Variants = {
  hidden: {
    opacity: 0,
    y: 12,
  },
  visible: {
    opacity: 1,
    y: 0,
    transition: {
      duration: 0.25,
      ease,
    },
  },
}

/**
 * Stagger-animated list container. Wraps <ul> with framer-motion.
 * Each direct child should be wrapped with <StaggerItem>.
 */
export function StaggerList({
  className,
  children,
  ...props
}: React.ComponentProps<typeof motion.ul>) {
  return (
    <motion.ul
      variants={containerVariants}
      initial="hidden"
      animate="visible"
      className={className}
      {...props}
    >
      {children}
    </motion.ul>
  )
}

/**
 * A single stagger-animated list item. Must be a child of <StaggerList>.
 */
export function StaggerItem({
  className,
  children,
  ...props
}: React.ComponentProps<typeof motion.li>) {
  return (
    <motion.li variants={itemVariants} className={className} {...props}>
      {children}
    </motion.li>
  )
}
