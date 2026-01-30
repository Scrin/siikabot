import { motion } from 'framer-motion'
import { TypewriterText } from './ui/TypewriterText'
import { useReducedMotion } from '../hooks/useReducedMotion'

export function LoadingSpinner() {
  const prefersReducedMotion = useReducedMotion()

  if (prefersReducedMotion) {
    return (
      <div className="flex flex-col items-center justify-center py-12">
        <div className="h-8 w-8 animate-spin rounded-full border-2 border-purple-500 border-t-transparent" />
        <p className="mt-6 text-sm tracking-wider text-blue-300/60">Initializing systems...</p>
      </div>
    )
  }

  return (
    <div className="flex flex-col items-center justify-center py-12">
      {/* Futuristic orbital spinner */}
      <div className="relative h-20 w-20">
        {/* Outer ring */}
        <motion.div
          className="absolute inset-0 rounded-full border-2 border-purple-500/30"
          animate={{ rotate: 360 }}
          transition={{ duration: 4, repeat: Infinity, ease: 'linear' }}
        />

        {/* Middle ring - opposite direction */}
        <motion.div
          className="absolute inset-2 rounded-full border-2 border-cyan-500/40"
          animate={{ rotate: -360 }}
          transition={{ duration: 3, repeat: Infinity, ease: 'linear' }}
        />

        {/* Inner ring */}
        <motion.div
          className="absolute inset-4 rounded-full border-2 border-blue-500/30"
          animate={{ rotate: 360 }}
          transition={{ duration: 2, repeat: Infinity, ease: 'linear' }}
        />

        {/* Orbiting dots */}
        {[0, 1, 2, 3].map((i) => (
          <motion.div
            key={i}
            className="absolute left-1/2 top-1/2 h-2 w-2 -translate-x-1/2 -translate-y-1/2 rounded-full bg-purple-500"
            animate={{
              x: [0, 30, 0, -30, 0],
              y: [-30, 0, 30, 0, -30],
              scale: [1, 0.7, 1, 0.7, 1],
              opacity: [1, 0.5, 1, 0.5, 1],
            }}
            transition={{
              duration: 2.5,
              repeat: Infinity,
              delay: i * 0.625,
              ease: 'easeInOut',
            }}
            style={{
              boxShadow: '0 0 10px #a855f7, 0 0 20px #a855f7',
            }}
          />
        ))}

        {/* Center glow */}
        <motion.div
          className="absolute inset-6 rounded-full bg-purple-500/20"
          animate={{
            scale: [1, 1.3, 1],
            opacity: [0.3, 0.6, 0.3],
          }}
          transition={{ duration: 1.5, repeat: Infinity }}
          style={{
            boxShadow: '0 0 20px rgba(168, 85, 247, 0.5)',
          }}
        />

        {/* Center dot */}
        <motion.div
          className="absolute left-1/2 top-1/2 h-2 w-2 -translate-x-1/2 -translate-y-1/2 rounded-full bg-white"
          animate={{
            opacity: [0.8, 1, 0.8],
            scale: [1, 1.2, 1],
          }}
          transition={{ duration: 1, repeat: Infinity }}
        />
      </div>

      {/* Typing animation for text */}
      <motion.p
        className="mt-8 font-mono text-sm tracking-wider text-blue-300/60"
        initial={{ opacity: 0 }}
        animate={{ opacity: 1 }}
        transition={{ delay: 0.3 }}
      >
        <TypewriterText text="Initializing systems..." delay={0.5} speed={0.06} />
      </motion.p>
    </div>
  )
}
