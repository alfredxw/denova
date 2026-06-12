import type { Variants } from 'motion/react'

export const novaSpring = {
  type: 'spring',
  stiffness: 430,
  damping: 34,
  mass: 0.8,
} as const

export const novaEase = [0.25, 1, 0.4, 1] as const

export const panelPresence: Variants = {
  initial: { opacity: 0 },
  animate: { opacity: 1 },
  exit: { opacity: 0 },
}

export const subtlePresence: Variants = {
  initial: { opacity: 0 },
  animate: { opacity: 1 },
  exit: { opacity: 0 },
}

export const listContainer: Variants = {
  animate: {
    transition: {
      staggerChildren: 0.018,
      delayChildren: 0.015,
    },
  },
}

export const listItem: Variants = {
  initial: { opacity: 0, y: 5 },
  animate: { opacity: 1, y: 0 },
  exit: { opacity: 0, y: -4 },
}
