import type { RoomMemberResponse } from '../api/types'

interface RoomMemberLabelProps {
  member: RoomMemberResponse
}

/** A room member as the room cards list them: their display name and user ID, or the ID alone */
export function RoomMemberLabel({ member }: RoomMemberLabelProps) {
  if (!member.display_name) {
    return <span className="font-mono">{member.user_id}</span>
  }
  return (
    <>
      <span>{member.display_name}</span>{' '}
      <span className="font-mono text-slate-500">({member.user_id})</span>
    </>
  )
}
