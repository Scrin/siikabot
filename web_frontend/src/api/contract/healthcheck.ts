// A real response of GET /api/healthcheck, saved by the end-to-end test TestAPIContract. tsc checks that HealthCheckResponse fits it,
// and the test that the API still answers in its shape. Don't edit it by hand: run the test with
// SIIKABOT_E2E_UPDATE_FIXTURES=1 to save it again.
import type { HealthCheckResponse } from '../types'

export const healthcheck: HealthCheckResponse = {
  "status": "ok",
  "uptime": "6s"
}
