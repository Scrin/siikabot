import { motion } from 'framer-motion'
import { useState, useRef } from 'react'
import { useReducedMotion } from '../../hooks/useReducedMotion'

interface CyberButtonProps {
  children: React.ReactNode
  onClick?: () => void
  variant?: 'primary' | 'secondary' | 'danger'
  disabled?: boolean
  className?: string
  type?: 'button' | 'submit' | 'reset'
}

export function CyberButton({
  children,
  onClick,
  variant = 'primary',
  disabled = false,
  className = '',
  type = 'button',
}: CyberButtonProps) {
  const [isHovered, setIsHovered] = useState(false)
  const buttonRef = useRef<HTMLButtonElement>(null)
  const prefersReducedMotion = useReducedMotion()

  // Ripple effect state
  const [ripples, setRipples] = useState<{ x: number; y: number; id: number }[]>([])

  const handleClick = (e: React.MouseEvent<HTMLButtonElement>) => {
    if (disabled) return

    // Create ripple
    const rect = buttonRef.current?.getBoundingClientRect()
    if (rect && !prefersReducedMotion) {
      const x = e.clientX - rect.left
      const y = e.clientY - rect.top
      setRipples((prev) => [...prev, { x, y, id: Date.now() }])
      setTimeout(() => setRipples((prev) => prev.slice(1)), 600)
    }

    onClick?.()
  }

  const variantStyles = {
    primary: {
      border: 'border-purple-500/50',
      bg: 'bg-purple-900/30',
      text: 'text-purple-300',
      hoverBorder: 'hover:border-purple-400',
      glow: 'rgba(168, 85, 247, 0.4)',
    },
    secondary: {
      border: 'border-cyan-500/50',
      bg: 'bg-cyan-900/30',
      text: 'text-cyan-300',
      hoverBorder: 'hover:border-cyan-400',
      glow: 'rgba(6, 182, 212, 0.4)',
    },
    danger: {
      border: 'border-rose-500/50',
      bg: 'bg-rose-900/30',
      text: 'text-rose-300',
      hoverBorder: 'hover:border-rose-400',
      glow: 'rgba(244, 63, 94, 0.4)',
    },
  }

  const style = variantStyles[variant]

  return (
    <motion.button
      ref={buttonRef}
      type={type}
      onClick={handleClick}
      onHoverStart={() => setIsHovered(true)}
      onHoverEnd={() => setIsHovered(false)}
      disabled={disabled}
      className={`relative overflow-hidden border px-6 py-3 font-mono text-sm tracking-wider transition-colors ${style.border} ${style.bg} ${style.text} ${style.hoverBorder} ${disabled ? 'cursor-not-allowed opacity-50' : 'cursor-pointer'} ${className}`}
      whileHover={prefersReducedMotion ? {} : { scale: 1.02 }}
      whileTap={prefersReducedMotion ? {} : { scale: 0.98 }}
    >
      {/* Glitch effect on hover */}
      <motion.span
        className="relative z-10"
        animate={
          isHovered && !prefersReducedMotion
            ? {
                x: [0, -2, 2, -1, 1, 0],
                textShadow: [
                  '0 0 0 transparent',
                  '-2px 0 #ff0000, 2px 0 #00ffff',
                  '2px 0 #ff0000, -2px 0 #00ffff',
                  '-1px 0 #ff0000, 1px 0 #00ffff',
                  '0 0 0 transparent',
                ],
              }
            : {}
        }
        transition={{ duration: 0.3 }}
      >
        {children}
      </motion.span>

      {/* Scanning line effect */}
      {!prefersReducedMotion && (
        <motion.div
          className="pointer-events-none absolute inset-0 bg-gradient-to-r from-transparent via-white/10 to-transparent"
          initial={{ x: '-100%' }}
          animate={isHovered ? { x: '200%' } : { x: '-100%' }}
          transition={{ duration: 0.5, ease: 'easeInOut' }}
        />
      )}

      {/* Neon glow on hover */}
      <motion.div
        className="pointer-events-none absolute inset-0 -z-10"
        animate={{
          boxShadow: isHovered
            ? `0 0 20px ${style.glow}, 0 0 40px ${style.glow.replace('0.4', '0.2')}, inset 0 0 20px ${style.glow.replace('0.4', '0.1')}`
            : '0 0 0 transparent',
        }}
        transition={{ duration: 0.3 }}
      />

      {/* Ripple effects */}
      {ripples.map((ripple) => (
        <motion.span
          key={ripple.id}
          className="pointer-events-none absolute rounded-full bg-white/30"
          style={{ left: ripple.x, top: ripple.y }}
          initial={{ width: 0, height: 0, x: 0, y: 0, opacity: 1 }}
          animate={{ width: 200, height: 200, x: -100, y: -100, opacity: 0 }}
          transition={{ duration: 0.6, ease: 'easeOut' }}
        />
      ))}
    </motion.button>
  )
}
