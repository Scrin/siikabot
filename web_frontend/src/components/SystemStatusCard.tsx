import { StatusBadge } from './StatusBadge'
import type { MetricsResponse } from '../api/types'

interface SystemStatusCardProps {
  status: string
  uptime: string
  metrics?: MetricsResponse
}

export function SystemStatusCard({ status, uptime, metrics }: SystemStatusCardProps) {
  return (
    <div className="space-y-6">
      {/* Status Header */}
      <div className="flex items-center justify-between border-b border-purple-500/20 pb-4">
        <h2 className="text-xl font-semibold tracking-wide text-purple-300">
          System Status
        </h2>
        <StatusBadge status={status} />
      </div>

      {/* Stats Grid */}
      <div className="grid grid-cols-2 gap-4 lg:grid-cols-3">
        {/* Uptime */}
        <MetricTile label="UPTIME" value={uptime} showCursor />

        {/* Status Indicator */}
        <MetricTile
          label="HEALTH"
          value={status === 'ok' ? 'Operational' : status === 'degraded' ? 'Degraded' : status === 'error' ? 'Error' : 'Unknown'}
          valueColor={getStatusColor(status)}
        />

        {/* Memory */}
        {metrics && (
          <MetricTile
            label="MEMORY"
            value={`${metrics.memory.resident_mb.toFixed(1)} MB`}
          />
        )}

        {/* Goroutines */}
        {metrics && (
          <MetricTile
            label="GOROUTINES"
            value={metrics.runtime.goroutines.toString()}
          />
        )}

        {/* DB Pool */}
        {metrics && (
          <MetricTile
            label="DB POOL"
            value={`${metrics.database.active_conns}/${metrics.database.max_conns}`}
            progress={metrics.database.active_conns / metrics.database.max_conns}
          />
        )}

        {/* Events */}
        {metrics && (
          <MetricTile
            label="EVENTS"
            value={metrics.bot.events_handled.toLocaleString()}
          />
        )}
      </div>
    </div>
  )
}

interface MetricTileProps {
  label: string
  value: string
  valueColor?: string
  progress?: number
  showCursor?: boolean
}

function MetricTile({ label, value, valueColor = 'text-white', progress, showCursor }: MetricTileProps) {
  return (
    <div className="border border-purple-500/20 bg-black/30 p-5">
      <div className="mb-1 text-xs font-medium tracking-widest text-purple-400/60">
        {label}
      </div>
      <div className="flex items-baseline gap-2">
        <span className={`font-mono text-2xl font-bold tracking-tight ${valueColor}`}>
          {value}
        </span>
        {showCursor && <span className="h-5 w-0.5 animate-pulse bg-purple-500/70" />}
      </div>
      {progress !== undefined && (
        <div className="mt-2 h-1 w-full overflow-hidden rounded-full bg-slate-700/50">
          <div
            className="h-full bg-purple-500/70 transition-all duration-300"
            style={{ width: `${Math.min(progress * 100, 100)}%` }}
          />
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
