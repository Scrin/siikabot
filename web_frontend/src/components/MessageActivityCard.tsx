import { useState } from 'react'
import { motion } from 'framer-motion'
import { useMessageActivity } from '../api/queries'
import type { RoomActivity } from '../api/types'
import { useReducedMotion } from '../hooks/useReducedMotion'
import { formatCount, formatExact, formatShare } from '../utils/format'
import { ActivityChart } from './ActivityChart'
import { PeriodSelector } from './PeriodSelector'
import { RankedBars, type RankedBar } from './RankedBars'
import { Stat } from './Stat'

const PERIODS = [7, 30, 90] as const

/** The text messages counted across every room, with the chat turns in each (admin only) */
export function MessageActivityCard() {
  const [days, setDays] = useState<number>(30)
  const { data, isLoading, error, isPlaceholderData } = useMessageActivity(days)
  const prefersReducedMotion = useReducedMotion()

  const periodSelector = <PeriodSelector periods={PERIODS} value={days} onChange={setDays} />

  if (isLoading) {
    return (
      <motion.div
        className="flex items-center gap-2 text-slate-400"
        initial={prefersReducedMotion ? {} : { opacity: 0 }}
        animate={{ opacity: 1 }}
      >
        <motion.div
          className="h-4 w-4 rounded-full border-2 border-purple-500 border-t-transparent"
          animate={{ rotate: 360 }}
          transition={{ duration: 1, repeat: Infinity, ease: 'linear' }}
        />
        <span className="font-mono text-sm">Loading message activity...</span>
      </motion.div>
    )
  }

  if (error) {
    return (
      <motion.div
        className="border border-rose-500/50 bg-rose-950/30 p-4"
        initial={prefersReducedMotion ? {} : { opacity: 0, x: -10 }}
        animate={{ opacity: 1, x: 0 }}
      >
        <span className="text-sm text-rose-400">Failed to load message activity</span>
      </motion.div>
    )
  }

  if (!data) {
    return null
  }

  const { activity } = data
  if (activity.first_day === null) {
    return (
      <div className="space-y-3">
        {periodSelector}
        <div className="border border-slate-700/50 bg-black/20 p-4">
          <span className="font-mono text-sm text-slate-500">No messages counted yet</span>
        </div>
      </div>
    )
  }

  const perDay = activity.daily.length > 0 ? activity.messages / activity.daily.length : 0

  return (
    <div className="space-y-3">
      {periodSelector}

      <div
        className={`space-y-4 transition-opacity duration-200 ${isPlaceholderData ? 'opacity-50' : ''}`}
      >
        <div className="border border-purple-500/30 bg-purple-950/20 p-4">
          <div className="grid grid-cols-2 gap-3 sm:grid-cols-4">
            <Stat label="Messages" value={formatCount(activity.messages)} />
            <Stat label="A day" value={perDay.toFixed(1)} />
            <Stat label="Active rooms" value={formatCount(data.rooms.length)} />
            <Stat label="Chat turns" value={formatCount(data.chat_turns)} />
          </div>
        </div>

        {activity.daily.length > 0 && (
          <ActivityChart
            daily={activity.daily}
            today={activity.to}
            title="Text messages per day, all rooms"
          />
        )}
        {activity.daily.length < activity.days && (
          <p className="font-mono text-xs text-slate-400">
            Counting began on {activity.first_day}, so the period starts then.
          </p>
        )}

        {data.rooms.length > 0 && (
          <section aria-label="Busiest rooms" className="space-y-3">
            <h4 className="font-mono text-xs tracking-wider text-slate-400 uppercase">
              Busiest rooms
            </h4>
            <RankedBars bars={roomBars(data.rooms)} label="Rooms with the most messages" />
          </section>
        )}
      </div>
    </div>
  )
}

/** The rooms as ranked bars, rooms with the same count sharing a place */
function roomBars(rooms: RoomActivity[]): RankedBar[] {
  let rank = 0
  return rooms.map((room, index) => {
    if (index === 0 || room.messages !== rooms[index - 1].messages) {
      rank = index + 1
    }
    return {
      key: room.room_id,
      rank,
      label: room.room_name || room.room_id,
      id: room.room_name ? room.room_id : undefined,
      value: room.messages,
      valueLabel: `${formatExact(room.messages)} · ${formatShare(room.share)}`,
      detail: `${formatExact(room.chat_turns)} chat ${room.chat_turns === 1 ? 'turn' : 'turns'}`,
    }
  })
}
