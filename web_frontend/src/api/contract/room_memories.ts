// A real response of GET /api/admin/room-memories, saved by the end-to-end test TestAPIContract. tsc checks that RoomMemoriesResponse fits it,
// and the test that the API still answers in its shape. Don't edit it by hand: run the test with
// SIIKABOT_E2E_UPDATE_FIXTURES=1 to save it again.
import type { RoomMemoriesResponse } from '../types'

export const roomMemories: RoomMemoriesResponse = {
  "rooms": [
    {
      "room_id": "!GOZSIcCXzL-pF-QPwtEeUL2iYzdiskkGvHrFngmX2yc",
      "room_name": "Room 3980cdd5",
      "last_reflected_at": "2026-11-24T11:37:54Z",
      "status": {
        "state": "up_to_date",
        "due_at": null,
        "latest_activity_at": null,
        "failed_attempts": 0,
        "last_attempt_at": "2026-11-24T11:37:54Z",
        "last_outcome": "changed",
        "last_trace_id": null
      },
      "memories": [
        {
          "id": 1,
          "memory": "The room talks about room-memory-TestAPIContract-8cdec963",
          "created_at": "2026-11-24T11:37:54Z",
          "updated_at": "2026-11-24T11:37:54Z",
          "created_trace_id": null,
          "updated_trace_id": null
        }
      ]
    }
  ]
}
