// A real response of POST /api/auth/poll, saved by the end-to-end test TestAPIContract. tsc checks that PollResponse fits it,
// and the test that the API still answers in its shape. Don't edit it by hand: run the test with
// SIIKABOT_E2E_UPDATE_FIXTURES=1 to save it again.
import type { PollResponse } from '../types'

export const pollPending: PollResponse = {
  "status": "pending"
}
