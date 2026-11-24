// A real response of GET /api/rooms, saved by the end-to-end test TestAPIContract. tsc checks that RoomsResponse fits it,
// and the test that the API still answers in its shape. Don't edit it by hand: run the test with
// SIIKABOT_E2E_UPDATE_FIXTURES=1 to save it again.
import type { RoomsResponse } from '../types'

export const rooms: RoomsResponse = {
  "rooms": [
    {
      "room_id": "!-QFGcqietL4neTWsyGyTlF5Va0ykQrwYrARGpRymXCg"
    },
    {
      "room_id": "!GOZSIcCXzL-pF-QPwtEeUL2iYzdiskkGvHrFngmX2yc",
      "room_name": "Room 3980cdd5"
    }
  ]
}
