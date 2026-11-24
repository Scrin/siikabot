// A real response of GET /api/admin/chat-config, saved by the end-to-end test TestAPIContract. tsc checks that ChatConfigResponse fits it,
// and the test that the API still answers in its shape. Don't edit it by hand: run the test with
// SIIKABOT_E2E_UPDATE_FIXTURES=1 to save it again.
import type { ChatConfigResponse } from '../types'

export const chatConfig: ChatConfigResponse = {
  "id": 2,
  "created_at": "2026-11-24T11:37:55.22515Z",
  "settings": [
    {
      "key": "model",
      "group": "Model",
      "value": "openrouter/deepseek/deepseek-v4.1-flash",
      "unit": "",
      "summary": "the model that answers every turn",
      "command": "!chat set model \u003cname\u003e"
    },
    {
      "key": "max_response_tokens",
      "group": "Model",
      "value": "3000",
      "unit": "tokens",
      "summary": "the longest answer one model call may write",
      "command": "!chat set max_response_tokens 4096"
    },
    {
      "key": "context_high_tokens",
      "group": "Context window",
      "value": "16384",
      "unit": "tokens",
      "summary": "the most history the window holds before it's trimmed",
      "command": "!chat set context_high_tokens 32k"
    },
    {
      "key": "context_low_tokens",
      "group": "Context window",
      "value": "8192",
      "unit": "tokens",
      "summary": "what a trimmed window is brought under",
      "command": "!chat set context_low_tokens 4k"
    },
    {
      "key": "turn_timeout",
      "group": "Time",
      "value": "3m",
      "unit": "",
      "summary": "the longest one turn may take, tools included",
      "command": "!chat set turn_timeout 10m"
    },
    {
      "key": "model_timeout",
      "group": "Time",
      "value": "45s",
      "unit": "",
      "summary": "the longest one attempt at a model call may take",
      "command": "!chat set model_timeout 30s"
    },
    {
      "key": "model_attempts",
      "group": "Time",
      "value": "3",
      "unit": "",
      "summary": "the attempts a model call gets when it fails for a passing reason",
      "command": "!chat set model_attempts 2"
    },
    {
      "key": "max_tool_iterations",
      "group": "Tools",
      "value": "5",
      "unit": "",
      "summary": "the rounds of tool calls one turn may make",
      "command": "!chat set max_tool_iterations 8"
    },
    {
      "key": "web_part_bytes",
      "group": "Tools",
      "value": "10240",
      "unit": "bytes",
      "summary": "the page text one get_web_content call returns",
      "command": "!chat set web_part_bytes 16k"
    },
    {
      "key": "image_detail",
      "group": "Images",
      "value": "low",
      "unit": "",
      "summary": "how closely images are shown to the model",
      "command": "!chat set image_detail high"
    },
    {
      "key": "room_memory_delay",
      "group": "Room memory",
      "value": "30m",
      "unit": "",
      "summary": "how long a room has to be quiet before the bot thinks back over it",
      "command": "!chat set room_memory_delay 30m"
    }
  ],
  "history": [
    {
      "id": 2,
      "created_at": "2026-11-24T11:37:55.22515Z",
      "changes": [
        {
          "key": "max_response_tokens",
          "from": "2048",
          "to": "3000",
          "unit": "tokens"
        }
      ],
      "first": false
    },
    {
      "id": 1,
      "created_at": "2026-11-24T11:37:49.841812Z",
      "changes": [],
      "first": true
    }
  ]
}
