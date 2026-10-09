export interface RankedBar {
  key: string
  /** The row's place. Rows with the same value share a place. */
  rank: number
  /** A name, or an ID when there is no name */
  label: string
  /** The ID, when the label is a name */
  id?: string
  /** A short note after the name, such as "(you)" */
  note?: string
  /** What the bar's length shows */
  value: number
  /** The value as written at the end of the row */
  valueLabel: string
  /** A secondary figure after the value */
  detail?: string
  /** Dims the row, for someone no longer in the room */
  muted?: boolean
}

interface RankedBarsProps {
  bars: RankedBar[]
  /** What the list ranks, which names it for screen readers */
  label: string
}

/**
 * A ranked list in which each row's bar shows its value against the largest one. Every value is
 * written out beside its bar, so the bars only add the comparison.
 */
export function RankedBars({ bars, label }: RankedBarsProps) {
  const max = Math.max(0, ...bars.map((bar) => bar.value))

  return (
    <ol className="space-y-2.5" aria-label={label}>
      {bars.map((bar) => (
        <li key={bar.key} className={bar.muted ? 'opacity-60' : undefined}>
          <div className="flex items-baseline justify-between gap-3 text-xs">
            <div className="min-w-0">
              <span className="mr-2 font-mono text-slate-400 tabular-nums">{bar.rank}.</span>
              <span className="text-slate-200">{bar.label}</span>
              {bar.id && <span className="ml-1 font-mono break-all text-slate-500">({bar.id})</span>}
              {bar.note && <span className="ml-2 font-mono text-slate-300">{bar.note}</span>}
            </div>
            <div className="shrink-0 font-mono text-slate-300 tabular-nums">
              {bar.valueLabel}
              {bar.detail && <span className="ml-2 text-slate-400">{bar.detail}</span>}
            </div>
          </div>
          <div className="mt-1 h-1.5" aria-hidden="true">
            <div
              className="h-full rounded-r-[4px] bg-purple-500"
              style={{
                width: max > 0 && bar.value > 0 ? `max(2px, ${(bar.value / max) * 100}%)` : 0,
              }}
            />
          </div>
        </li>
      ))}
    </ol>
  )
}
