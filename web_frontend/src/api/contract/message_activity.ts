// A real response of GET /api/admin/message-activity, saved by the end-to-end test TestAPIContract. tsc checks that MessageActivityResponse fits it,
// and the test that the API still answers in its shape. Don't edit it by hand: run the test with
// SIIKABOT_E2E_UPDATE_FIXTURES=1 to save it again.
import type { MessageActivityResponse } from '../types'

export const messageActivity: MessageActivityResponse = {
  "activity": {
    "days": 30,
    "from": "2026-11-24",
    "to": "2026-11-24",
    "first_day": "2026-11-24",
    "messages": 10,
    "words": 35,
    "characters": 333,
    "busiest": {
      "date": "2026-11-24",
      "messages": 10,
      "words": 35
    },
    "daily": [
      {
        "date": "2026-11-24",
        "messages": 10,
        "words": 35
      }
    ]
  },
  "chat_turns": 3,
  "rooms": [
    {
      "room_id": "!GOZSIcCXzL-pF-QPwtEeUL2iYzdiskkGvHrFngmX2yc",
      "room_name": "Room 3980cdd5",
      "messages": 6,
      "words": 23,
      "share": 0.6,
      "chat_turns": 2
    },
    {
      "room_id": "!-QFGcqietL4neTWsyGyTlF5Va0ykQrwYrARGpRymXCg",
      "messages": 2,
      "words": 6,
      "share": 0.2,
      "chat_turns": 1
    },
    {
      "room_id": "!QH4DBP8xkbeXpiCMORWy3rFjQaPuQ3SLKo6kcTjmRZo",
      "messages": 1,
      "words": 2,
      "share": 0.1,
      "chat_turns": 0
    },
    {
      "room_id": "!qzMO_BL1cX4xfgdybVgAZNAmawvEa2EXsRSraSAL4pM",
      "messages": 1,
      "words": 4,
      "share": 0.1,
      "chat_turns": 0
    }
  ]
}
