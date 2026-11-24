// A real response of POST /api/auth/challenge, saved by the end-to-end test TestAPIContract. tsc checks that ChallengeResponse fits it,
// and the test that the API still answers in its shape. Don't edit it by hand: run the test with
// SIIKABOT_E2E_UPDATE_FIXTURES=1 to save it again.
import type { ChallengeResponse } from '../types'

export const challenge: ChallengeResponse = {
  "challenge": "Hhfo10B3Xti9sFh_26mFMgAAAABrBXdPaa59WPg1lXJ8pu5KissUhw",
  "poll_secret": "nZCXN7nNRUo3pEMZ5Jzeqzf7grNMhdNyQ1MS6de0a58",
  "expires_at": "2026-11-24T11:38:55Z"
}
