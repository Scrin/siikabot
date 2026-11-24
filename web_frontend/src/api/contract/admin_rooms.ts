// A real response of GET /api/admin/rooms, saved by the end-to-end test TestAPIContract. tsc checks that RoomsResponse fits it,
// and the test that the API still answers in its shape. Don't edit it by hand: run the test with
// SIIKABOT_E2E_UPDATE_FIXTURES=1 to save it again.
import type { RoomsResponse } from '../types'

export const adminRooms: RoomsResponse = {
  "rooms": [
    {
      "room_id": "!-QFGcqietL4neTWsyGyTlF5Va0ykQrwYrARGpRymXCg"
    },
    {
      "room_id": "!GOZSIcCXzL-pF-QPwtEeUL2iYzdiskkGvHrFngmX2yc",
      "room_name": "Room 3980cdd5"
    },
    {
      "room_id": "!QH4DBP8xkbeXpiCMORWy3rFjQaPuQ3SLKo6kcTjmRZo"
    },
    {
      "room_id": "!l1ISdUCY3YtnXZaScngQAZ5TkatkOI29QvPJ8g3y7jQ"
    },
    {
      "room_id": "!qzMO_BL1cX4xfgdybVgAZNAmawvEa2EXsRSraSAL4pM"
    }
  ]
}
