import { useState } from 'react'
import { motion, AnimatePresence } from 'framer-motion'
import {
  useRoomMemories,
  useDeleteRoomMemory,
  useDeleteRoomMemories,
  useReflectRoom,
} from '../api/queries'
import type { ReflectResponse, RoomMemories, RoomMemory, RoomMemoryStatus } from '../api/types'
import { formatDateTime } from '../utils/dateTime'
import { AnimatedList } from './ui/AnimatedList'
import { CopyButton } from './CopyButton'
import { useReducedMotion } from '../hooks/useReducedMotion'

/**
 * The bot's own memories of each group room (admin only). The bot forms them by thinking back over
 * its conversations in a room once the room has gone quiet. The card says how each room stands with
 * those passes and how the latest one went, with the traces to look them up by, runs a pass at once on
 * request, and deletes memories.
 */
export function RoomMemoriesCard() {
  const { data, isLoading, error } = useRoomMemories()
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
        <span className="font-mono text-sm">Loading room memories...</span>
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
        <span className="text-sm text-rose-400">Failed to load room memories</span>
      </motion.div>
    )
  }

  const rooms = data?.rooms ?? []

  return (
    <div className="space-y-4">
      <p className="text-xs text-slate-500">
        What the bot has come to remember about each group room on its own. It uses them only in
        that room, revises them after conversations there, and doesn't mention them unless asked.
      </p>

      {rooms.length === 0 ? (
        <motion.div
          className="border border-slate-700/50 bg-black/20 p-4"
          initial={prefersReducedMotion ? {} : { opacity: 0 }}
          animate={{ opacity: 1 }}
        >
          <span className="font-mono text-sm text-slate-500">No room memories yet</span>
        </motion.div>
      ) : (
        <AnimatedList
          items={rooms}
          keyExtractor={(room) => room.room_id}
          className="space-y-5"
          renderItem={(room) => <RoomMemoriesGroup room={room} />}
        />
      )}
    </div>
  )
}

interface RoomMemoriesGroupProps {
  room: RoomMemories
}

