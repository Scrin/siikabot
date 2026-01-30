import { motion, useMotionValue, useSpring, useTransform } from 'framer-motion'
import { useRef, useEffect, type ReactNode } from 'react'
import { useReducedMotion } from '../../hooks/useReducedMotion'

interface TiltCardProps {
  children: ReactNode
  className?: string
  glowColor?: string
  /** Track mouse globally (whole window) instead of just when hovering over the card */
  trackGlobally?: boolean
}

export function TiltCard({
  children,
  className = '',
  glowColor = 'rgba(168, 85, 247, 0.3)',
  trackGlobally = true,
}: TiltCardProps) {
  const cardRef = useRef<HTMLDivElement>(null)
  const prefersReducedMotion = useReducedMotion()

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

  // Global mouse tracking
  useEffect(() => {
    if (prefersReducedMotion || !trackGlobally) return

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
  }, [prefersReducedMotion, trackGlobally, x, y])

  // Local mouse tracking (fallback when trackGlobally is false)
  const handleMouseMove = (e: React.MouseEvent) => {
    if (prefersReducedMotion || trackGlobally) return

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

  if (prefersReducedMotion) {
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
        style={{
          background: useTransform(
            [glowX, glowY],
            ([gx, gy]) =>
              `radial-gradient(600px circle at ${gx} ${gy}, ${glowColor}, transparent 40%)`,
          ),
        }}
      />

      {children}
    </motion.div>
  )
}
