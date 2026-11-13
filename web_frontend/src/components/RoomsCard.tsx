import { useState } from 'react'
import { motion, AnimatePresence } from 'framer-motion'
import {
  useRooms,
  useRoomMembers,
  useRoomStats,
  useAdminRooms,
  useAdminRoomMembers,
  useAdminRoomStats,
} from '../api/queries'
import type { RoomResponse } from '../api/types'
import { useAuth } from '../context/AuthContext'
import { AnimatedList } from './ui/AnimatedList'
import { RoomMemberLabel } from './RoomMemberLabel'
import { RoomStats } from './RoomStats'
import { useReducedMotion } from '../hooks/useReducedMotion'

/**
 * Which rooms the card lists: the rooms the viewer shares with the bot, or, for an admin, every
 * room the bot is in, read through the admin endpoints
 */
type RoomScope = 'shared' | 'all'

const scopeTexts: Record<RoomScope, { loading: string; failed: string; empty: string }> = {
  shared: {
    loading: 'Loading rooms...',
    failed: 'Failed to load rooms',
    empty: 'No shared rooms',
  },
  all: {
    loading: 'Loading all rooms...',
    failed: 'Failed to load admin rooms',
    empty: 'No rooms found',
  },
}

/**
 * The rooms the viewer can look into: every room the bot is in for an admin, and the rooms they
 * share with the bot for everyone else.
 *
 * The two lists are separate components rather than one that picks its hooks by isAdmin, which
 * changes when someone logs in or out. Switching components keeps to the rules of hooks, and only
 * the list on screen is fetched.
 */
export function RoomsCard() {
  const { isAdmin } = useAuth()
  return isAdmin ? <AllRoomsList /> : <SharedRoomsList />
}

function SharedRoomsList() {
  const { data, isLoading, error } = useRooms()
  return <RoomList scope="shared" rooms={data?.rooms} isLoading={isLoading} error={error} />
}

function AllRoomsList() {
  const { data, isLoading, error } = useAdminRooms()
  return <RoomList scope="all" rooms={data?.rooms} isLoading={isLoading} error={error} />
}

interface RoomListProps {
  scope: RoomScope
  rooms: RoomResponse[] | undefined
  isLoading: boolean
  error: Error | null
}

function RoomList({ scope, rooms = [], isLoading, error }: RoomListProps) {
  const prefersReducedMotion = useReducedMotion()
  const texts = scopeTexts[scope]

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
        <span className="font-mono text-sm">{texts.loading}</span>
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
        <span className="text-sm text-rose-400">{texts.failed}</span>
      </motion.div>
    )
  }

  if (rooms.length === 0) {
    return (
      <motion.div
        className="border border-slate-700/50 bg-black/20 p-4"
        initial={prefersReducedMotion ? {} : { opacity: 0 }}
        animate={{ opacity: 1 }}
      >
        <span className="font-mono text-sm text-slate-500">{texts.empty}</span>
      </motion.div>
    )
  }

  return (
    <div className="space-y-2">
      {scope === 'all' && (
        <div className="font-mono text-xs text-slate-400">
          {rooms.length === 1 ? 'The one room the bot is in' : `All ${rooms.length} rooms the bot is in`}
        </div>
      )}
      <AnimatedList
        items={rooms}
        keyExtractor={(room) => room.room_id}
        className="space-y-3"
        renderItem={(room) => <RoomItem room={room} scope={scope} />}
      />
    </div>
  )
}

interface RoomItemProps {
  room: RoomResponse
  scope: RoomScope
}

function RoomItem({ room, scope }: RoomItemProps) {
  const prefersReducedMotion = useReducedMotion()
  const [isExpanded, setIsExpanded] = useState(false)
  const [statsDays, setStatsDays] = useState(30)

  // Both pairs of hooks are called, and only the pair of the item's scope is enabled: an expanded
  // room fetches its members and stats from the endpoints of the list it came from
  const isAll = scope === 'all'
  const sharedMembers = useRoomMembers(room.room_id, isExpanded && !isAll)
  const allMembers = useAdminRoomMembers(room.room_id, isExpanded && isAll)
  const sharedStats = useRoomStats(room.room_id, statsDays, isExpanded && !isAll)
  const allStats = useAdminRoomStats(room.room_id, statsDays, isExpanded && isAll)
  const { data: membersData, isLoading: membersLoading } = isAll ? allMembers : sharedMembers
  const {
    data: stats,
    isLoading: statsLoading,
    isPlaceholderData: statsRefreshing,
    error: statsError,
  } = isAll ? allStats : sharedStats

  const members = membersData?.members ?? []

  return (
    <motion.div
      className="glow-border-hover border border-purple-500/20 bg-black/30 transition-all duration-300"
      whileHover={prefersReducedMotion ? {} : { scale: 1.01, x: 4 }}
      transition={{ type: 'spring', stiffness: 400, damping: 17 }}
    >
      {/* Header - clickable to expand */}
      <div
        className="flex cursor-pointer items-center justify-between p-4"
        onClick={() => setIsExpanded(!isExpanded)}
      >
        <div className="text-sm text-slate-200">
          {room.room_name ? (
            <>
              <span>{room.room_name}</span>{' '}
              <span className="font-mono text-slate-500">({room.room_id})</span>
            </>
          ) : (
            <span className="font-mono">{room.room_id}</span>
          )}
        </div>
        <motion.span
          className="text-slate-400"
          animate={{ rotate: isExpanded ? 180 : 0 }}
          transition={{ duration: 0.2 }}
        >
          ▼
        </motion.span>
      </div>

      {/* Expanded content - stats and members list */}
      <AnimatePresence>
        {isExpanded && (
          <motion.div
            initial={prefersReducedMotion ? {} : { height: 0, opacity: 0 }}
            animate={{ height: 'auto', opacity: 1 }}
            exit={prefersReducedMotion ? {} : { height: 0, opacity: 0 }}
            transition={{ duration: 0.2 }}
            className="overflow-hidden border-t border-slate-700/50"
          >
            <div className="space-y-2 p-4">
              <RoomStats
                stats={stats}
                isLoading={statsLoading}
                isRefreshing={statsRefreshing}
                error={statsError}
                days={statsDays}
                onDaysChange={setStatsDays}
              />
              <div className="mt-4 mb-2 border-t border-slate-700/50 pt-4 font-mono text-xs text-slate-500">
                Members ({members.length})
              </div>
              {membersLoading ? (
                <div className="flex items-center gap-2 text-slate-400">
                  <motion.div
                    className="h-3 w-3 rounded-full border-2 border-purple-500 border-t-transparent"
                    animate={{ rotate: 360 }}
                    transition={{ duration: 1, repeat: Infinity, ease: 'linear' }}
                  />
                  <span className="font-mono text-xs">Loading members...</span>
                </div>
              ) : members.length === 0 ? (
                <div className="font-mono text-xs text-slate-500">No members found</div>
              ) : (
                <div className="space-y-1">
                  {members.map((member) => (
                    <motion.div
                      key={member.user_id}
                      className="rounded border border-slate-700/30 bg-black/20 px-3 py-2 text-xs text-slate-300"
                      initial={prefersReducedMotion ? {} : { opacity: 0, x: -10 }}
                      animate={{ opacity: 1, x: 0 }}
                      transition={{ type: 'spring', stiffness: 300, damping: 20 }}
                    >
                      <RoomMemberLabel member={member} />
                    </motion.div>
                  ))}
                </div>
              )}
            </div>
          </motion.div>
        )}
      </AnimatePresence>
    </motion.div>
  )
}
