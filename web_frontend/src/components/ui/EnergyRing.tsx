import { motion } from 'framer-motion'
import { useReducedMotion } from '../../hooks/useReducedMotion'

interface EnergyRingProps {
  size?: number
  color?: string
  rings?: number
  className?: string
  speed?: 'slow' | 'normal' | 'fast'
}

export function EnergyRing({
  size = 40,
  color = '#a855f7',
  rings = 3,
  className = '',
  speed = 'normal',
}: EnergyRingProps) {
  const prefersReducedMotion = useReducedMotion()

  const speedMultiplier = {
    slow: 1.5,
    normal: 1,
    fast: 0.6,
  }

  const baseDuration = 2 * speedMultiplier[speed]

  if (prefersReducedMotion) {
    return (
      <div
        className={`relative ${className}`}
        style={{ width: size, height: size }}
      >
        <svg width={size} height={size} viewBox="0 0 40 40">
          <circle
            cx="20"
            cy="20"
            r="18"
            stroke={color}
            strokeWidth="1"
            fill="none"
            opacity="0.5"
          />
        </svg>
      </div>
    )
  }

  return (
    <div
      className={`relative ${className}`}
      style={{ width: size, height: size }}
    >
      {Array.from({ length: rings }).map((_, i) => (
        <motion.div
          key={i}
          className="absolute inset-0"
          initial={{ scale: 0.5, opacity: 1 }}
          animate={{
            scale: [0.5, 1.5],
            opacity: [0.8, 0],
          }}
          transition={{
            duration: baseDuration,
            repeat: Infinity,
            delay: i * (baseDuration / rings),
            ease: 'easeOut',
          }}
        >
          <svg width="100%" height="100%" viewBox="0 0 40 40">
            <circle
              cx="20"
              cy="20"
              r="18"
              stroke={color}
              strokeWidth="2"
              fill="none"
              style={{
                filter: `drop-shadow(0 0 4px ${color})`,
              }}
            />
          </svg>
        </motion.div>
      ))}

      {/* Static center ring */}
      <motion.svg
        width="100%"
        height="100%"
        viewBox="0 0 40 40"
        className="absolute inset-0"
        animate={{
          rotate: 360,
        }}
        transition={{
          duration: baseDuration * 4,
          repeat: Infinity,
          ease: 'linear',
        }}
      >
        <circle
          cx="20"
          cy="20"
          r="8"
          stroke={color}
          strokeWidth="1.5"
          strokeDasharray="4 4"
          fill="none"
          opacity="0.8"
        />
      </motion.svg>

      {/* Pulsing center dot */}
      <motion.div
        className="absolute"
        style={{
          top: '50%',
          left: '50%',
          transform: 'translate(-50%, -50%)',
          width: size * 0.15,
          height: size * 0.15,
          borderRadius: '50%',
          backgroundColor: color,
          boxShadow: `0 0 10px ${color}, 0 0 20px ${color}`,
        }}
        animate={{
          scale: [1, 1.3, 1],
          opacity: [1, 0.7, 1],
        }}
        transition={{
          duration: baseDuration * 0.5,
          repeat: Infinity,
          ease: 'easeInOut',
        }}
      />
    </div>
  )
}

// Variant for status indicators
interface StatusEnergyRingProps extends Omit<EnergyRingProps, 'color'> {
  status: 'ok' | 'degraded' | 'error' | 'unknown'
}

export function StatusEnergyRing({ status, ...props }: StatusEnergyRingProps) {
  const colorMap = {
    ok: '#10b981', // emerald
    degraded: '#f59e0b', // amber
    error: '#ef4444', // red
    unknown: '#6b7280', // gray
  }

  return <EnergyRing color={colorMap[status]} {...props} />
}
