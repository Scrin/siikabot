import { motion } from 'framer-motion'
import { useEffect, useState } from 'react'
import { useReducedMotion } from '../../hooks/useReducedMotion'

interface GlitchTextProps {
  text: string
  className?: string
  glitchOnHover?: boolean
  continuous?: boolean
}

export function GlitchText({
  text,
  className = '',
  glitchOnHover = false,
  continuous = false,
}: GlitchTextProps) {
  const [isGlitching, setIsGlitching] = useState(false)
  const prefersReducedMotion = useReducedMotion()

  // Random glitch intervals for continuous mode
  useEffect(() => {
    if (!continuous || prefersReducedMotion) return

    const triggerGlitch = () => {
      setIsGlitching(true)
      setTimeout(() => setIsGlitching(false), 200)
    }

    // Initial glitch after a delay
    const initialDelay = setTimeout(triggerGlitch, 2000)

    // Random interval glitches
    const interval = setInterval(
      () => {
        triggerGlitch()
      },
      4000 + Math.random() * 5000,
    )

    return () => {
      clearTimeout(initialDelay)
      clearInterval(interval)
    }
  }, [continuous, prefersReducedMotion])

  if (prefersReducedMotion) {
    return <span className={className}>{text}</span>
  }

  return (
    <span
      className={`relative inline-block ${className}`}
      onMouseEnter={() => glitchOnHover && setIsGlitching(true)}
      onMouseLeave={() => glitchOnHover && setIsGlitching(false)}
    >
      {/* Base text - always visible */}
      {text}

      {/* Glitch layers - only shown during glitch effect */}
      {isGlitching && (
        <>
          {/* Cyan layer - offset left */}
          <motion.span
            className="pointer-events-none absolute inset-0"
            style={{
              color: '#22d3ee',
              clipPath: 'polygon(0 0, 100% 0, 100% 45%, 0 45%)',
              textShadow: '0 0 10px #22d3ee',
            }}
            initial={{ x: 0, opacity: 0 }}
            animate={{
              x: [0, -4, 3, -2, 4, 0],
              opacity: [0, 0.8, 0.8, 0.8, 0.8, 0],
            }}
            transition={{ duration: 0.2, ease: 'easeInOut' }}
          >
            {text}
          </motion.span>

          {/* Red/magenta layer - offset right */}
          <motion.span
            className="pointer-events-none absolute inset-0"
            style={{
              color: '#f472b6',
              clipPath: 'polygon(0 55%, 100% 55%, 100% 100%, 0 100%)',
              textShadow: '0 0 10px #f472b6',
            }}
            initial={{ x: 0, opacity: 0 }}
            animate={{
              x: [0, 4, -3, 2, -4, 0],
              opacity: [0, 0.8, 0.8, 0.8, 0.8, 0],
            }}
            transition={{ duration: 0.2, ease: 'easeInOut' }}
          >
            {text}
          </motion.span>
        </>
      )}
    </span>
  )
}
