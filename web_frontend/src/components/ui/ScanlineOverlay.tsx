import { motion } from 'framer-motion'
import { useReducedMotion } from '../../hooks/useReducedMotion'

interface ScanlineOverlayProps {
  intensity?: 'light' | 'medium' | 'heavy'
}

export function ScanlineOverlay({ intensity = 'light' }: ScanlineOverlayProps) {
  const prefersReducedMotion = useReducedMotion()

  const opacities = {
    light: 0.02,
    medium: 0.04,
    heavy: 0.08,
  }

  if (prefersReducedMotion) {
    return null
  }

  return (
    <>
      {/* Static horizontal scanlines */}
      <div
        className="pointer-events-none fixed inset-0"
        style={{
          zIndex: 100,
          background: `repeating-linear-gradient(
            0deg,
            rgba(0, 0, 0, 0) 0px,
            rgba(0, 0, 0, 0) 2px,
            rgba(0, 0, 0, ${opacities[intensity]}) 2px,
            rgba(0, 0, 0, ${opacities[intensity]}) 4px
          )`,
        }}
      />

      {/* Moving scan line */}
      <motion.div
        className="pointer-events-none fixed left-0 right-0 h-[2px] bg-gradient-to-r from-transparent via-purple-500/20 to-transparent"
        style={{ zIndex: 101 }}
        initial={{ top: '-2px' }}
        animate={{ top: ['0%', '100%'] }}
        transition={{
          duration: 8,
          repeat: Infinity,
          ease: 'linear',
        }}
      />

      {/* Subtle CRT flicker overlay */}
      <motion.div
        className="pointer-events-none fixed inset-0 bg-black/[0.01]"
        style={{ zIndex: 99 }}
        animate={{
          opacity: [0.01, 0.02, 0.01, 0.015, 0.01],
        }}
        transition={{
          duration: 0.1,
          repeat: Infinity,
          repeatType: 'mirror',
        }}
      />
    </>
  )
}
