import { StatusBadge } from './StatusBadge'

interface SystemStatusCardProps {
  status: string
  uptime: string
}

export function SystemStatusCard({ status, uptime }: SystemStatusCardProps) {
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
      <div className="grid grid-cols-1 gap-4 sm:grid-cols-2">
        {/* Uptime */}
        <div className="border border-purple-500/20 bg-black/30 p-5">
          <div className="mb-1 text-xs font-medium tracking-widest text-purple-400/60">
            UPTIME
          </div>
          <div className="flex items-baseline gap-2">
            <span className="font-mono text-2xl font-bold tracking-tight text-white">
              {uptime}
            </span>
            <span className="h-5 w-0.5 animate-pulse bg-purple-500/70" />
          </div>
        </div>

        {/* Status Indicator */}
        <div className="border border-purple-500/20 bg-black/30 p-5">
          <div className="mb-1 text-xs font-medium tracking-widest text-purple-400/60">
            HEALTH
          </div>
          <div className={`text-2xl font-bold tracking-tight ${getStatusColor(status)}`}>
            {status === 'ok' ? 'Operational' : status === 'degraded' ? 'Degraded' : status === 'error' ? 'Error' : 'Unknown'}
          </div>
        </div>
      </div>
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

