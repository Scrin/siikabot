import { useState } from 'react'
import { motion, AnimatePresence } from 'framer-motion'
import { useRoomMemories, useDeleteRoomMemory, useDeleteRoomMemories } from '../api/queries'
import type { RoomMemories, RoomMemory } from '../api/types'
import { formatDateTime } from '../utils/dateTime'
import { AnimatedList } from './ui/AnimatedList'
import { useReducedMotion } from '../hooks/useReducedMotion'

/**
 * The bot's own memories of each group room (admin only). The bot forms them by thinking back over
 * its conversations in a room once the room has gone quiet, and the admin can delete them here.
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

/** One room's memories, with the control that forgets all of them */
function RoomMemoriesGroup({ room }: RoomMemoriesGroupProps) {
  const deleteRoomMemories = useDeleteRoomMemories()
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
        </div>

        <AnimatePresence mode="wait">
          {showForgetConfirm ? (
            <motion.div
              key="confirm"
              className="flex items-center gap-2"
              initial={prefersReducedMotion ? {} : { opacity: 0, x: 10 }}
              animate={{ opacity: 1, x: 0 }}
              exit={prefersReducedMotion ? {} : { opacity: 0, x: 10 }}
            >
              <span className="text-xs text-slate-400">Forget everything about {roomLabel}?</span>
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
      </div>

      {deleteRoomMemories.isError && (
        <p className="text-xs text-rose-400">
          {deleteRoomMemories.error?.message || 'Failed to delete the memories'}
        </p>
      )}

      <AnimatedList
        items={room.memories}
        keyExtractor={(memory) => memory.id}
        className="space-y-3"
        renderItem={(memory) => <RoomMemoryItem memory={memory} />}
      />
    </section>
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
          <span className="text-xs text-slate-500">
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
