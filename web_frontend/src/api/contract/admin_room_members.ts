// A real response of GET /api/admin/rooms/:roomId/members, saved by the end-to-end test TestAPIContract. tsc checks that RoomMembersResponse fits it,
// and the test that the API still answers in its shape. Don't edit it by hand: run the test with
// SIIKABOT_E2E_UPDATE_FIXTURES=1 to save it again.
import type { RoomMembersResponse } from '../types'

export const adminRoomMembers: RoomMembersResponse = {
  "members": [
    {
      "user_id": "@alice1:localhost",
      "display_name": "alice1"
    },
    {
      "user_id": "@bob2:localhost",
      "display_name": "bob2"
    },
    {
      "user_id": "@siikabot:localhost",
      "display_name": "siikabot"
    }
  ]
}
