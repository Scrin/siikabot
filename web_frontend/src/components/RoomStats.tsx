import { motion } from 'framer-motion'
import type { PersonStats, RoomStatsResponse } from '../api/types'
import { formatDateTime, weekdayOf } from '../utils/dateTime'
import { formatCount, formatExact, formatShare } from '../utils/format'
import { ActivityChart } from './ActivityChart'
import { PeriodSelector } from './PeriodSelector'
import { RankedBars, type RankedBar } from './RankedBars'
import { Stat } from './Stat'

const PERIODS = [7, 30, 90] as const

interface RoomStatsProps {
  stats: RoomStatsResponse | undefined
  isLoading: boolean
  /** The stats shown are still the previous period's, while the new ones load */
  isRefreshing: boolean
  error: Error | null
  days: number
  onDaysChange: (days: number) => void
}

/**
 * A room's message stats: its activity over a period, then the counts of its people. Those have no
 * breakdown by date, so they cover everything since counting began, whatever the period.
 */
export function RoomStats({
  stats,
  isLoading,
  isRefreshing,
  error,
  days,
  onDaysChange,
}: RoomStatsProps) {
  if (isLoading) {
    return (
      <div className="flex items-center gap-2 text-slate-400">
        <motion.div
          className="h-3 w-3 rounded-full border-2 border-purple-500 border-t-transparent"
          animate={{ rotate: 360 }}
          transition={{ duration: 1, repeat: Infinity, ease: 'linear' }}
        />
        <span className="font-mono text-xs">Loading stats...</span>
      </div>
    )
  }

  if (error) {
    return <div className="font-mono text-xs text-rose-400">Failed to load stats</div>
  }

  if (!stats) {
    return null
  }

  const { activity, people } = stats
  if (activity.first_day === null && people.counted_since === null) {
    return (
      <div className="font-mono text-xs text-slate-500">No messages counted in this room yet</div>
    )
  }

  const perDay = activity.daily.length > 0 ? activity.messages / activity.daily.length : 0
  const me = people.me
  const meListed = me !== null && people.top.some((person) => person.user_id === me.user_id)

  return (
    <div className="space-y-5">
      <section aria-label="Activity" className="space-y-3">
        <div className="flex flex-wrap items-center justify-between gap-2">
          <h4 className="font-mono text-xs tracking-wider text-slate-400 uppercase">Activity</h4>
          <PeriodSelector periods={PERIODS} value={days} onChange={onDaysChange} />
        </div>
        <div
          className={`space-y-3 transition-opacity duration-200 ${isRefreshing ? 'opacity-50' : ''}`}
        >
          <div className="grid grid-cols-2 gap-3 sm:grid-cols-4">
            <Stat label="Messages" value={formatCount(activity.messages)} />
            <Stat label="A day" value={perDay.toFixed(1)} />
            <Stat
              label="Busiest day"
              value={activity.busiest ? formatCount(activity.busiest.messages) : '–'}
              detail={
                activity.busiest
                  ? `${weekdayOf(activity.busiest.date)} ${activity.busiest.date}`
                  : undefined
              }
            />
            <Stat label="Words" value={formatCount(activity.words)} />
          </div>
          {activity.daily.length > 0 && (
            <ActivityChart
              daily={activity.daily}
              today={activity.to}
              title="Text messages per day"
            />
          )}
          {activity.first_day && activity.daily.length < activity.days && (
            <p className="font-mono text-xs text-slate-400">
              Counting began on {activity.first_day}, so the period starts then.
            </p>
          )}
        </div>
      </section>

      {people.counted_since && (
        <section aria-label="Most messages" className="space-y-3">
          <div className="space-y-1">
            <h4 className="font-mono text-xs tracking-wider text-slate-400 uppercase">
              Most messages · all time
            </h4>
            <p className="font-mono text-xs text-slate-400">
              {formatExact(people.people_count)}{' '}
              {people.people_count === 1 ? 'person has' : 'people have'} sent{' '}
              {formatExact(people.messages)} text messages since{' '}
              {formatDateTime(new Date(people.counted_since)).slice(0, 10)}
            </p>
          </div>
          <RankedBars
            bars={people.top.map((person) => personBar(person, me))}
            label="People with the most messages in this room"
          />
          {me && !meListed && (
            <p className="font-mono text-xs text-slate-300">
              You: #{me.rank} of {formatExact(people.people_count)} ·{' '}
              {formatExact(me.messages)} messages · {formatShare(me.share)}
            </p>
          )}
        </section>
      )}
    </div>
  )
}

function personBar(person: PersonStats, me: PersonStats | null): RankedBar {
  let note: string | undefined
  if (person.user_id === me?.user_id) {
    note = '(you)'
  } else if (!person.in_room) {
    note = '(not in room)'
  }

  return {
    key: person.user_id,
    rank: person.rank,
    label: person.display_name || person.user_id,
    id: person.display_name ? person.user_id : undefined,
    note,
    value: person.messages,
    valueLabel: `${formatExact(person.messages)} · ${formatShare(person.share)}`,
    detail: `${formatCount(person.words)} words`,
    muted: !person.in_room,
  }
}
