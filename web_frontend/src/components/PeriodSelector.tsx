interface PeriodSelectorProps {
  /** The periods to choose from, in days */
  periods: readonly number[]
  value: number
  onChange: (days: number) => void
}

/** Picks the period, in days, that the figures below it cover */
export function PeriodSelector({ periods, value, onChange }: PeriodSelectorProps) {
  return (
    <div className="flex items-center gap-2">
      <span className="font-mono text-xs text-slate-500">Period:</span>
      {periods.map((days) => (
        <button
          key={days}
          type="button"
          onClick={() => onChange(days)}
          aria-pressed={value === days}
          className={`border px-2 py-1 font-mono text-xs transition-colors duration-200 ${
            value === days
              ? 'border-purple-500/50 bg-purple-500/20 text-purple-300'
              : 'border-slate-700/50 text-slate-400 hover:border-slate-600 hover:text-slate-300'
          }`}
        >
          {days}d
        </button>
      ))}
    </div>
  )
}
