import { motion } from 'framer-motion'
import { StatusBadge } from './StatusBadge'
import { TiltCard } from './ui/TiltCard'
import type { MetricsResponse } from '../api/types'
import { useReducedMotion } from '../hooks/useReducedMotion'
import { staggerContainer, listItem } from '../utils/animations'

interface SystemStatusCardProps {
  status: string
  uptime: string
  metrics?: MetricsResponse
}

export function SystemStatusCard({ status, uptime, metrics }: SystemStatusCardProps) {
  const prefersReducedMotion = useReducedMotion()

  const metricsData = [
    { label: 'UPTIME', value: uptime, showCursor: true },
    {
      label: 'HEALTH',
      value:
        status === 'ok'
          ? 'Operational'
          : status === 'degraded'
            ? 'Degraded'
            : status === 'error'
              ? 'Error'
              : 'Unknown',
      valueColor: getStatusColor(status),
    },
    ...(metrics
      ? [
          { label: 'MEMORY', value: `${metrics.memory.resident_mb.toFixed(1)} MB` },
          { label: 'GOROUTINES', value: metrics.runtime.goroutines.toString() },
          {
            label: 'DB POOL',
            value: `${metrics.database.active_conns}/${metrics.database.max_conns}`,
            progress: metrics.database.active_conns / metrics.database.max_conns,
          },
          { label: 'EVENTS', value: metrics.bot.events_handled.toLocaleString() },
        ]
      : []),
  ]

  return (
    <div className="space-y-6">
      {/* Status Header */}
      <motion.div
        className="flex items-center justify-between border-b border-purple-500/20 pb-4"
        initial={prefersReducedMotion ? {} : { opacity: 0, y: -10 }}
        animate={{ opacity: 1, y: 0 }}
        transition={{ duration: 0.4 }}
      >
        <h2 className="text-xl font-semibold tracking-wide text-purple-300">System Status</h2>
        <StatusBadge status={status} />
      </motion.div>

      {/* Stats Grid */}
      <motion.div
        className="grid grid-cols-2 gap-4 lg:grid-cols-3"
        variants={prefersReducedMotion ? {} : staggerContainer}
        initial="hidden"
        animate="visible"
      >
        {metricsData.map((metric, index) => (
          <motion.div key={metric.label} variants={prefersReducedMotion ? {} : listItem}>
            <TiltCard trackGlobally={false}>
              <MetricTile
                label={metric.label}
                value={metric.value}
                valueColor={metric.valueColor}
                progress={metric.progress}
                showCursor={metric.showCursor}
                index={index}
              />
            </TiltCard>
          </motion.div>
        ))}
      </motion.div>
    </div>
  )
}

interface MetricTileProps {
  label: string
  value: string
  valueColor?: string
  progress?: number
  showCursor?: boolean
  index?: number
}

function MetricTile({
  label,
  value,
  valueColor = 'text-white',
  progress,
  showCursor,
}: MetricTileProps) {
  const prefersReducedMotion = useReducedMotion()

  return (
    <div className="glow-border-hover border border-purple-500/20 bg-black/30 p-5 transition-all duration-300">
      <div className="mb-1 text-xs font-medium tracking-widest text-purple-400/60">{label}</div>
      <div className="flex items-baseline gap-2">
        <span className={`font-mono text-2xl font-bold tracking-tight ${valueColor}`}>{value}</span>
        {showCursor && (
          <motion.span
            className="h-5 w-0.5 bg-purple-500/70"
            animate={prefersReducedMotion ? {} : { opacity: [1, 0, 1] }}
            transition={{ duration: 1, repeat: Infinity }}
          />
        )}
      </div>
      {progress !== undefined && (
        <div className="relative mt-2 h-1 w-full overflow-hidden rounded-full bg-slate-700/50">
          <motion.div
            className="h-full bg-gradient-to-r from-purple-500/70 to-cyan-500/70"
            initial={prefersReducedMotion ? {} : { width: 0 }}
            animate={{ width: `${Math.min(progress * 100, 100)}%` }}
            transition={{ duration: 0.8, delay: 0.3 }}
          />
          {/* Shimmer effect on progress bar */}
          {!prefersReducedMotion && (
            <motion.div
              className="absolute inset-0 bg-gradient-to-r from-transparent via-white/20 to-transparent"
              animate={{ x: ['-100%', '200%'] }}
              transition={{ duration: 2, repeat: Infinity, repeatDelay: 1 }}
            />
          )}
        </div>
      )}
    </div>
  )
}

const getStatusColor = (status: string) => {
  switch (status) {
    case 'ok':
      return 'text-emerald-400'
    case 'degraded':
      return 'text-amber-400'
    case 'error':
      return 'text-rose-400'
    default:
      return 'text-slate-400'
  }
}
