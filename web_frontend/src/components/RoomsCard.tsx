import { useRooms } from '../api/queries'
import type { RoomResponse } from '../api/types'

export function RoomsCard() {
  const { data, isLoading, error } = useRooms()

  if (isLoading) {
    return (
      <div className="flex items-center gap-2 text-slate-400">
        <div className="h-4 w-4 animate-spin rounded-full border-2 border-purple-500 border-t-transparent" />
        <span className="font-mono text-sm">Loading rooms...</span>
      </div>
    )
  }

  if (error) {
    return (
      <div className="border border-rose-500/50 bg-rose-950/30 p-4">
        <span className="text-sm text-rose-400">Failed to load rooms</span>
      </div>
    )
  }

  const rooms = data?.rooms ?? []

  if (rooms.length === 0) {
    return (
      <div className="border border-slate-700/50 bg-black/20 p-4">
        <span className="font-mono text-sm text-slate-500">No shared rooms</span>
      </div>
    )
  }

  return (
    <div className="space-y-3">
      {rooms.map((room) => (
        <RoomItem key={room.room_id} room={room} />
      ))}
    </div>
  )
}

interface RoomItemProps {
  room: RoomResponse
}

function RoomItem({ room }: RoomItemProps) {
  return (
    <div className="border border-purple-500/20 bg-black/30 p-4">
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
    </div>
  )
}
