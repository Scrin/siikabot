// A real response of GET /api/admin/rooms/:roomId/stats, saved by the end-to-end test TestAPIContract. tsc checks that RoomStatsResponse fits it,
// and the test that the API still answers in its shape. Don't edit it by hand: run the test with
// SIIKABOT_E2E_UPDATE_FIXTURES=1 to save it again.
import type { RoomStatsResponse } from '../types'

export const adminRoomStats: RoomStatsResponse = {
  "room_id": "!GOZSIcCXzL-pF-QPwtEeUL2iYzdiskkGvHrFngmX2yc",
  "activity": {
    "days": 30,
    "from": "2026-11-24",
    "to": "2026-11-24",
    "first_day": "2026-11-24",
    "messages": 6,
    "words": 23,
    "characters": 152,
    "busiest": {
      "date": "2026-11-24",
      "messages": 6,
      "words": 23
    },
    "daily": [
      {
        "date": "2026-11-24",
        "messages": 6,
        "words": 23
      }
    ]
  },
  "people": {
    "counted_since": "2026-11-24T11:37:52.540673Z",
    "people_count": 2,
    "messages": 6,
    "words": 23,
    "characters": 152,
    "top": [
      {
        "user_id": "@alice1:localhost",
        "display_name": "alice1",
        "in_room": true,
        "rank": 1,
        "messages": 4,
        "words": 13,
        "share": 0.6666666666666666,
        "last_message": "2026-11-24T11:37:53.057583Z"
      },
      {
        "user_id": "@bob2:localhost",
        "display_name": "bob2",
        "in_room": true,
        "rank": 2,
        "messages": 2,
        "words": 10,
        "share": 0.3333333333333333,
        "last_message": "2026-11-24T11:37:52.589875Z"
      }
    ],
    "me": null
  }
}
