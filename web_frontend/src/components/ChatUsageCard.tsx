import { useState } from 'react'
import { motion } from 'framer-motion'
import { useChatUsage } from '../api/queries'
import type { ChatUsageEntry } from '../api/types'
import { AnimatedList } from './ui/AnimatedList'
import { useReducedMotion } from '../hooks/useReducedMotion'

const DAY_RANGES = [1, 7, 30] as const

export function ChatUsageCard() {
  const [days, setDays] = useState<number>(7)
  const { data, isLoading, error } = useChatUsage(days)
  const prefersReducedMotion = useReducedMotion()

  const rangeSelector = (
    <div className="flex items-center gap-2">
      <span className="font-mono text-xs text-slate-500">Period:</span>
      {DAY_RANGES.map((range) => (
        <button
          key={range}
          onClick={() => setDays(range)}
          className={`border px-2 py-1 font-mono text-xs transition-colors duration-200 ${
            days === range
              ? 'border-purple-500/50 bg-purple-500/20 text-purple-300'
              : 'border-slate-700/50 text-slate-400 hover:border-slate-600 hover:text-slate-300'
          }`}
        >
          {range}d
        </button>
      ))}
    </div>
  )

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
        <span className="font-mono text-sm">Loading chat usage...</span>
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
        <span className="text-sm text-rose-400">Failed to load chat usage</span>
      </motion.div>
    )
  }

  const entries = data?.entries ?? []
  const totals = data?.totals

  if (entries.length === 0) {
    return (
      <div className="space-y-3">
        {rangeSelector}
        <motion.div
          className="border border-slate-700/50 bg-black/20 p-4"
          initial={prefersReducedMotion ? {} : { opacity: 0 }}
          animate={{ opacity: 1 }}
        >
          <span className="font-mono text-sm text-slate-500">
            No chat usage recorded in the last {days}d
          </span>
        </motion.div>
      </div>
    )
  }

  return (
    <div className="space-y-3">
      {rangeSelector}

      {totals && <UsageTotals totals={totals} />}

      <AnimatedList
        items={entries}
        keyExtractor={(entry) => `${entry.room_id}:${entry.model}`}
        renderItem={(entry) => <UsageItem entry={entry} />}
      />
    </div>
  )
}

interface UsageTotalsProps {
  totals: ChatUsageEntry
}

function UsageTotals({ totals }: UsageTotalsProps) {
  return (
    <div className="border border-purple-500/30 bg-purple-950/20 p-4">
      <div className="grid grid-cols-2 gap-3 sm:grid-cols-4">
        <Stat label="Turns" value={formatCount(totals.turns)} />
        <Stat label="Prompt tokens" value={formatCount(totals.prompt_tokens)} />
        <Stat label="Completion tokens" value={formatCount(totals.completion_tokens)} />
        <Stat
          label="Cache hit rate"
          value={formatPercent(totals.cache_hit_rate)}
          tone={cacheTone(totals.cache_hit_rate)}
        />
      </div>
      {totals.failures > 0 && (
        <div className="mt-3 font-mono text-xs text-rose-400">
          {totals.failures} failed {totals.failures === 1 ? 'turn' : 'turns'}
        </div>
      )}
    </div>
  )
}

interface StatProps {
  label: string
  value: string
  tone?: string
}

function Stat({ label, value, tone = 'text-slate-200' }: StatProps) {
  return (
    <div>
      <div className="font-mono text-xs tracking-wider text-slate-500 uppercase">{label}</div>
      <div className={`font-mono text-lg ${tone}`}>{value}</div>
    </div>
  )
}

interface UsageItemProps {
  entry: ChatUsageEntry
}

function UsageItem({ entry }: UsageItemProps) {
  const prefersReducedMotion = useReducedMotion()

  return (
    <motion.div
      className="glow-border-hover border border-purple-500/20 bg-black/30 p-4 transition-all duration-300"
      whileHover={prefersReducedMotion ? {} : { scale: 1.01, x: 4 }}
      transition={{ type: 'spring', stiffness: 400, damping: 17 }}
    >
      <div className="space-y-2">
        <div className="flex flex-wrap items-baseline gap-2">
          <span className="text-sm text-slate-200">{entry.room_name || entry.room_id}</span>
          <span className="font-mono text-xs text-blue-400">{entry.model}</span>
        </div>

        <div className="flex flex-wrap items-center gap-3 font-mono text-xs">
          <span className="text-purple-400">{formatCount(entry.turns)} turns</span>
          <span className="text-slate-500">|</span>
          <span className="text-slate-400">
            {formatCount(entry.prompt_tokens)} in / {formatCount(entry.completion_tokens)} out
          </span>
          <span className="text-slate-500">|</span>
          <span className={cacheTone(entry.cache_hit_rate)}>
            {formatPercent(entry.cache_hit_rate)} cached
          </span>
          {entry.tool_iterations > 0 && (
            <>
              <span className="text-slate-500">|</span>
              <span className="text-slate-400">{formatCount(entry.tool_iterations)} tool iters</span>
            </>
          )}
          {entry.failures > 0 && (
            <>
              <span className="text-slate-500">|</span>
              <span className="text-rose-400">{entry.failures} failed</span>
            </>
          )}
        </div>

        {entry.room_name && (
          <div className="font-mono text-xs text-slate-500">{entry.room_id}</div>
        )}
      </div>
    </motion.div>
  )
}

/**
 * Colour the cache hit rate by how much of the prompt is being served from cache. A prompt prefix
 * that is stable between turns is what makes caching possible, so a low rate here is the signal
 * that something has destabilised it.
 */
function cacheTone(rate: number): string {
  if (rate >= 0.5) {
    return 'text-emerald-400'
  }
  if (rate >= 0.2) {
    return 'text-amber-400'
  }
  return 'text-slate-400'
}

function formatPercent(rate: number): string {
  return `${Math.round(rate * 100)}%`
}

function formatCount(value: number): string {
  if (value >= 1_000_000) {
    return `${(value / 1_000_000).toFixed(1)}M`
  }
  if (value >= 10_000) {
    return `${Math.round(value / 1000)}k`
  }
  if (value >= 1000) {
    return `${(value / 1000).toFixed(1)}k`
  }
  return String(value)
}
