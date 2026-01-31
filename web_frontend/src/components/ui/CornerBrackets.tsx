import { memo, useMemo } from 'react'
import { motion, type Variants } from 'framer-motion'
import { useReducedMotion } from '../../hooks/useReducedMotion'

interface CornerBracketsProps {
  size?: number
  strokeWidth?: number
  color?: string
  glowColor?: string
  animated?: boolean
  className?: string
}

interface CornerProps {
  position: 'tl' | 'tr' | 'bl' | 'br'
  size: number
  strokeWidth: number
  color: string
  shouldAnimate: boolean
  pathVariants: Variants
  pulseVariants: Variants
}

// Static path variants (don't depend on props)
const pathVariants: Variants = {
  hidden: { pathLength: 0, opacity: 0 },
  visible: {
    pathLength: 1,
    opacity: 1,
    transition: {
      pathLength: { duration: 0.8, ease: 'easeInOut' as const },
      opacity: { duration: 0.2 },
    },
  },
}

const rotation = {
  tl: 0,
  tr: 90,
  bl: 270,
  br: 180,
}

const positionClass = {
  tl: 'top-0 left-0',
  tr: 'top-0 right-0',
  bl: 'bottom-0 left-0',
  br: 'bottom-0 right-0',
}

const Corner = memo(function Corner({
  position,
  size,
  strokeWidth,
  color,
  shouldAnimate,
  pathVariants,
  pulseVariants,
}: CornerProps) {
  return (
    <motion.svg
      width={size}
      height={size}
      viewBox="0 0 20 20"
      fill="none"
      className={`absolute ${positionClass[position]}`}
      style={{ transform: `rotate(${rotation[position]}deg)` }}
      variants={shouldAnimate ? pulseVariants : {}}
      animate={shouldAnimate ? 'pulse' : undefined}
    >
      <motion.path
        d="M2 18V8C2 4.68629 4.68629 2 8 2H18"
        stroke={color}
        strokeWidth={strokeWidth}
        strokeLinecap="round"
        fill="none"
        variants={shouldAnimate ? pathVariants : {}}
        initial={shouldAnimate ? 'hidden' : 'visible'}
        animate="visible"
      />
    </motion.svg>
  )
})

export const CornerBrackets = memo(function CornerBrackets({
  size = 20,
  strokeWidth = 2,
  color = '#a855f7',
  glowColor = 'rgba(168, 85, 247, 0.6)',
  animated = true,
  className = '',
}: CornerBracketsProps) {
  const prefersReducedMotion = useReducedMotion()
  const shouldAnimate = animated && !prefersReducedMotion

  const pulseVariants = useMemo<Variants>(
    () => ({
      pulse: {
        filter: [
          `drop-shadow(0 0 2px ${glowColor})`,
          `drop-shadow(0 0 8px ${glowColor})`,
          `drop-shadow(0 0 2px ${glowColor})`,
        ],
        transition: {
          duration: 2,
          repeat: Infinity,
          ease: 'easeInOut' as const,
        },
      },
    }),
    [glowColor],
  )

  return (
    <div className={`pointer-events-none absolute inset-0 ${className}`}>
      <Corner
        position="tl"
        size={size}
        strokeWidth={strokeWidth}
        color={color}
        shouldAnimate={shouldAnimate}
        pathVariants={pathVariants}
        pulseVariants={pulseVariants}
      />
      <Corner
        position="tr"
        size={size}
        strokeWidth={strokeWidth}
        color={color}
        shouldAnimate={shouldAnimate}
        pathVariants={pathVariants}
        pulseVariants={pulseVariants}
      />
      <Corner
        position="bl"
        size={size}
        strokeWidth={strokeWidth}
        color={color}
        shouldAnimate={shouldAnimate}
        pathVariants={pathVariants}
        pulseVariants={pulseVariants}
      />
      <Corner
        position="br"
        size={size}
        strokeWidth={strokeWidth}
        color={color}
        shouldAnimate={shouldAnimate}
        pathVariants={pathVariants}
        pulseVariants={pulseVariants}
      />
    </div>
  )
})

// Variant with marching dashes
interface MarchingCornerProps {
  position: 'tl' | 'tr' | 'bl' | 'br'
  size: number
  strokeWidth: number
  color: string
  prefersReducedMotion: boolean
}

const MarchingCorner = memo(function MarchingCorner({
  position,
  size,
  strokeWidth,
  color,
  prefersReducedMotion,
}: MarchingCornerProps) {
  return (
    <svg
      width={size}
      height={size}
      viewBox="0 0 20 20"
      fill="none"
      className={`absolute ${positionClass[position]}`}
      style={{ transform: `rotate(${rotation[position]}deg)` }}
    >
      <path
        d="M2 18V8C2 4.68629 4.68629 2 8 2H18"
        stroke={color}
        strokeWidth={strokeWidth}
        strokeLinecap="round"
        strokeDasharray="4 4"
        fill="none"
        style={{
          animation: prefersReducedMotion ? 'none' : 'marchingAnts 1s linear infinite',
        }}
      />
    </svg>
  )
})

export const CornerBracketsMarching = memo(function CornerBracketsMarching({
  size = 20,
  strokeWidth = 2,
  color = '#a855f7',
  className = '',
}: Omit<CornerBracketsProps, 'animated' | 'glowColor'>) {
  const prefersReducedMotion = useReducedMotion()

  return (
    <div className={`pointer-events-none absolute inset-0 ${className}`}>
      <MarchingCorner
        position="tl"
        size={size}
        strokeWidth={strokeWidth}
        color={color}
        prefersReducedMotion={prefersReducedMotion}
      />
      <MarchingCorner
        position="tr"
        size={size}
        strokeWidth={strokeWidth}
        color={color}
        prefersReducedMotion={prefersReducedMotion}
      />
      <MarchingCorner
        position="bl"
        size={size}
        strokeWidth={strokeWidth}
        color={color}
        prefersReducedMotion={prefersReducedMotion}
      />
      <MarchingCorner
        position="br"
        size={size}
        strokeWidth={strokeWidth}
        color={color}
        prefersReducedMotion={prefersReducedMotion}
      />
    </div>
  )
})
