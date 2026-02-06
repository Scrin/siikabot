import { motion } from 'framer-motion'
import { useAdminRooms } from '../api/queries'
import type { RoomResponse } from '../api/types'
import { AnimatedList } from './ui/AnimatedList'
import { useReducedMotion } from '../hooks/useReducedMotion'

export function AdminRoomsCard() {
  const { data, isLoading, error } = useAdminRooms()
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
        <span className="font-mono text-sm">Loading all rooms...</span>
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
        <span className="text-sm text-rose-400">Failed to load admin rooms</span>
      </motion.div>
    )
  }

  const rooms = data?.rooms ?? []

  if (rooms.length === 0) {
    return (
      <motion.div
        className="border border-slate-700/50 bg-black/20 p-4"
        initial={prefersReducedMotion ? {} : { opacity: 0 }}
        animate={{ opacity: 1 }}
      >
        <span className="font-mono text-sm text-slate-500">No rooms found</span>
      </motion.div>
    )
  }

  return (
    <div className="space-y-2">
      <div className="text-xs text-slate-400 font-mono">Total rooms: {rooms.length}</div>
      <AnimatedList
        items={rooms}
        keyExtractor={(room) => room.room_id}
        className="space-y-3"
        renderItem={(room) => <AdminRoomItem room={room} />}
      />
    </div>
  )
}

interface AdminRoomItemProps {
  room: RoomResponse
}

function AdminRoomItem({ room }: AdminRoomItemProps) {
  const prefersReducedMotion = useReducedMotion()

  return (
    <motion.div
      className="glow-border-hover border border-purple-500/20 bg-black/30 p-4 transition-all duration-300"
      whileHover={prefersReducedMotion ? {} : { scale: 1.01, x: 4 }}
      transition={{ type: 'spring', stiffness: 400, damping: 17 }}
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
    </motion.div>
  )
}
