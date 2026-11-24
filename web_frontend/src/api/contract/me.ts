// A real response of GET /api/auth/me, saved by the end-to-end test TestAPIContract. tsc checks that MeResponse fits it,
// and the test that the API still answers in its shape. Don't edit it by hand: run the test with
// SIIKABOT_E2E_UPDATE_FIXTURES=1 to save it again.
import type { MeResponse } from '../types'

export const me: MeResponse = {
  "user_id": "@alice1:localhost",
  "authorizations": {
    "admin": false
  }
}
