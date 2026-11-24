// A real response of GET /api/reminders, saved by the end-to-end test TestAPIContract. tsc checks that RemindersResponse fits it,
// and the test that the API still answers in its shape. Don't edit it by hand: run the test with
// SIIKABOT_E2E_UPDATE_FIXTURES=1 to save it again.
import type { RemindersResponse } from '../types'

export const reminders: RemindersResponse = {
  "reminders": [
    {
      "id": 1,
      "remind_time": "2026-11-24T12:37:52Z",
      "room_id": "!GOZSIcCXzL-pF-QPwtEeUL2iYzdiskkGvHrFngmX2yc",
      "room_name": "Room 3980cdd5",
      "message": "reminder-TestAPIContract-68616096"
    }
  ]
}
