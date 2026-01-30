import type { Variants, Transition } from 'framer-motion'

// Spring configurations for natural movement
export const springConfig = {
  gentle: { type: 'spring', stiffness: 120, damping: 14 } as Transition,
  bouncy: { type: 'spring', stiffness: 300, damping: 10 } as Transition,
  stiff: { type: 'spring', stiffness: 400, damping: 30 } as Transition,
  wobbly: { type: 'spring', stiffness: 180, damping: 12 } as Transition,
} as const

// Fade and slide variants
export const fadeInUp: Variants = {
  hidden: { opacity: 0, y: 20, filter: 'blur(10px)' },
  visible: {
    opacity: 1,
    y: 0,
    filter: 'blur(0px)',
    transition: { type: 'spring', stiffness: 120, damping: 14 },
  },
}

export const fadeInScale: Variants = {
  hidden: { opacity: 0, scale: 0.9, filter: 'blur(10px)' },
  visible: {
    opacity: 1,
    scale: 1,
    filter: 'blur(0px)',
    transition: { type: 'spring', stiffness: 120, damping: 14 },
  },
}

export const fadeInLeft: Variants = {
  hidden: { opacity: 0, x: -20, filter: 'blur(10px)' },
  visible: {
    opacity: 1,
    x: 0,
    filter: 'blur(0px)',
    transition: { type: 'spring', stiffness: 120, damping: 14 },
  },
}

// Stagger container for lists
export const staggerContainer: Variants = {
  hidden: { opacity: 0 },
  visible: {
    opacity: 1,
    transition: {
      staggerChildren: 0.08,
      delayChildren: 0.1,
    },
  },
}

// List item variants
export const listItem: Variants = {
  hidden: {
    opacity: 0,
    x: -20,
    filter: 'blur(10px)',
  },
  visible: {
    opacity: 1,
    x: 0,
    filter: 'blur(0px)',
    transition: {
      type: 'spring',
      stiffness: 100,
      damping: 12,
    },
  },
  exit: {
    opacity: 0,
    x: 20,
    filter: 'blur(10px)',
    transition: { duration: 0.2 },
  },
}

// Glitch effect variants
export const glitchVariants: Variants = {
  idle: { x: 0, skewX: 0 },
  glitch: {
    x: [0, -3, 3, -1, 1, 0],
    skewX: [0, 2, -2, 1, -1, 0],
    transition: { duration: 0.4, ease: 'easeInOut' },
  },
}

// Scale on hover
export const hoverScale: Variants = {
  rest: { scale: 1 },
  hover: { scale: 1.02, transition: { type: 'spring', stiffness: 400, damping: 17 } },
  tap: { scale: 0.98 },
}

// Card entrance animation
export const cardEntrance: Variants = {
  hidden: {
    opacity: 0,
    y: 30,
    scale: 0.95,
    filter: 'blur(20px)',
  },
  visible: {
    opacity: 1,
    y: 0,
    scale: 1,
    filter: 'blur(0px)',
    transition: {
      type: 'spring',
      stiffness: 100,
      damping: 15,
      duration: 0.6,
    },
  },
}

// Neon pulse animation (for use with animate prop)
export const neonPulseAnimation = {
  boxShadow: [
    '0 0 5px currentColor, 0 0 10px currentColor, 0 0 20px currentColor',
    '0 0 10px currentColor, 0 0 20px currentColor, 0 0 40px currentColor, 0 0 60px currentColor',
    '0 0 5px currentColor, 0 0 10px currentColor, 0 0 20px currentColor',
  ],
}

// Floating animation
export const floatAnimation = {
  y: [0, -10, 0],
  transition: {
    duration: 6,
    repeat: Infinity,
    ease: 'easeInOut',
  },
}

// Gradient text animation
export const gradientShift = {
  backgroundPosition: ['0% 50%', '100% 50%', '0% 50%'],
  transition: {
    duration: 5,
    repeat: Infinity,
    ease: 'linear',
  },
}

// Section reveal (for scroll-triggered animations)
export const sectionReveal: Variants = {
  hidden: {
    opacity: 0,
    y: 40,
    filter: 'blur(10px)',
  },
  visible: {
    opacity: 1,
    y: 0,
    filter: 'blur(0px)',
    transition: {
      duration: 0.6,
      ease: [0.25, 0.1, 0.25, 1],
    },
  },
}

// Status badge transition
export const statusTransition: Variants = {
  initial: { opacity: 0, scale: 0.8, filter: 'blur(4px)' },
  animate: { opacity: 1, scale: 1, filter: 'blur(0px)' },
  exit: { opacity: 0, scale: 0.8, filter: 'blur(4px)' },
}
