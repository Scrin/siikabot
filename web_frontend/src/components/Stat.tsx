interface StatProps {
  label: string
  value: string
  tone?: string
  /** A line under the value that says what it is of, such as the date of the busiest day */
  detail?: string
}

/** A single figure in a card's row of headline numbers */
export function Stat({ label, value, tone = 'text-slate-200', detail }: StatProps) {
  return (
    <div>
      <div className="font-mono text-xs tracking-wider text-slate-500 uppercase">{label}</div>
      <div className={`font-mono text-lg ${tone}`}>{value}</div>
      {detail && <div className="font-mono text-xs text-slate-400">{detail}</div>}
    </div>
  )
}
