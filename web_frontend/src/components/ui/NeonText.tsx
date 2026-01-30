import { motion } from 'framer-motion'
import { useReducedMotion } from '../../hooks/useReducedMotion'

interface NeonTextProps {
  children: React.ReactNode
  color?: 'purple' | 'cyan' | 'pink' | 'green' | 'blue'
  flicker?: boolean
  className?: string
  as?: 'span' | 'p' | 'h1' | 'h2' | 'h3' | 'h4'
}

const colorConfig = {
  purple: {
    text: 'text-purple-400',
    glow: 'rgba(168, 85, 247, 0.8)',
    glowStrong: 'rgba(168, 85, 247, 1)',
  },
  cyan: {
    text: 'text-cyan-400',
    glow: 'rgba(6, 182, 212, 0.8)',
    glowStrong: 'rgba(6, 182, 212, 1)',
  },
  pink: {
    text: 'text-pink-400',
    glow: 'rgba(236, 72, 153, 0.8)',
    glowStrong: 'rgba(236, 72, 153, 1)',
  },
  green: {
    text: 'text-emerald-400',
    glow: 'rgba(52, 211, 153, 0.8)',
    glowStrong: 'rgba(52, 211, 153, 1)',
  },
  blue: {
    text: 'text-blue-400',
    glow: 'rgba(59, 130, 246, 0.8)',
    glowStrong: 'rgba(59, 130, 246, 1)',
  },
}

export function NeonText({
  children,
  color = 'purple',
  flicker = false,
  className = '',
  as = 'span',
}: NeonTextProps) {
  const config = colorConfig[color]
  const prefersReducedMotion = useReducedMotion()

  const staticGlow = `0 0 10px ${config.glow}, 0 0 20px ${config.glow}, 0 0 40px ${config.glow}`

  const flickerAnimation =
    flicker && !prefersReducedMotion
      ? {
          textShadow: [
            `0 0 10px ${config.glow}, 0 0 20px ${config.glow}, 0 0 40px ${config.glow}`,
            `0 0 5px ${config.glow}, 0 0 10px ${config.glow}`,
            `0 0 10px ${config.glow}, 0 0 20px ${config.glow}, 0 0 40px ${config.glow}`,
            `0 0 15px ${config.glowStrong}, 0 0 30px ${config.glowStrong}, 0 0 60px ${config.glowStrong}`,
            `0 0 10px ${config.glow}, 0 0 20px ${config.glow}, 0 0 40px ${config.glow}`,
          ],
        }
      : {}

  const MotionComponent = motion[as]

  return (
    <MotionComponent
      className={`${config.text} ${className}`}
      style={!flicker || prefersReducedMotion ? { textShadow: staticGlow } : undefined}
      animate={flickerAnimation}
      transition={flicker && !prefersReducedMotion ? { duration: 2, repeat: Infinity } : undefined}
    >
      {children}
    </MotionComponent>
  )
}