/** One room: how it stands with the passes, its memories, and the controls for both */
function RoomMemoriesGroup({ room }: RoomMemoriesGroupProps) {
  const deleteRoomMemories = useDeleteRoomMemories()
  const reflectRoom = useReflectRoom()
  const prefersReducedMotion = useReducedMotion()
  const [showForgetConfirm, setShowForgetConfirm] = useState(false)
  const roomLabel = room.room_name || room.room_id

  const handleForget = async () => {
    try {
      await deleteRoomMemories.mutateAsync(room.room_id)
      setShowForgetConfirm(false)
    } catch {
      // Error handled by mutation
    }
  }

  const handleReflect = async () => {
    try {
      await reflectRoom.mutateAsync(room.room_id)
    } catch {
      // Error handled by mutation
    }
  }

  return (
    <section aria-label={`Memories of ${roomLabel}`} className="space-y-3">
      <div className="flex flex-wrap items-start justify-between gap-3">
        <div className="min-w-0 space-y-1">
          <div className="text-sm text-slate-300">
            {room.room_name ? (
              <>
                {room.room_name}{' '}
                <span className="font-mono text-xs break-all text-slate-500">({room.room_id})</span>
              </>
            ) : (
              <span className="font-mono text-xs break-all">{room.room_id}</span>
            )}
          </div>
          <div className="text-xs text-slate-500">
            {room.memories.length} {room.memories.length === 1 ? 'memory' : 'memories'}
            {room.last_reflected_at && (
              <>
                {' · '}last thought back{' '}
                <span className="font-mono text-purple-400">
                  {formatDateTime(new Date(room.last_reflected_at))}
                </span>
              </>
            )}
          </div>
          <RoomStatusLine status={room.status} />
          <LastPassLine status={room.status} />
        </div>

        <div className="flex flex-wrap items-center gap-2">
          <motion.button
            className="border border-purple-500/40 bg-purple-500/10 px-3 py-1 font-mono text-xs text-purple-300 transition-all hover:border-purple-500/60 hover:bg-purple-500/20 disabled:cursor-not-allowed disabled:opacity-50"
            onClick={handleReflect}
            disabled={reflectRoom.isPending}
            whileHover={prefersReducedMotion ? {} : { scale: 1.02 }}
            whileTap={prefersReducedMotion ? {} : { scale: 0.98 }}
          >
            {reflectRoom.isPending ? 'Thinking back...' : 'Think back now'}
          </motion.button>

          {room.memories.length > 0 && (
            <AnimatePresence mode="wait">
              {showForgetConfirm ? (
                <motion.div
                  key="confirm"
                  className="flex items-center gap-2"
                  initial={prefersReducedMotion ? {} : { opacity: 0, x: 10 }}
                  animate={{ opacity: 1, x: 0 }}
                  exit={prefersReducedMotion ? {} : { opacity: 0, x: 10 }}
                >
                  <span className="text-xs text-slate-400">
                    Forget everything about {roomLabel}?
                  </span>
                  <motion.button
                    className="border border-rose-500/50 bg-rose-500/20 px-3 py-1 font-mono text-xs text-rose-300 transition-all hover:bg-rose-500/30 disabled:cursor-not-allowed disabled:opacity-50"
                    onClick={handleForget}
                    disabled={deleteRoomMemories.isPending}
                    whileHover={prefersReducedMotion ? {} : { scale: 1.02 }}
                    whileTap={prefersReducedMotion ? {} : { scale: 0.98 }}
                  >
                    {deleteRoomMemories.isPending ? 'Forgetting...' : 'Yes'}
                  </motion.button>
                  <motion.button
                    className="border border-slate-500/50 bg-slate-500/20 px-3 py-1 font-mono text-xs text-slate-300 transition-all hover:bg-slate-500/30"
                    onClick={() => setShowForgetConfirm(false)}
                    whileHover={prefersReducedMotion ? {} : { scale: 1.02 }}
                    whileTap={prefersReducedMotion ? {} : { scale: 0.98 }}
                  >
                    No
                  </motion.button>
                </motion.div>
              ) : (
                <motion.button
                  key="forget-btn"
                  className="border border-rose-500/30 bg-rose-500/10 px-3 py-1 font-mono text-xs text-rose-400 transition-all hover:border-rose-500/50 hover:bg-rose-500/20"
                  onClick={() => setShowForgetConfirm(true)}
                  initial={prefersReducedMotion ? {} : { opacity: 0, x: 10 }}
                  animate={{ opacity: 1, x: 0 }}
                  exit={prefersReducedMotion ? {} : { opacity: 0, x: 10 }}
                  whileHover={prefersReducedMotion ? {} : { scale: 1.02 }}
                  whileTap={prefersReducedMotion ? {} : { scale: 0.98 }}
                >
                  Forget this room
                </motion.button>
              )}
            </AnimatePresence>
          )}
        </div>
      </div>

      {reflectRoom.isSuccess && reflectRoom.data && <ReflectionResult result={reflectRoom.data} />}
      {reflectRoom.isError && (
        <p className="text-xs text-rose-400">
          {reflectRoom.error?.message || 'Failed to think back over the room'}
        </p>
      )}
      {deleteRoomMemories.isError && (
        <p className="text-xs text-rose-400">
          {deleteRoomMemories.error?.message || 'Failed to delete the memories'}
        </p>
      )}

      {room.memories.length === 0 ? (
        <p className="font-mono text-xs text-slate-500">No memories of this room yet</p>
      ) : (
        <AnimatedList
          items={room.memories}
          keyExtractor={(memory) => memory.id}
          className="space-y-3"
          renderItem={(memory) => <RoomMemoryItem memory={memory} />}
        />
      )}
    </section>
  )
}

