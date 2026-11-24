// A real response of GET /api/admin/webhooks, saved by the end-to-end test TestAPIContract. tsc checks that WebhooksResponse fits it,
// and the test that the API still answers in its shape. Don't edit it by hand: run the test with
// SIIKABOT_E2E_UPDATE_FIXTURES=1 to save it again.
import type { WebhooksResponse } from '../types'

export const webhooks: WebhooksResponse = {
  "webhooks": [
    {
      "id": "ZoQepxOwAig7NaEYmmtJfQ",
      "name": "Hook b53b19ee renamed",
      "room_id": "!GOZSIcCXzL-pF-QPwtEeUL2iYzdiskkGvHrFngmX2yc",
      "room_name": "Room 3980cdd5",
      "bot_in_room": true,
      "url": "https://siikabot.example.org/hooks/message/ZoQepxOwAig7NaEYmmtJfQ",
      "created_at": "2026-11-24T11:37:54Z",
      "last_used_at": "2026-11-24T11:37:54Z"
    }
  ]
}
