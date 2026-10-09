import { useState, type KeyboardEvent, type PointerEvent } from 'react'
import type { DayCounts } from '../api/types'
import { niceCeiling } from '../utils/chart'
import { weekdayOf } from '../utils/dateTime'
import { formatExact } from '../utils/format'

interface ActivityChartProps {
  /** Every day of the period, oldest first */
  daily: DayCounts[]
  /** Today's date, whose counts are still growing */
  today: string
  /** What the chart shows, which also names both views for screen readers */
  title: string
}

type View = 'chart' | 'table'

const VIEWS: readonly View[] = ['chart', 'table']

/**
 * Text messages per day as columns, each day's counts shown on hover, on tap or with the arrow keys,
 * and the same days as a table for reading them all without hovering
 */
export function ActivityChart({ daily, today, title }: ActivityChartProps) {
  const [view, setView] = useState<View>('chart')

  return (
    <figure className="space-y-2">
      <div className="flex items-center justify-between gap-2">
        <figcaption className="font-mono text-xs text-slate-400">{title}</figcaption>
        <div className="flex gap-1">
          {VIEWS.map((option) => (
            <button
              key={option}
              type="button"
              onClick={() => setView(option)}
              aria-pressed={view === option}
              className={`border px-2 py-0.5 font-mono text-xs capitalize transition-colors duration-200 ${
                view === option
                  ? 'border-purple-500/50 text-purple-300'
                  : 'border-slate-700/50 text-slate-400 hover:border-slate-600 hover:text-slate-300'
              }`}
            >
              {option}
            </button>
          ))}
        </div>
      </div>
      {view === 'chart' ? (
        <Columns daily={daily} today={today} title={title} />
      ) : (
        <DayTable daily={daily} today={today} title={title} />
      )}
    </figure>
  )
}

function Columns({ daily, today, title }: ActivityChartProps) {
  const [active, setActive] = useState<number | null>(null)
  const ceiling = niceCeiling(Math.max(0, ...daily.map((day) => day.messages)))
  const last = daily.length - 1

  const onKeyDown = (event: KeyboardEvent) => {
    if (event.key === 'Escape') {
      setActive(null)
      return
    }
    const current = active ?? last
    const moves: Record<string, number> = {
      ArrowLeft: current - 1,
      ArrowRight: current + 1,
      Home: 0,
      End: last,
    }
    if (!(event.key in moves)) {
      return
    }
    event.preventDefault()
    setActive(Math.min(last, Math.max(0, moves[event.key])))
  }

  // A tap leaves its day showing, where a pointer that moves away hides it
  const onPointerLeave = (event: PointerEvent) => {
    if (event.pointerType !== 'touch') {
      setActive(null)
    }
  }

  const activeDay = active === null ? undefined : daily[active]

  return (
    <div className="grid grid-cols-[auto_1fr] gap-x-2">
      {/* The scale: a gridline at a round number, and the baseline */}
      <div
        className="flex h-24 flex-col justify-between font-mono text-[10px] text-slate-400 tabular-nums"
        aria-hidden="true"
      >
        <span className="-translate-y-1/2">{formatExact(ceiling)}</span>
        <span className="translate-y-1/2">0</span>
      </div>
      <div
        tabIndex={0}
        role="group"
        aria-label={`${title}. Use the arrow keys to read each day.`}
        onKeyDown={onKeyDown}
        onFocus={() => setActive((current) => current ?? last)}
        onBlur={() => setActive(null)}
        onPointerLeave={onPointerLeave}
        className="relative flex h-24 items-end gap-[2px] border-t border-b border-t-slate-800 border-b-slate-600 outline-none focus-visible:ring-1 focus-visible:ring-purple-400"
      >
        {daily.map((day, index) => (
          <div
            key={day.date}
            data-testid="activity-day"
            onPointerEnter={() => setActive(index)}
            className="flex h-full min-w-0 flex-1 items-end justify-center"
          >
            <div
              className={`w-full max-w-6 rounded-t-[4px] transition-colors duration-150 ${
                index === active ? 'bg-purple-400' : 'bg-purple-500'
              }`}
              style={{
                height: day.messages > 0 ? `max(2px, ${(day.messages / ceiling) * 100}%)` : 0,
              }}
            />
          </div>
        ))}
        {activeDay && active !== null && (
          <DayTooltip
            day={activeDay}
            position={(active + 0.5) / daily.length}
            isToday={activeDay.date === today}
          />
        )}
      </div>
      <div />
      <div
        className="mt-1 flex justify-between font-mono text-[10px] text-slate-400"
        aria-hidden="true"
      >
        <span>{daily[0].date}</span>
        <span>{daily[last].date === today ? 'today' : daily[last].date}</span>
      </div>
    </div>
  )
}

interface DayTooltipProps {
  day: DayCounts
  /** Where the day's column is across the chart, from 0 to 1 */
  position: number
  isToday: boolean
}

function DayTooltip({ day, position, isToday }: DayTooltipProps) {
  // Near either edge the tooltip opens inwards, so it stays inside the chart
  const shift = position < 0.25 ? '0%' : position > 0.75 ? '-100%' : '-50%'

  return (
    <div
      role="status"
      className="pointer-events-none absolute bottom-full z-10 mb-1 border border-purple-500/40 bg-slate-950/95 px-2 py-1 font-mono text-xs whitespace-nowrap shadow-lg"
      style={{ left: `${position * 100}%`, transform: `translateX(${shift})` }}
    >
      <div className="text-slate-100">
        <span className="font-semibold">{formatExact(day.messages)}</span>{' '}
        {day.messages === 1 ? 'message' : 'messages'}
      </div>
      <div className="text-slate-400">{formatExact(day.words)} words</div>
      <div className="text-slate-400">
        {weekdayOf(day.date)} {day.date}
        {isToday && ' (today, so far)'}
      </div>
    </div>
  )
}

function DayTable({ daily, today, title }: ActivityChartProps) {
  return (
    <div className="max-h-56 overflow-y-auto border border-slate-700/50">
      <table className="w-full font-mono text-xs">
        <caption className="sr-only">{title}</caption>
        <thead className="sticky top-0 bg-slate-900 text-slate-400">
          <tr>
            <th scope="col" className="px-3 py-1.5 text-left font-normal">
              Day
            </th>
            <th scope="col" className="px-3 py-1.5 text-right font-normal">
              Messages
            </th>
            <th scope="col" className="px-3 py-1.5 text-right font-normal">
              Words
            </th>
          </tr>
        </thead>
        <tbody className="text-slate-300 tabular-nums">
          {daily.map((day) => (
            <tr key={day.date} className="border-t border-slate-800">
              <td className="px-3 py-1">
                {weekdayOf(day.date)} {day.date}
                {day.date === today && <span className="text-slate-400"> (today, so far)</span>}
              </td>
              <td className="px-3 py-1 text-right">{formatExact(day.messages)}</td>
              <td className="px-3 py-1 text-right">{formatExact(day.words)}</td>
            </tr>
          ))}
        </tbody>
      </table>
    </div>
  )
}
