import { motion } from 'framer-motion'
import { useReducedEffects } from '../../context/ReducedEffectsContext'

export function ReducedEffectsToggle() {
  const { effectsLevel, cycleEffectsLevel } = useReducedEffects()

  const config = {
    full: { color: 'bg-green-500', label: 'Full FX' },
    reduced: { color: 'bg-yellow-500', label: 'Reduced FX' },
    minimal: { color: 'bg-red-500', label: 'Minimal FX' },
  }

  const { color, label } = config[effectsLevel]

  return (
    <motion.button
      onClick={cycleEffectsLevel}
      className="fixed top-4 left-4 flex items-center gap-2 rounded border border-purple-500/30 bg-slate-900/80 px-3 py-1.5 font-mono text-xs text-slate-400 backdrop-blur-sm transition-colors hover:border-purple-500/50 hover:text-slate-300"
      style={{ zIndex: 50 }}
      initial={{ opacity: 0, x: -10 }}
      animate={{ opacity: 1, x: 0 }}
      transition={{ delay: 0.5 }}
      title="Click to cycle effects level"
    >
      <span className={`h-2 w-2 rounded-full transition-colors ${color}`} />
      <span>{label}</span>
    </motion.button>
  )
}
