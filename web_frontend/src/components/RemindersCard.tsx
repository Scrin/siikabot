import { useReminders } from '../api/queries'
import type { ReminderResponse } from '../api/types'
import { formatDateTime } from '../utils/dateTime'

export function RemindersCard() {
  const { data, isLoading, error } = useReminders()

  if (isLoading) {
    return (
      <div className="flex items-center gap-2 text-slate-400">
        <div className="h-4 w-4 animate-spin rounded-full border-2 border-purple-500 border-t-transparent" />
        <span className="font-mono text-sm">Loading reminders...</span>
      </div>
    )
  }

  if (error) {
    return (
      <div className="border border-rose-500/50 bg-rose-950/30 p-4">
        <span className="text-sm text-rose-400">Failed to load reminders</span>
      </div>
    )
  }

  const reminders = data?.reminders ?? []

  if (reminders.length === 0) {
    return (
      <div className="border border-slate-700/50 bg-black/20 p-4">
        <span className="font-mono text-sm text-slate-500">
          No active reminders
        </span>
      </div>
    )
  }

  return (
    <div className="space-y-3">
      {reminders.map((reminder) => (
        <ReminderItem key={reminder.id} reminder={reminder} />
      ))}
    </div>
  )
}

interface ReminderItemProps {
  reminder: ReminderResponse
}

function ReminderItem({ reminder }: ReminderItemProps) {
  const remindTime = new Date(reminder.remind_time)
  const timeUntil = formatTimeUntil(remindTime)
  const formattedDate = formatDateTime(remindTime)

  return (
    <div className="border border-purple-500/20 bg-black/30 p-4">
      <div className="space-y-2">
        <p className="text-sm text-slate-200">{reminder.message}</p>

        <div className="flex flex-wrap items-center gap-3 text-xs">
          <span className="font-mono text-purple-400">{formattedDate}</span>
          <span className="text-slate-500">|</span>
          <span className="font-mono text-blue-400">{timeUntil}</span>
        </div>

        <div className="text-xs text-slate-500">
          Room:{' '}
          {reminder.room_name ? (
            <span className="text-slate-400">
              {reminder.room_name}{' '}
              <span className="font-mono text-slate-500">
                ({reminder.room_id})
              </span>
            </span>
          ) : (
            <span className="font-mono text-slate-400">{reminder.room_id}</span>
          )}
        </div>
      </div>
    </div>
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
