// A real response of PATCH /api/admin/webhooks/:id, saved by the end-to-end test TestAPIContract. tsc checks that SavedWebhookResponse fits it,
// and the test that the API still answers in its shape. Don't edit it by hand: run the test with
// SIIKABOT_E2E_UPDATE_FIXTURES=1 to save it again.
import type { SavedWebhookResponse } from '../types'

export const webhookUpdated: SavedWebhookResponse = {
  "webhook": {
    "id": "ZoQepxOwAig7NaEYmmtJfQ",
    "name": "Hook b53b19ee renamed",
    "room_id": "!GOZSIcCXzL-pF-QPwtEeUL2iYzdiskkGvHrFngmX2yc",
    "room_name": "Room 3980cdd5",
    "bot_in_room": true,
    "url": "https://siikabot.example.org/hooks/message/ZoQepxOwAig7NaEYmmtJfQ",
    "created_at": "2026-11-24T11:37:54Z",
    "last_used_at": "2026-11-24T11:37:54Z"
  }
}
