import { motion } from 'framer-motion'
import { useReminders } from '../api/queries'
import type { ReminderResponse } from '../api/types'
import { formatDateTime } from '../utils/dateTime'
import { AnimatedList } from './ui/AnimatedList'
import { useReducedMotion } from '../hooks/useReducedMotion'

export function RemindersCard() {
  const { data, isLoading, error } = useReminders()
  const prefersReducedMotion = useReducedMotion()

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
        <span className="font-mono text-sm">Loading reminders...</span>
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
        <span className="text-sm text-rose-400">Failed to load reminders</span>
      </motion.div>
    )
  }

  const reminders = data?.reminders ?? []

  if (reminders.length === 0) {
    return (
      <motion.div
        className="border border-slate-700/50 bg-black/20 p-4"
        initial={prefersReducedMotion ? {} : { opacity: 0 }}
        animate={{ opacity: 1 }}
      >
        <span className="font-mono text-sm text-slate-500">No active reminders</span>
      </motion.div>
    )
  }

  return (
    <AnimatedList
      items={reminders}
      keyExtractor={(reminder) => reminder.id}
      className="space-y-3"
      renderItem={(reminder) => <ReminderItem reminder={reminder} />}
    />
  )
}

interface ReminderItemProps {
  reminder: ReminderResponse
}

function ReminderItem({ reminder }: ReminderItemProps) {
  const remindTime = new Date(reminder.remind_time)
  const timeUntil = formatTimeUntil(remindTime)
  const formattedDate = formatDateTime(remindTime)
  const prefersReducedMotion = useReducedMotion()

  return (
    <motion.div
      className="glow-border-hover border border-purple-500/20 bg-black/30 p-4 transition-all duration-300"
      whileHover={prefersReducedMotion ? {} : { scale: 1.01, x: 4 }}
      transition={{ type: 'spring', stiffness: 400, damping: 17 }}
    >
      <div className="space-y-2">
        <p className="text-sm text-slate-200">{reminder.message}</p>

        <div className="flex flex-wrap items-center gap-3 text-xs">
          <span className="font-mono text-purple-400">{formattedDate}</span>
          <span className="text-slate-500">|</span>
          <motion.span
            className="font-mono text-blue-400"
            animate={
              prefersReducedMotion
                ? {}
                : {
                    textShadow: [
                      '0 0 0 transparent',
                      '0 0 8px rgba(59, 130, 246, 0.5)',
                      '0 0 0 transparent',
                    ],
                  }
            }
            transition={{ duration: 2, repeat: Infinity }}
          >
            {timeUntil}
          </motion.span>
        </div>

        <div className="text-xs text-slate-500">
          Room:{' '}
          {reminder.room_name ? (
            <span className="text-slate-400">
              {reminder.room_name}{' '}
              <span className="font-mono text-slate-500">({reminder.room_id})</span>
            </span>
          ) : (
            <span className="font-mono text-slate-400">{reminder.room_id}</span>
          )}
        </div>
      </div>
    </motion.div>
  )
}

function formatTimeUntil(date: Date): string {
  const now = new Date()
  const diffMs = date.getTime() - now.getTime()

  if (diffMs <= 0) {
    return 'any moment now'
  }

  const diffSeconds = Math.floor(diffMs / 1000)
  const diffMinutes = Math.floor(diffSeconds / 60)
  const diffHours = Math.floor(diffMinutes / 60)
  const diffDays = Math.floor(diffHours / 24)

  if (diffDays > 0) {
    const hours = diffHours % 24
    return `in ${diffDays}d ${hours}h`
  }
  if (diffHours > 0) {
    const minutes = diffMinutes % 60
    return `in ${diffHours}h ${minutes}m`
  }
  if (diffMinutes > 0) {
    return `in ${diffMinutes}m`
  }
  return `in ${diffSeconds}s`
}
