import { useState } from 'react'
import { motion, AnimatePresence } from 'framer-motion'
import { useMemories, useDeleteMemory, useDeleteAllMemories } from '../api/queries'
import type { MemoryResponse } from '../api/types'
import { formatDateTime } from '../utils/dateTime'
import { AnimatedList } from './ui/AnimatedList'
import { useReducedMotion } from '../hooks/useReducedMotion'

export function MemoriesCard() {
  const { data, isLoading, error } = useMemories()
  const deleteAllMemories = useDeleteAllMemories()
  const prefersReducedMotion = useReducedMotion()
  const [showClearConfirm, setShowClearConfirm] = useState(false)

  const handleClearAll = async () => {
    try {
      await deleteAllMemories.mutateAsync()
      setShowClearConfirm(false)
    } catch {
      // Error handled by mutation
    }
  }

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
        <span className="font-mono text-sm">Loading memories...</span>
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
        <span className="text-sm text-rose-400">Failed to load memories</span>
      </motion.div>
    )
  }

  const memories = data?.memories ?? []

  if (memories.length === 0) {
    return (
      <motion.div
        className="border border-slate-700/50 bg-black/20 p-4"
        initial={prefersReducedMotion ? {} : { opacity: 0 }}
        animate={{ opacity: 1 }}
      >
        <span className="font-mono text-sm text-slate-500">No memories stored</span>
      </motion.div>
    )
  }

  return (
    <div className="space-y-4">
      {/* Header with count and clear all button */}
      <div className="flex items-center justify-between">
        <span className="font-mono text-xs text-slate-500">
          {memories.length} {memories.length === 1 ? 'memory' : 'memories'} stored
        </span>

        <AnimatePresence mode="wait">
          {showClearConfirm ? (
            <motion.div
              key="confirm"
              className="flex items-center gap-2"
              initial={prefersReducedMotion ? {} : { opacity: 0, x: 10 }}
              animate={{ opacity: 1, x: 0 }}
              exit={prefersReducedMotion ? {} : { opacity: 0, x: 10 }}
            >
              <span className="text-xs text-slate-400">Clear all?</span>
              <motion.button
                className="border border-rose-500/50 bg-rose-500/20 px-3 py-1 font-mono text-xs text-rose-300 transition-all hover:bg-rose-500/30 disabled:cursor-not-allowed disabled:opacity-50"
                onClick={handleClearAll}
                disabled={deleteAllMemories.isPending}
                whileHover={prefersReducedMotion ? {} : { scale: 1.02 }}
                whileTap={prefersReducedMotion ? {} : { scale: 0.98 }}
              >
                {deleteAllMemories.isPending ? 'Clearing...' : 'Yes'}
              </motion.button>
              <motion.button
                className="border border-slate-500/50 bg-slate-500/20 px-3 py-1 font-mono text-xs text-slate-300 transition-all hover:bg-slate-500/30"
                onClick={() => setShowClearConfirm(false)}
                whileHover={prefersReducedMotion ? {} : { scale: 1.02 }}
                whileTap={prefersReducedMotion ? {} : { scale: 0.98 }}
              >
                No
              </motion.button>
            </motion.div>
          ) : (
            <motion.button
              key="clear-btn"
              className="border border-rose-500/30 bg-rose-500/10 px-3 py-1 font-mono text-xs text-rose-400 transition-all hover:border-rose-500/50 hover:bg-rose-500/20"
              onClick={() => setShowClearConfirm(true)}
              initial={prefersReducedMotion ? {} : { opacity: 0, x: 10 }}
              animate={{ opacity: 1, x: 0 }}
              exit={prefersReducedMotion ? {} : { opacity: 0, x: 10 }}
              whileHover={prefersReducedMotion ? {} : { scale: 1.02 }}
              whileTap={prefersReducedMotion ? {} : { scale: 0.98 }}
            >
              Clear all
            </motion.button>
          )}
        </AnimatePresence>
      </div>

      {deleteAllMemories.isError && (
        <p className="text-xs text-rose-400">
          {deleteAllMemories.error?.message || 'Failed to clear memories'}
        </p>
      )}

      {/* Memories list */}
      <AnimatedList
        items={memories}
        keyExtractor={(memory) => memory.id}
        className="space-y-3"
        renderItem={(memory) => <MemoryItem memory={memory} />}
      />
    </div>
  )
}

interface MemoryItemProps {
  memory: MemoryResponse
}

function MemoryItem({ memory }: MemoryItemProps) {
  const deleteMemory = useDeleteMemory()
  const prefersReducedMotion = useReducedMotion()
  const [showDeleteConfirm, setShowDeleteConfirm] = useState(false)
  const createdAt = new Date(memory.created_at)
  const formattedDate = formatDateTime(createdAt)

  const handleDelete = async () => {
    try {
      await deleteMemory.mutateAsync(memory.id)
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
        <p className="whitespace-pre-wrap text-sm text-slate-200">{memory.memory}</p>

        <div className="flex flex-wrap items-center justify-between gap-3">
          <span className="font-mono text-xs text-purple-400">{formattedDate}</span>

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
                  disabled={deleteMemory.isPending}
                  whileHover={prefersReducedMotion ? {} : { scale: 1.1 }}
                >
                  {deleteMemory.isPending ? '...' : 'Confirm'}
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

        {deleteMemory.isError && (
          <p className="text-xs text-rose-400">
            {deleteMemory.error?.message || 'Failed to delete memory'}
          </p>
        )}
      </div>
    </motion.div>
  )
}