/** How a pass's outcome is described */
const OUTCOME_LABELS: Record<string, string> = {
  changed: 'changed the memories',
  unchanged: 'no changes',
  nothing_new: 'nothing new',
  direct_chat: 'a direct chat, which gets no memories',
  room_unknown: "the room couldn't be looked up",
  history_unavailable: "the chat history couldn't be read",
  memories_unavailable: "the memories couldn't be read",
  timeout: 'timed out (turn_timeout)',
  request_failed: 'model call failed',
  no_choices: 'no answer from the model',
  invalid_changes: "the model's changes couldn't be read",
  save_failed: "the changes couldn't be saved",
}

/** Describes a pass's outcome, by its own name when it has no description, so a new one still shows */
function describeOutcome(outcome: string): string {
  return OUTCOME_LABELS[outcome] ?? outcome
}

/** Says how a room stands with the schedule of passes */
function RoomStatusLine({ status }: { status: RoomMemoryStatus }) {
  const dueAt = status.due_at ? formatDateTime(new Date(status.due_at)) : null

  switch (status.state) {
    case 'up_to_date':
      return <div className="text-xs text-slate-500">Up to date</div>
    case 'waiting':
      return (
        <div className="text-xs text-slate-400">
          New conversations, waiting for the room to be quiet · due{' '}
          <span className="font-mono text-purple-400">{dueAt}</span>
        </div>
      )
    case 'due':
      return (
        <div className="text-xs text-slate-400">
          Due now: the next check, within 5 minutes, thinks back over it
        </div>
      )
    case 'retrying':
      return (
        <div className="text-xs text-rose-400">
          {status.failed_attempts} failed {status.failed_attempts === 1 ? 'pass' : 'passes'} in a
          row · retrying <span className="font-mono">{dueAt}</span>
        </div>
      )
    case 'off':
      return (
        <div className="text-xs text-slate-400">
          New conversations, but the passes are off (room_memory_delay)
        </div>
      )
  }
}

/** Says how the room's latest pass went, with its trace */
function LastPassLine({ status }: { status: RoomMemoryStatus }) {
  if (!status.last_outcome) {
    return null
  }
  return (
    <div className="flex flex-wrap items-center gap-x-2 gap-y-1 text-xs text-slate-500">
      <span>
        Last pass: {describeOutcome(status.last_outcome)}
        {status.last_attempt_at && (
          <>
            {' '}
            <span className="font-mono text-purple-400">
              {formatDateTime(new Date(status.last_attempt_at))}
            </span>
          </>
        )}
      </span>
      {status.last_trace_id && <TraceID id={status.last_trace_id} />}
    </div>
  )
}

/**
 * Says how a pass the admin started went
 *
 * @example
 * describeReflection(result) // "Thought back over 4 conversations (3 shown): 1 added, 2 updated"
 */
function describeReflection(result: ReflectResponse): string {
  if (result.skipped || result.failed) {
    return `${result.failed ? 'The pass failed' : 'Skipped'}: ${describeOutcome(result.outcome)}`
  }
  const shown =
    result.shown_conversation_count < result.conversation_count
      ? ` (${result.shown_conversation_count} shown)`
      : ''
  const changes = [
    result.added > 0 && `${result.added} added`,
    result.updated > 0 && `${result.updated} updated`,
    result.deleted > 0 && `${result.deleted} deleted`,
    result.rejected > 0 && `${result.rejected} rejected`,
  ].filter(Boolean)
  const conversations = `${result.conversation_count} ${result.conversation_count === 1 ? 'conversation' : 'conversations'}`
  return `Thought back over ${conversations}${shown}: ${changes.length > 0 ? changes.join(', ') : 'no changes'}`
}

/** The result of a pass the admin started, with its trace */
function ReflectionResult({ result }: { result: ReflectResponse }) {
  return (
    <div
      className={`flex flex-wrap items-center gap-x-2 gap-y-1 text-xs ${result.failed ? 'text-rose-400' : 'text-slate-300'}`}
    >
      <span>{describeReflection(result)}</span>
      {result.trace_id && <TraceID id={result.trace_id} />}
    </div>
  )
}

