// A real response of POST /api/admin/rooms/:roomId/reflect, saved by the end-to-end test TestAPIContract. tsc checks that ReflectResponse fits it,
// and the test that the API still answers in its shape. Don't edit it by hand: run the test with
// SIIKABOT_E2E_UPDATE_FIXTURES=1 to save it again.
import type { ReflectResponse } from '../types'

export const reflect: ReflectResponse = {
  "outcome": "changed",
  "skipped": false,
  "failed": false,
  "conversation_count": 1,
  "shown_conversation_count": 1,
  "added": 1,
  "updated": 0,
  "deleted": 0,
  "rejected": 0,
  "trace_id": ""
}
