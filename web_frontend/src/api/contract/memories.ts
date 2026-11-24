// A real response of GET /api/memories, saved by the end-to-end test TestAPIContract. tsc checks that MemoriesResponse fits it,
// and the test that the API still answers in its shape. Don't edit it by hand: run the test with
// SIIKABOT_E2E_UPDATE_FIXTURES=1 to save it again.
import type { MemoriesResponse } from '../types'

export const memories: MemoriesResponse = {
  "memories": [
    {
      "id": 2,
      "memory": "dm-memory-TestAPIContract-862efe16",
      "room_id": null,
      "created_at": "2026-11-24T11:37:54Z"
    },
    {
      "id": 1,
      "memory": "memory-TestAPIContract-6b2f5016",
      "room_id": "!GOZSIcCXzL-pF-QPwtEeUL2iYzdiskkGvHrFngmX2yc",
      "room_name": "Room 3980cdd5",
      "created_at": "2026-11-24T11:37:52Z"
    }
  ]
}
