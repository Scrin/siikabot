import { motion } from 'framer-motion'
import { GlitchText } from './ui/GlitchText'
import { TypewriterText } from './ui/TypewriterText'
import { useReducedMotion } from '../hooks/useReducedMotion'

export function PageHeader() {
  const prefersReducedMotion = useReducedMotion()

  return (
    <motion.div
      className="mb-10 text-center"
      initial={prefersReducedMotion ? {} : { opacity: 0, y: -20 }}
      animate={{ opacity: 1, y: 0 }}
      transition={{ duration: 0.6, ease: [0.25, 0.1, 0.25, 1] }}
    >
      <h1 className="text-6xl font-black tracking-tight md:text-7xl">
        <GlitchText text="SIIKABOT" className="gradient-text-animated" continuous />
      </h1>

      {/* Animated divider line */}
      <motion.div
        className="relative mx-auto mt-3 h-px w-48 overflow-hidden"
        initial={prefersReducedMotion ? {} : { scaleX: 0 }}
        animate={{ scaleX: 1 }}
        transition={{ duration: 0.8, delay: 0.3 }}
      >
        <div className="h-full w-full bg-gradient-to-r from-transparent via-purple-500/70 to-transparent" />
        {/* Scanning highlight */}
        {!prefersReducedMotion && (
          <motion.div
            className="absolute inset-0 bg-gradient-to-r from-transparent via-white/50 to-transparent"
            animate={{ x: ['-100%', '200%'] }}
            transition={{ duration: 2, repeat: Infinity, repeatDelay: 3 }}
          />
        )}
      </motion.div>

      <motion.p
        className="mt-4 text-base font-light tracking-wider text-blue-300/60"
        initial={prefersReducedMotion ? {} : { opacity: 0 }}
        animate={{ opacity: 1 }}
        transition={{ duration: 0.5, delay: 0.5 }}
      >
        {prefersReducedMotion ? (
          'System Status Monitor'
        ) : (
          <TypewriterText text="System Status Monitor" delay={0.8} speed={0.04} showCursor={false} />
        )}
      </motion.p>
    </motion.div>
  )
}
