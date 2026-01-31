import { motion, useMotionValue, useSpring, useTransform } from 'framer-motion'
import { useRef, useEffect, type ReactNode } from 'react'
import { useReducedMotion } from '../../hooks/useReducedMotion'
import { useReducedEffects } from '../../context/ReducedEffectsContext'

interface TiltCardProps {
  children: ReactNode
  className?: string
  glowColor?: string
  /** Track mouse globally (whole window) instead of just when hovering over the card */
  trackGlobally?: boolean
  /** Enable holographic/prismatic color shift effect */
  holographic?: boolean
}

export function TiltCard({
  children,
  className = '',
  glowColor = 'rgba(168, 85, 247, 0.3)',
  trackGlobally = true,
  holographic = false,
}: TiltCardProps) {
  const cardRef = useRef<HTMLDivElement>(null)
  const prefersReducedMotion = useReducedMotion()
  const { isReduced } = useReducedEffects()
  const shouldReduceMotion = prefersReducedMotion || isReduced

  const x = useMotionValue(0)
  const y = useMotionValue(0)

  // Smooth spring-based rotation
  const rotateX = useSpring(useTransform(y, [-0.5, 0.5], [6, -6]), {
    stiffness: 150,
    damping: 20,
  })
  const rotateY = useSpring(useTransform(x, [-0.5, 0.5], [-6, 6]), {
    stiffness: 150,
    damping: 20,
  })

  // Glow position based on mouse
  const glowX = useTransform(x, [-0.5, 0.5], ['0%', '100%'])
  const glowY = useTransform(y, [-0.5, 0.5], ['0%', '100%'])

  // Holographic transforms
  const holoAngle = useTransform(x, [-0.5, 0.5], [135, 195])
  const holoHue = useTransform(x, [-0.5, 0.5], [0, 60])
  const shimmerPos = useTransform(x, [-0.5, 0.5], [0, 100])

  // Pre-compute all derived transforms at the top level (hooks must be called unconditionally)
  const glowBackground = useTransform(
    [glowX, glowY] as const,
    ([gx, gy]: string[]) =>
      `radial-gradient(600px circle at ${gx} ${gy}, ${glowColor}, transparent 40%)`,
  )

  const holoBackground = useTransform(
    holoAngle,
    (angle) =>
      `linear-gradient(
        ${angle}deg,
        rgba(168, 85, 247, 0.15) 0%,
        rgba(59, 130, 246, 0.15) 25%,
        rgba(6, 182, 212, 0.15) 50%,
        rgba(236, 72, 153, 0.15) 75%,
        rgba(168, 85, 247, 0.15) 100%
      )`,
  )

  const holoFilter = useTransform(holoHue, (hue) => `hue-rotate(${hue}deg)`)

  const rainbowBackground = useTransform(
    [glowX, glowY] as const,
    ([gx, gy]: string[]) =>
      `conic-gradient(
        from ${parseFloat(gx) * 3.6}deg at ${gx} ${gy},
        transparent 0deg,
        rgba(255, 0, 0, 0.1) 60deg,
        rgba(255, 255, 0, 0.1) 120deg,
        rgba(0, 255, 0, 0.1) 180deg,
        rgba(0, 255, 255, 0.1) 240deg,
        rgba(0, 0, 255, 0.1) 300deg,
        transparent 360deg
      )`,
  )

  const shimmerBackground = useTransform(
    shimmerPos,
    (pos) =>
      `linear-gradient(
        90deg,
        transparent,
        transparent ${pos - 5}%,
        rgba(255, 255, 255, 0.2) ${pos}%,
        transparent ${pos + 5}%,
        transparent
      )`,
  )

  // Global mouse tracking
  useEffect(() => {
    if (shouldReduceMotion || !trackGlobally) return

    const handleGlobalMouseMove = (e: MouseEvent) => {
      const rect = cardRef.current?.getBoundingClientRect()
      if (!rect) return

      const centerX = rect.left + rect.width / 2
      const centerY = rect.top + rect.height / 2

      // Calculate relative position with clamping for smoother effect
      const relX = Math.max(-0.5, Math.min(0.5, (e.clientX - centerX) / rect.width))
      const relY = Math.max(-0.5, Math.min(0.5, (e.clientY - centerY) / rect.height))

      x.set(relX)
      y.set(relY)
    }

    window.addEventListener('mousemove', handleGlobalMouseMove)
    return () => window.removeEventListener('mousemove', handleGlobalMouseMove)
  }, [shouldReduceMotion, trackGlobally, x, y])

  // Local mouse tracking (fallback when trackGlobally is false)
  const handleMouseMove = (e: React.MouseEvent) => {
    if (shouldReduceMotion || trackGlobally) return

    const rect = cardRef.current?.getBoundingClientRect()
    if (!rect) return

    const centerX = rect.left + rect.width / 2
    const centerY = rect.top + rect.height / 2

    x.set((e.clientX - centerX) / rect.width)
    y.set((e.clientY - centerY) / rect.height)
  }

  const handleMouseLeave = () => {
    if (trackGlobally) return // Don't reset when tracking globally
    x.set(0)
    y.set(0)
  }

  // Use conditional rendering instead of early return to keep hook count stable
  if (shouldReduceMotion) {
    return <div className={className}>{children}</div>
  }

  return (
    <motion.div
      ref={cardRef}
      onMouseMove={handleMouseMove}
      onMouseLeave={handleMouseLeave}
      style={{
        rotateX,
        rotateY,
        transformStyle: 'preserve-3d',
        transformPerspective: 1000,
      }}
      className={`relative ${className}`}
    >
      {/* Dynamic glow following cursor */}
      <motion.div
        className="pointer-events-none absolute -inset-px"
        style={{ background: glowBackground }}
      />

      {/* Holographic overlay effect */}
      {holographic && (
        <>
          {/* Prismatic color shift layer */}
          <motion.div
            className="pointer-events-none absolute -inset-px overflow-hidden"
            style={{
              background: holoBackground,
              filter: holoFilter,
              mixBlendMode: 'overlay',
            }}
          />

          {/* Rainbow edge reflection */}
          <motion.div
            className="pointer-events-none absolute inset-0"
            style={{
              background: rainbowBackground,
              opacity: 0.3,
              mixBlendMode: 'screen',
            }}
          />

          {/* Shimmer line effect */}
          <motion.div
            className="pointer-events-none absolute inset-0 overflow-hidden"
            style={{ background: shimmerBackground }}
          />
        </>
      )}

      {children}
    </motion.div>
  )
}
