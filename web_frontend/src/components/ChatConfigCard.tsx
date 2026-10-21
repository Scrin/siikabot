import { motion } from 'framer-motion'
import { useChatConfig } from '../api/queries'
import type { ChatConfigVersion, ChatSetting } from '../api/types'
import { useReducedMotion } from '../hooks/useReducedMotion'
import { formatDateTime } from '../utils/dateTime'

/**
 * The chat configuration in effect, under the same headings as !chat config, and the latest changes
 * to it. Read-only: a setting is changed by sending its command in a room, which answers there.
 */
export function ChatConfigCard() {
  const { data, isLoading, error } = useChatConfig()
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
        <span className="font-mono text-sm">Loading chat configuration...</span>
      </motion.div>
    )
  }

  if (error || !data) {
    return (
      <motion.div
        className="border border-rose-500/50 bg-rose-950/30 p-4"
        initial={prefersReducedMotion ? {} : { opacity: 0, x: -10 }}
        animate={{ opacity: 1, x: 0 }}
      >
        <span className="text-sm text-rose-400">Failed to load chat configuration</span>
      </motion.div>
    )
  }

  return (
    <div className="space-y-3">
      <div className="font-mono text-xs text-slate-500">
        Configuration #{data.id}, set {formatDateTime(new Date(data.created_at))}. Change a setting
        by sending its command in a room.
      </div>

      {groupSettings(data.settings).map(([group, settings]) => (
        <SettingGroup key={group} name={group} settings={settings} />
      ))}

      <ConfigHistory history={data.history} />
    </div>
  )
}

interface SettingGroupProps {
  name: string
  settings: ChatSetting[]
}

function SettingGroup({ name, settings }: SettingGroupProps) {
  return (
    <div className="border border-purple-500/20 bg-black/30 p-4">
      <div className="mb-3 font-mono text-xs tracking-wider text-purple-400 uppercase">{name}</div>
      <div className="space-y-3">
        {settings.map((setting) => (
          <div key={setting.key} className="space-y-1">
            <div className="flex flex-wrap items-baseline gap-2">
              <span className="font-mono text-sm text-blue-400">{setting.key}</span>
              <span className="text-sm text-slate-200">{withUnit(setting.value, setting.unit)}</span>
            </div>
            <div className="text-xs text-slate-400">{setting.summary}</div>
            <code className="block font-mono text-xs text-slate-500 select-all">
              {setting.command}
            </code>
          </div>
        ))}
      </div>
    </div>
  )
}

interface ConfigHistoryProps {
  history: ChatConfigVersion[]
}

function ConfigHistory({ history }: ConfigHistoryProps) {
  return (
    <div className="border border-slate-700/50 bg-black/20 p-4">
      <div className="mb-3 font-mono text-xs tracking-wider text-slate-400 uppercase">
        Latest changes
      </div>
      <ul className="space-y-1 font-mono text-xs">
        {history.map((version) => (
          <li key={version.id} className="text-slate-400">
            <span className="text-purple-400">#{version.id}</span>{' '}
            <span className="text-slate-500">{formatDateTime(new Date(version.created_at))}</span>{' '}
            {describeVersion(version)}
          </li>
        ))}
      </ul>
      <div className="mt-3 font-mono text-xs text-slate-500">
        Go back to one in a room:{' '}
        <code className="select-all">{'!chat config revert <id>'}</code>
      </div>
    </div>
  )
}

/**
 * Group the settings under their headings, keeping the order they come in, which is that of
 * !chat config
 */
function groupSettings(settings: ChatSetting[]): [string, ChatSetting[]][] {
  const groups: [string, ChatSetting[]][] = []
  for (const setting of settings) {
    const last = groups[groups.length - 1]
    if (last && last[0] === setting.group) {
      last[1].push(setting)
    } else {
      groups.push([setting.group, [setting]])
    }
  }
  return groups
}

/**
 * Say what a configuration changed from the one before it, as !chat config history does
 *
 * @example
 * describeVersion(version) // "turn_timeout 3m → 10m, max_tool_iterations 5 → 8"
 */
function describeVersion(version: ChatConfigVersion): string {
  if (version.first) {
    return 'the first configuration'
  }
  if (version.changes.length === 0) {
    return 'no change'
  }
  return version.changes
    .map((change) => `${change.key} ${change.from} → ${withUnit(change.to, change.unit)}`)
    .join(', ')
}

function withUnit(value: string, unit: string): string {
  return unit ? `${value} ${unit}` : value
}
