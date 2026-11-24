// A real response of GET /api/admin/chat-usage, saved by the end-to-end test TestAPIContract. tsc checks that ChatUsageResponse fits it,
// and the test that the API still answers in its shape. Don't edit it by hand: run the test with
// SIIKABOT_E2E_UPDATE_FIXTURES=1 to save it again.
import type { ChatUsageResponse } from '../types'

export const chatUsage: ChatUsageResponse = {
  "since": "2026-11-17T11:37:55.612947844Z",
  "days": 7,
  "entries": [
    {
      "room_id": "!GOZSIcCXzL-pF-QPwtEeUL2iYzdiskkGvHrFngmX2yc",
      "room_name": "Room 3980cdd5",
      "model": "openrouter/deepseek/deepseek-v4.1-flash",
      "turns": 2,
      "prompt_tokens": 300,
      "completion_tokens": 30,
      "cached_prompt_tokens": 0,
      "cache_hit_rate": 0,
      "tool_iterations": 2,
      "failures": 1,
      "silent": 0,
      "failures_by_outcome": {
        "request_failed": 1
      },
      "reflections": 1,
      "failed_reflections": 0
    },
    {
      "room_id": "!-QFGcqietL4neTWsyGyTlF5Va0ykQrwYrARGpRymXCg",
      "room_name": "",
      "model": "openrouter/deepseek/deepseek-v4.1-flash",
      "turns": 1,
      "prompt_tokens": 200,
      "completion_tokens": 20,
      "cached_prompt_tokens": 0,
      "cache_hit_rate": 0,
      "tool_iterations": 2,
      "failures": 0,
      "silent": 0,
      "failures_by_outcome": {},
      "reflections": 0,
      "failed_reflections": 0
    }
  ],
  "totals": {
    "room_id": "",
    "room_name": "",
    "model": "all",
    "turns": 3,
    "prompt_tokens": 500,
    "completion_tokens": 50,
    "cached_prompt_tokens": 0,
    "cache_hit_rate": 0,
    "tool_iterations": 4,
    "failures": 1,
    "silent": 0,
    "failures_by_outcome": {
      "request_failed": 1
    },
    "reflections": 1,
    "failed_reflections": 0
  }
}
