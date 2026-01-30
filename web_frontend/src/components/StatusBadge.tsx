import { motion, AnimatePresence } from 'framer-motion'
import { useReducedMotion } from '../hooks/useReducedMotion'

interface StatusBadgeProps {
  status: string
}

export function StatusBadge({ status }: StatusBadgeProps) {
  const prefersReducedMotion = useReducedMotion()

  const getStatusColor = (status: string) => {
    switch (status) {
      case 'ok':
        return 'text-emerald-400 border-emerald-500/50 bg-emerald-500/10'
      case 'degraded':
        return 'text-amber-400 border-amber-500/50 bg-amber-500/10'
      case 'error':
        return 'text-rose-400 border-rose-500/50 bg-rose-500/10'
      default:
        return 'text-slate-400 border-slate-500/50 bg-slate-500/10'
    }
  }

  const getDotColor = (status: string) => {
    switch (status) {
      case 'ok':
        return 'bg-emerald-500'
      case 'degraded':
        return 'bg-amber-500'
      case 'error':
        return 'bg-rose-500'
      default:
        return 'bg-slate-500'
    }
  }

  const getGlowColor = (status: string) => {
    switch (status) {
      case 'ok':
        return 'rgba(52, 211, 153, 0.8)'
      case 'degraded':
        return 'rgba(251, 191, 36, 0.8)'
      case 'error':
        return 'rgba(244, 63, 94, 0.8)'
      default:
        return 'rgba(148, 163, 184, 0.8)'
    }
  }

  if (prefersReducedMotion) {
    return (
      <div className={`flex items-center gap-2 border px-3 py-1.5 ${getStatusColor(status)}`}>
        <div className={`h-2 w-2 rounded-full ${getDotColor(status)}`} />
        <span className="text-xs font-bold tracking-wider">{status.toUpperCase()}</span>
      </div>
    )
  }

  return (
    <motion.div
      className={`flex items-center gap-2 border px-3 py-1.5 ${getStatusColor(status)}`}
      initial={{ scale: 0.9, opacity: 0 }}
      animate={{ scale: 1, opacity: 1 }}
      whileHover={{ scale: 1.05 }}
      transition={{ type: 'spring', stiffness: 400, damping: 17 }}
      layout
    >
      {/* Pulsing dot with glow */}
      <motion.div
        className={`h-2 w-2 rounded-full ${getDotColor(status)}`}
        animate={{
          scale: [1, 1.3, 1],
          boxShadow: [
            `0 0 0 0 ${getGlowColor(status)}`,
            `0 0 10px 3px ${getGlowColor(status)}`,
            `0 0 0 0 ${getGlowColor(status)}`,
          ],
        }}
        transition={{ duration: 2, repeat: Infinity, ease: 'easeInOut' }}
      />

      {/* Status text with animated transition */}
      <AnimatePresence mode="wait">
        <motion.span
          key={status}
          className="text-xs font-bold tracking-wider"
          initial={{ opacity: 0, y: -10, filter: 'blur(4px)' }}
          animate={{ opacity: 1, y: 0, filter: 'blur(0px)' }}
          exit={{ opacity: 0, y: 10, filter: 'blur(4px)' }}
          transition={{ duration: 0.2 }}
        >
          {status.toUpperCase()}
        </motion.span>
      </AnimatePresence>
    </motion.div>
  )
}
