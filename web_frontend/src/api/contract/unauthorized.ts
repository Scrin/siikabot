// A real response of GET /api/auth/me without a session, saved by the end-to-end test TestAPIContract. tsc checks that AuthErrorResponse fits it,
// and the test that the API still answers in its shape. Don't edit it by hand: run the test with
// SIIKABOT_E2E_UPDATE_FIXTURES=1 to save it again.
import type { AuthErrorResponse } from '../types'

export const unauthorized: AuthErrorResponse = {
  "error": "Missing or invalid Authorization header"
}
