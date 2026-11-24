// A real response of GET /api/metrics, saved by the end-to-end test TestAPIContract. tsc checks that MetricsResponse fits it,
// and the test that the API still answers in its shape. Don't edit it by hand: run the test with
// SIIKABOT_E2E_UPDATE_FIXTURES=1 to save it again.
import type { MetricsResponse } from '../types'

export const metrics: MetricsResponse = {
  "memory": {
    "resident_mb": 39.5
  },
  "runtime": {
    "goroutines": 21
  },
  "database": {
    "active_conns": 3,
    "max_conns": 8,
    "idle_conns": 3
  },
  "bot": {
    "events_handled": 96
  }
}
