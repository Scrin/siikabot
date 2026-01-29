interface StatusBadgeProps {
  status: string
}

export function StatusBadge({ status }: StatusBadgeProps) {
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

  return (
    <div
      className={`flex items-center gap-2 border px-3 py-1.5 ${getStatusColor(status)}`}
    >
      <div className={`h-2 w-2 ${getDotColor(status)} animate-pulse`} />
      <span className="text-xs font-bold tracking-wider">
        {status.toUpperCase()}
      </span>
    </div>
  )
}