/** A trace ID, shortened, with a button that copies it whole */
function TraceID({ id }: { id: string }) {
  return (
    <span className="inline-flex items-center gap-2">
      <span className="font-mono text-slate-500" title={id}>
        trace {id.slice(0, 8)}
      </span>
      <CopyButton text={id} label="trace ID" />
    </span>
  )
}

interface RoomMemoryItemProps {
  memory: RoomMemory
}

function RoomMemoryItem({ memory }: RoomMemoryItemProps) {
  const deleteRoomMemory = useDeleteRoomMemory()
  const prefersReducedMotion = useReducedMotion()
  const [showDeleteConfirm, setShowDeleteConfirm] = useState(false)
  const revised = memory.updated_at !== memory.created_at
  const traceID = memory.updated_trace_id ?? memory.created_trace_id

  const handleDelete = async () => {
    try {
      await deleteRoomMemory.mutateAsync(memory.id)
    } catch {
      // Error handled by mutation
    }
  }

  return (
    <motion.div
      className="glow-border-hover border border-purple-500/20 bg-black/30 p-4 transition-all duration-300"
      whileHover={prefersReducedMotion ? {} : { scale: 1.01, x: 4 }}
      transition={{ type: 'spring', stiffness: 400, damping: 17 }}
    >
      <div className="space-y-2">
        <p className="text-sm whitespace-pre-wrap text-slate-200">{memory.memory}</p>

        <div className="flex flex-wrap items-center justify-between gap-3">
          <span className="flex flex-wrap items-center gap-x-2 gap-y-1 text-xs text-slate-500">
            <span>
              Formed{' '}
              <span className="font-mono text-purple-400">
                {formatDateTime(new Date(memory.created_at))}
              </span>
              {revised && (
                <>
                  {' · '}revised{' '}
                  <span className="font-mono text-purple-400">
                    {formatDateTime(new Date(memory.updated_at))}
                  </span>
                </>
              )}
            </span>
            {traceID && <TraceID id={traceID} />}
          </span>

          <AnimatePresence mode="wait">
            {showDeleteConfirm ? (
              <motion.div
                key="confirm"
                className="flex items-center gap-2"
                initial={prefersReducedMotion ? {} : { opacity: 0, scale: 0.9 }}
                animate={{ opacity: 1, scale: 1 }}
                exit={prefersReducedMotion ? {} : { opacity: 0, scale: 0.9 }}
              >
                <motion.button
                  className="text-xs text-rose-400 hover:text-rose-300 disabled:opacity-50"
                  onClick={handleDelete}
                  disabled={deleteRoomMemory.isPending}
                  whileHover={prefersReducedMotion ? {} : { scale: 1.1 }}
                >
                  {deleteRoomMemory.isPending ? '...' : 'Confirm'}
                </motion.button>
                <motion.button
                  className="text-xs text-slate-400 hover:text-slate-300"
                  onClick={() => setShowDeleteConfirm(false)}
                  whileHover={prefersReducedMotion ? {} : { scale: 1.1 }}
                >
                  Cancel
                </motion.button>
              </motion.div>
            ) : (
              <motion.button
                key="delete-btn"
                className="text-xs text-rose-400 hover:text-rose-300"
                onClick={() => setShowDeleteConfirm(true)}
                initial={prefersReducedMotion ? {} : { opacity: 0 }}
                animate={{ opacity: 1 }}
                exit={prefersReducedMotion ? {} : { opacity: 0 }}
                whileHover={prefersReducedMotion ? {} : { scale: 1.1 }}
              >
                Delete
              </motion.button>
            )}
          </AnimatePresence>
        </div>

        {deleteRoomMemory.isError && (
          <p className="text-xs text-rose-400">
            {deleteRoomMemory.error?.message || 'Failed to delete the memory'}
          </p>
        )}
      </div>
    </motion.div>
  )
}
