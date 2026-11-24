# SiikaBot - Project Documentation

This document provides context for working with SiikaBot using Claude Code CLI.

## Project Overview

**SiikaBot** is a multi-purpose Matrix (matrix.org) bot written in Go, using PostgreSQL for database and deployed as a Docker image. It's a personal/experimental bot with diverse features for a single user/admin, not intended for public use.

### Key Features
- **AI Chat** - LLM-powered conversations with tool calling (FMI weather and environment data, web search, GitHub, news, electricity prices, room message statistics, the bot's own source code). Besides the memories people ask it to keep about themselves, the bot keeps memories of each group room on its own, formed by thinking back over its conversations there once the room has gone quiet
- **Network Utilities** - Ping and traceroute commands
- **Reminders** - Time-based reminders, set with `!remind` or in the chat. The chat also lists and deletes them, and posts the command that sets a deleted reminder again. People can delete theirs on the web dashboard too
- **Alerting** - Alertmanager webhook relayed into Matrix rooms. Each alert is headed by its `summary` annotation, or its `description` when it has no summary, with the description under the heading, its labels, and a link to its `generatorURL`. All of it shows as text, never as markup
- **Message Webhooks** - Webhooks the admin sets up on the web dashboard, which post the Markdown message a request brings into a room, under the webhook's name
- **Federation Info** - Matrix server federation details
- **Web Dashboard** - React-based system health monitoring UI

## Technology Stack

### Backend (Go)
- **Matrix**: `maunium.net/go/mautrix`
- **Database**: PostgreSQL with `jackc/pgx/v5`
- **Logging**: `rs/zerolog`
- **HTTP**: Standard library `net/http`
- **Monitoring**: Prometheus client
- **LLM**: Cloudflare AI Gateway (REST API) integration

### Frontend (React + TypeScript)
- **Framework**: Vite
- **Styling**: Tailwind CSS
- **Data Fetching**: TanStack React Query
- **3D Graphics**: Three.js

### Deployment
- Docker multi-stage builds (AMD64 + ARM64)
- GitHub Actions CI/CD
- GitHub Container Registry (ghcr.io)

## Architecture

The bot behavior is mostly **reactive**, responding to certain messages or events (like `!commands`). In addition, there are some **asynchronous features** such as sending reminders, and externally triggered behavior through webhooks.

In general, each command or webhook is considered a **self-contained feature** and should be located in its own package.

### Message Flow
1. Matrix client receives event via continuous sync loop
2. `bot/bot.go` routes message to appropriate command handler
3. Command handlers execute in goroutines (non-blocking)
4. Responses sent back through Matrix client
5. Relevant data persisted to PostgreSQL

### Chat Privacy Invariant

**Non-negotiable.** The chat model may only see room content that was addressed to the bot, plus the messages that such a message explicitly refers to: the one it replies to, and up to three it links to.

- A message is addressed to the bot if it mentions the bot, opens with its name, replies to one of its messages, or is the first message of a thread on one of its messages. Messages the bot relayed from a webhook count as its messages here. An image's caption counts like a text message. An edit counts only when it newly mentions the bot.
- The replied-to message is a real `m.in_reply_to`, or the thread root for the first message of a thread. A thread fallback never counts, and any nested reply fallback is stripped.
- A linked message is a `matrix.to` link or `matrix:` URI to an event in the same room. Links inside a referenced message are not followed.
- A referenced message is used only if the person referring to it could see it themselves. In a room that shows its history only from when people joined or were invited, anything older than their membership is left out (`matrix.VisibleTo`).
- Nothing else from a room may be passed to the model, persisted in `chat_history` or exposed through an LLM tool, apart from reminders and the bot's memories of rooms as described below, and no tool may read room events.
- Metadata that is not message content (member list, room name, message counts) is allowed, for the room the turn happens in only. A tool that looks such things up takes the room from the chat turn, never from its arguments: `find_room_members` gets the member list from the turn, and `get_message_stats` reads the counts for the turn's room.
- A reminder's text may reach the model even when it was set with `!remind`, but only in the room it was set in. The chat sees and acts on only the reminders that the author of the latest message set in the room the turn happens in, and `manage_reminders` takes that room and that person from the turn, never from its arguments. A reminder set in one room can never be listed, read or deleted from a conversation in another room, a direct chat included, not even by the person who set it. They manage it there or on the dashboard.
- The bot keeps memories of each group room (`room_memory`). It forms them by thinking back over the room's chat history once the room has been quiet for `room_memory_delay` (`commands/chat/room_memory.go`): the people's messages and its answers, never tool results. They reach the model only in that room's turns, never in another room or a direct chat, and direct chats have none. A memory is about the room as a whole, never one person. Since a pass reads a conversation only after the delay, a message redacted before then never reaches it. A memory isn't deleted when a message it drew on is redacted later: the message was up for everyone in the room to see, and the bot remembers it as they do. No chat tool changes room memories: later passes revise them, and the admin deletes them on the dashboard. Every step of a pass is traced and logged, since what it decides shows only much later: its traces may hold the conversations, as chat turns' traces do, but its logs name them only by their event IDs.
- Redacted messages must leave the chat history, and a message its sender edits is stored as edited. When an edit is redacted, the message is stored as it then reads: as the sender's latest edit that stands, or as sent, which the bot asks the homeserver for.
- The image of a message the model may see is stored with the turn that showed it (`chat_images`), and is shown again while that turn is in the context window. It leaves the history with the turn, and at once when its own message is redacted.

The referenced events are resolved once, in `bot/reply.go`, and handed to the chat turn in `chat.Trigger`. The chat package never fetches room events itself.

## Code Structure

### Directory Layout

- **`./bot/`** - General bot logic, routing commands to their appropriate handlers. Entry point for handling Matrix events.

- **`./commands/`** - All command implementations (one package per command), called from the bot package
  - Each command has its own subdirectory (e.g., `ping/`, `chat/`, `remind/`)
  - Reference implementation: `commands/ping/ping.go`
  - Commands are registered in `bot/bot.go` in the switch statement

- **`./db/`** - All database operations. Queries should be organized by feature.
  - `postgres.go` - Connection pool and initialization
  - `migrations.go` - Schema management (runs on startup)
  - `queries_*.go` - Feature-specific database queries
  - Reference implementation: `db/queries_remind.go`

- **`./db/migrations/`** - PostgreSQL migration files. Executed during startup only if not run yet.

- **`./llmtools/`** - Tools available to the LLM AI for function calling
  - Each tool is a self-contained package
  - Reference implementation: `llmtools/electricity_prices.go`

- **`./matrix/`** - All Matrix homeserver communication is abstracted here
  - `client.go` - Client initialization and event handling
  - `send.go`, `servers.go`, `media.go` - Protocol operations
  - State stores for crypto, sync, and state management

- **`./aigateway/`** - Cloudflare AI Gateway communication for LLM integration
  - `aigateway.go` - HTTP client. Inference goes to the Unified API (`gateway.ai.cloudflare.com/.../compat/chat/completions`), which is the only endpoint that honours the OTel trace headers; the REST API on `api.cloudflare.com` is used for the management API only
  - `logs.go` - Background poller that records cost/token/latency metrics from gateway logs
  - `tools.go` - Tool definition framework for function calling

- **`./fmi/`** - The Finnish Meteorological Institute's open data: observations, forecasts, warnings, air quality, sea, lightning and more, used by the `get_fmi_data` LLM tool (`llmtools/fmi.go`)
  - Most data comes from FMI's timeseries API (JSON), warnings from its CAP feed, and only the wave buoys from WFS
  - FMI's open data is CC BY 4.0, so anything that presents it credits FMI

- **`./webfetch/`** - Reads web pages for the `get_web_content` LLM tool (`llmtools/web.go`): fetches a URL, extracts the main content with readability, reads PDFs, and judges how the fetch went (`judge.go`), so a failure reaches the model as an explanation rather than an error
  - `guard.go` refuses connections to addresses that aren't public, checked when connecting so redirects are covered
  - `provenance.go` holds the provenance rule: once a chat turn holds a result from a tool that isn't `Trusted`, it may fetch only URLs that appeared in the room's messages or in tool results, and look up (DNS, whois) only names that did. What the model wrote never counts, even when a tool repeats it: its answers, its tool calls' arguments, and the bot's own messages the room quotes. A URL that matches a seen one is fetched as it was seen, not as the model wrote it. The chat turn passes the rule to the tools through the context
  - `GuardedDialer` in `guard.go` is for any other tool that connects to hosts a stranger can name, as whois follows referrals

- **`./sourcecode/`** - The bot's own source code, for the `get_own_source_code` LLM tool (`llmtools/source_code.go`): downloads the default branch of the repository in `SIIKABOT_SOURCE_URL` from GitHub as one archive, keeps it in memory for 15 minutes, and searches and reads its files
  - Only a repository on GitHub can be read. Elsewhere the chat still links to the repository, but the tool tells the model it can't look anything up there

- **`./webhooks/`** - The rules for the webhooks the admin sets up on the web dashboard: what their names and tokens may be, how a token is checked, and how the message a request brings is read
  - The endpoint, `POST /hooks/message/:id` with the webhook's bearer token, is `bot/hook_message.go`, and managing webhooks is `api/webhooks.go`
  - A token is stored only as its hash, so it can be replaced but never shown again
  - A relayed message carries its webhook in its content (`matrix/webhook.go`), and the chat tells the model that a quoted one is in the webhook's words, not the bot's

- **`./api/`** - HTTP API endpoints
  - `healthcheck.go` - `/api/healthcheck` endpoint

- **`./config/`** - Environment variable loading and configuration

- **`./logging/`** - Centralized logging setup

- **`./metrics/`** - Prometheus metrics collection

- **`./web_frontend/`** - React SPA for system monitoring
  - Built with Vite, embedded in Go binary via `bot/static.go`
  - `e2e/` - Its tests in a browser (Playwright), which the Go end-to-end tests run (see *Testing*)
  - `src/api/contract/` - A real response of each endpoint it calls, saved by the end-to-end tests and typed with `types.ts`, so `tsc` checks the two agree

- **`./testutil/`** - Helpers for tests only, never imported by the bot itself
  - `pgtest` - A PostgreSQL database of a test's own (see *Testing*)
  - `dbtest` - Points the `db` package at such a database, with the migrations run
  - `synapsetest` - A throwaway, non-federated Synapse for the end-to-end tests, with every user it registers exempt from its rate limits
  - `fakenet` - The end-to-end tests' stand-in for the network, with a scripted model on the AI gateway's hosts
  - `leftovers` - Removes the temporary directories that test processes ending with a panic leave behind
  - `knownbug` - Skips the tests for known open bugs, unless `SIIKABOT_KNOWN_BUGS` is set

- **`./testhooks/`** - Where a build with the `e2e` tag reaches into the bot: it sends the bot's outbound HTTP to the tests' stand-in for the network, lets the fetch guard reach it, turns tracing off, and has the bot look for rooms due a pass as often as `SIIKABOT_E2E_REFLECTION_INTERVAL` says when a test sets it. In every other build each hook is a pass-through, so production can't be switched into it. A client that reaches the internet wraps its transport with `testhooks.Transport`

- **`./e2e/`** - The end-to-end tests (see *Testing*)

- **`./scripts/`** - `install-synapse.sh` installs the Synapse the end-to-end tests run against

## Coding Conventions

### Go Code - Logging Rules

**ALWAYS follow these zerolog logging practices:**

1. **Always use zerolog** for all logging
2. **Always include the current context** using `.Ctx(ctx)`
3. **All relevant variables should be added as log fields**
4. **Log fields should always use snake_case** naming
5. **Never use dynamic or changing text in the message** - use fields instead

#### Example

```go
log.Error().Ctx(ctx).Err(err).Str("recipient", recipient).Int("attempt_count", attemptCount).Msg("Failed to send a message")
```

**Good:**
```go
log.Debug().
    Ctx(ctx).
    Str("room_id", roomID).
    Str("target", target).
    Int("count", count).
    Bool("ipv6", isV6).
    Msg("Executing ping command")
```

**Bad:**
```go
log.Debug().Msg(fmt.Sprintf("Executing ping command for %s in room %s with count %d", target, roomID, count))
```

### Go Code - Error Handling

1. **Log errors where they occur** with full context using log fields
2. **Return errors up the call stack** for the caller to decide what to do
3. **Wrap errors** with additional context using `fmt.Errorf("context: %w", err)`
4. **Send user-facing messages** to Matrix when appropriate

#### Example Pattern

```go
err := someOperation()
if err != nil {
    log.Error().Ctx(ctx).Err(err).Str("room_id", roomID).Msg("Failed to perform operation")
    matrix.SendMessage(roomID, "Operation failed: " + err.Error())
    return err
}
```

### Go Code - Database Query Patterns

1. **One file per feature** - Group related queries in `db/queries_<feature>.go`
2. **Use struct types** for query results with db tags:
   ```go
   type Reminder struct {
       ID         int64     `db:"id"`
       RemindTime time.Time `db:"remind_time"`
       UserID     string    `db:"user_id"`
   }
   ```
3. **Always pass context** as the first parameter to all database functions
4. **Use pgx.CollectRows** for collecting query results into structs
5. **Log query failures** with relevant parameters

#### Example Pattern

```go
func GetReminders(ctx context.Context) ([]Reminder, error) {
    rows, err := pool.Query(ctx, "SELECT id, remind_time, user_id, room_id, message FROM reminders")
    if err != nil {
        log.Error().Ctx(ctx).Err(err).Msg("Failed to query reminders")
        return nil, err
    }
    return pgx.CollectRows(rows, pgx.RowToStructByName[Reminder])
}
```

### Go Code - Command Structure Pattern

Commands should follow this pattern:

1. **Package per command** in `./commands/<commandname>/`
2. **Handle function** as the entry point: `Handle(ctx context.Context, roomID, msg string)`
3. **Parse arguments** from the message string
4. **Execute command logic** (may involve database, external APIs, etc.)
5. **Send typing indicator** for long-running operations: `matrix.SendTyping(ctx, roomID, true, duration)`
6. **Send result** back to room: `matrix.SendMessage(roomID, response)`
7. **Log all operations** with appropriate context

#### Example Command Structure

```go
package mycommand

import (
    "context"
    "strings"
    "github.com/Scrin/siikabot/matrix"
    "github.com/rs/zerolog/log"
)

// Handle handles the !mycommand command
func Handle(ctx context.Context, roomID, msg string) {
    split := strings.Split(msg, " ")
    if len(split) < 2 {
        return
    }

    arg := split[1]

    log.Debug().Ctx(ctx).Str("room_id", roomID).Str("arg", arg).Msg("Executing mycommand")

    // Command logic here
    result := doSomething(arg)

    matrix.SendMessage(roomID, result)
}
```

#### Registering Commands

Add commands to the switch statement in `bot/bot.go`:

```go
case "!mycommand":
    go mycommand.Handle(ctx, evt.RoomID.String(), msg)
```

### Go Code - LLM Tool Pattern

LLM tools allow the AI chatbot to call functions. Follow this pattern:

1. **One file per tool** in `./llmtools/`
2. **Export a ToolDefinition** variable with the OpenAI function-calling schema
3. **Implement a handler function** that takes `(ctx context.Context, arguments string) (string, error)`
4. **Parse JSON arguments** into a struct
5. **Fetch data** from external APIs or database
6. **Format response** as markdown string for the LLM
7. **Use caching** where appropriate (with mutex for thread safety)
8. **Set `Trusted: true`** only if the tool's results can't hold text a stranger wrote: they come from the bot, the room, or a source the bot relies on, such as FMI. A tool that returns web pages, search results, issues or other text from the internet stays untrusted, and labels that text with `webfetch.UntrustedLabel`
9. **Check the provenance rule** in a tool that reaches a host or name the model chooses, since that's a way to send the room's content out: `webfetch.ProvenanceFrom(ctx).AllowsName` before a lookup, as `dns_lookup` and `whois_lookup` do. Fetching goes through `webfetch.Fetch`, which checks it itself
10. **Test it against fakes,** never the internet. `useFakeInternet` (`llmtools/fakeinternet_test.go`) answers the shared client's requests with a handler per host, and `useFakeDNS` and `useFakeWhois` stand in for DNS and whois servers. A tool whose package has a client of its own, as FMI's has, is tested end to end with `e.net.Handle`. The package's tests run against PostgreSQL (`dbtest.Main`)

#### Example Tool Structure

```go
package llmtools

import (
    "context"
    "encoding/json"
    "github.com/Scrin/siikabot/aigateway"
    "github.com/rs/zerolog/log"
)

var MyToolDefinition = aigateway.ToolDefinition{
    Type: "function",
    Function: aigateway.FunctionSchema{
        Name:        "my_tool_name",
        Description: "What this tool does",
        Parameters: json.RawMessage(`{
            "type": "object",
            "properties": {
                "param": {
                    "type": "string",
                    "description": "Parameter description"
                }
            },
            "required": ["param"]
        }`),
    },
    Handler: handleMyToolCall,
}

func handleMyToolCall(ctx context.Context, arguments string) (string, error) {
    var args struct {
        Param string `json:"param"`
    }

    log.Debug().Ctx(ctx).Str("arguments", arguments).Msg("Received my_tool call")

    if err := json.Unmarshal([]byte(arguments), &args); err != nil {
        log.Error().Ctx(ctx).Err(err).Str("arguments", arguments).Msg("Failed to parse tool arguments")
        return "", fmt.Errorf("failed to parse arguments: %w", err)
    }

    // Tool logic here
    result := fetchData(ctx, args.Param)

    return formatResult(result), nil
}
```

#### Registering Tools

Tools are registered in `Init` in `commands/chat/chat.go`, and every registered tool is offered to the model in every room.

### Go Code - Metrics Pattern

When adding new features or commands, add appropriate Prometheus metrics for observability:

1. **Define metrics in `./metrics/`** - Group related metrics by feature in dedicated files (e.g., `metrics/matrix.go`, `metrics/remind.go`)
2. **Use `makeCollector()`** to register all collectors
3. **Use the `metricPrefix`** constant (`siikabot_`) for all metric names
4. **Export `Record*` functions** for instrumentation - callers should never interact with collectors directly
5. **Use low-cardinality labels** - Avoid labels that can produce unbounded values (user IDs, room IDs, raw URL paths)
6. **Standard metric types**:
   - Counter for monotonically increasing values (requests, errors, events)
   - Gauge for values that go up and down (queue depth, active count)
   - Histogram for measuring distributions (latency, durations) using `defaultBuckets` or custom buckets

#### Example Metric File

```go
package metrics

import "github.com/prometheus/client_golang/prometheus"

var myFeatureRequests = makeCollector(prometheus.NewCounterVec(prometheus.CounterOpts{
    Name: metricPrefix + "my_feature_requests_count",
    Help: "Total number of my feature requests",
}, []string{"status"}))

func RecordMyFeatureRequest(success bool) {
    status := "success"
    if !success {
        status = "failure"
    }
    myFeatureRequests.WithLabelValues(status).Inc()
}
```

### React/TypeScript Code

1. **Component structure** - Functional components with TypeScript
2. **Data fetching** - Use React Query hooks in `src/api/queries.ts`
3. **API client** - Centralized in `src/api/client.ts`
4. **Styling** - Tailwind CSS utility classes
5. **Type safety** - Define types in `src/api/types.ts`

#### Example Component Pattern

```typescript
import { useSystemStatus } from './api/queries'
import { LoadingSpinner } from './components/LoadingSpinner'
import { ErrorMessage } from './components/ErrorMessage'

export function StatusView() {
  const { data, isLoading, error } = useSystemStatus()

  if (isLoading) return <LoadingSpinner />
  if (error) return <ErrorMessage error={error} />

  return (
    <div className="p-4">
      <h1 className="text-2xl font-bold">{data.status}</h1>
    </div>
  )
}
```

## Common Patterns

### Async Operations in Commands

Commands are executed in goroutines (spawned with `go commandHandler()`), so they don't block the main event loop. Long-running operations should:

1. Send typing indicator: `matrix.SendTyping(ctx, roomID, true, 30*time.Second)`
2. Perform the operation
3. Stop typing: `matrix.SendTyping(ctx, roomID, false, 0)`
4. Send result

### Context Propagation

Always pass `context.Context` as the first parameter to functions. The context carries:
- Request tracing information
- Logging fields (added via `logging.ContextWithStr()`)
- Cancellation signals

### Matrix Message Sending

Use the `matrix` package for all Matrix operations:
- `matrix.SendMessage(roomID, text)` - Send text message
- `matrix.SendTyping(ctx, roomID, isTyping, timeout)` - Send typing indicator
- `matrix.GetClient()` - Get underlying mautrix client for advanced operations

## Development Workflow

### Keeping the Web Frontend in Step

The web dashboard shows much of what the bot stores and does, so a bot change is not finished until the dashboard matches it. **Whenever a change affects data the dashboard shows, check the web side and update it in the same change.** That covers the rules behind the data as well as its fields: what gets stored, how it is scoped, what counts as a failure, and who can see or delete what. For example, when memories were scoped to rooms, the dashboard went on listing them as if every memory applied everywhere.

This applies to plans as much as to code. A plan for a bot change says what changes on the web side, even when the answer is nothing.

For each part of the dashboard a change touches, update:
1. The API response in `api/`
2. The types in `web_frontend/src/api/types.ts`
3. The component, including its wording
4. The tests on both sides
5. The contract: run `TestAPIContract` with `SIIKABOT_E2E_UPDATE_FIXTURES=1` to save the responses again, and bring `types.ts` in line until `npx tsc -b` passes

What the dashboard shows, and where the data comes from:

| Component | API handler | Data, and what writes it |
|-----------|-------------|--------------------------|
| `MemoriesCard` | `api/memories.go` | `user_memory`, written by the memory tool (`llmtools/memory.go`) |
| `RoomMemoriesCard` | `api/room_memories.go` | `room_memory` and `room_memory_progress`, written by the passes that think back over a room once it has gone quiet (`commands/chat/room_memory.go`), or at once when the admin asks for one on the card (`POST /api/admin/rooms/:roomId/reflect`). The card shows how each group room stands with the passes, how its latest pass went, and the traces of that pass and of the passes that wrote each memory. The memories are deleted by the admin on the dashboard and by later passes, but not by redactions. Admin only |
| `RemindersCard` | `api/reminders.go` | `reminders`, written by `!remind` and the `manage_reminders` tool (`llmtools/reminder.go`). People delete their reminders on the dashboard, in any room, and through `manage_reminders`, only in the room they set them in, which posts the command that sets the reminder again. Deleting one also stops its timer (`commands/remind/schedule.go`) |
| `RoomsCard` | `api/rooms.go`, `api/admin.go` | `room_members`, kept by the Matrix state store (`matrix/state_store.go`) from sync, and rebuilt from the homeserver at startup. The card lists the rooms someone shares with the bot, and every room for an admin |
| `RoomStats` (in `RoomsCard`) | `api/message_stats.go` | `message_stats` and `room_daily_stats`, written for every text message from anyone but the bot, edits aside (`bot/bot.go`), and read by the `get_message_stats` tool too. Only someone who has joined a room sees its stats, like its members |
| `MessageActivityCard` | `api/message_stats.go` | `room_daily_stats` across every room, and the chat turns in `chat_usage` |
| `ChatUsageCard` | `api/chat_usage.go` | `chat_usage`, written at the end of every chat turn (`commands/chat/turn_stats.go`) and every pass over a room (`commands/chat/room_memory.go`), told apart by `kind`. The tokens count both, while the turn and failure counts are of turns only |
| `WebhooksCard` | `api/webhooks.go` | `webhooks`, written only by the admin on the dashboard, and read by the message hook (`bot/hook_message.go`), which also sets `last_used_at`. Admin only. A token is stored only as its hash and is never shown again |
| `ChatConfigCard` | `api/chat_config.go` | `chat_config`, written by `!chat set` and `!chat config revert` (`commands/chat/chat.go`), and read through the settings declared in `commands/chat/settings.go`. Read-only: the settings change only in a room |
| `SystemStatusCard` | `api/healthcheck.go`, `api/metrics.go` | Process and connection pool stats |

When a dashboard component or API endpoint is added, add it to this table, its response to `TestAPIContract` (`e2e/contract_test.go`), and what it shows to the browser tests (`web_frontend/e2e/dashboard.spec.ts`).

The cards are laid out by topic in `web_frontend/src/App.tsx`. A new card goes in its topic's `DashboardSection`, wrapped in `DashboardCard`, and takes `adminOnly` if its endpoint is under `/api/admin`: the card is then left out for everyone but an admin.

The chat's system prompt also tells the model what people see on the dashboard (`sourceSection` in `commands/chat/prompt.go`), so a change to that updates the prompt too.

### Testing

**CRITICAL: Always run tests after making code changes**

When modifying code, you MUST run the relevant tests to ensure nothing is broken:

#### Go Backend Tests
```bash
# Run all tests
go test ./...

# Run tests for a specific package
go test ./db
go test ./api
go test ./commands/...

# Run with verbose output
go test -v ./...

# Run with coverage
go test -cover ./...
```

Tests that need PostgreSQL run against a real one, and are never skipped:
- **The server:** `testutil/pgtest` starts a throwaway cluster from the PostgreSQL server binaries installed here. It listens only on a Unix socket in a temporary directory, so test runs never meet each other, and it stops after the tests.
- **CI and other servers:** CI points the tests at a `postgres:18.7` service with `SIIKABOT_TEST_POSTGRES`, a connection string whose user can create databases. Each database's name carries the test process's ID.
- **A database per test:** each test gets a database of its own, copied from a template that has had the migrations run once.
- **Outside `db`:** a package's tests call `dbtest.Use(t)` (`testutil/dbtest`), which points the `db` package at the test's database until the test ends. `db`'s own tests use `useTestDatabase(t)` instead.
- **One at a time:** the `db` package works on one database at a time, so these tests can't call `t.Parallel()`.
- **`TestMain`:** a package with such tests calls `dbtest.Main(m)`, or `pgtest.Run(m)` from a `TestMain` that does more, so the cluster stops after the tests.

```bash
# On a server of your own instead of a throwaway cluster
SIIKABOT_TEST_POSTGRES="postgres://user@localhost:5432/postgres?sslmode=disable" go test ./db
```

Without libolm installed, build with `-tags goolm`.

CI runs the Go tests with `-race`, the end-to-end tests (the browser tests among them), and the frontend's tests and type-check on every pull request, and all of them on every push to master, before `main.yml` builds and publishes the image. A failing test stops the image from being published.

#### End-to-End Tests

The end-to-end tests (`e2e/`) run the bot as it's deployed and talk to it the way people do. Each test run gets its own temporary directories and free ports, so runs never meet each other.
- **The bot:** built with the `e2e` tag and run as a subprocess, with its configuration in the environment.
- **Its surroundings:** a throwaway Synapse with federation off (`testutil/synapsetest`), and a throwaway PostgreSQL.
- **The network:** a stand-in (`testutil/fakenet`). Every outbound request goes there, the model's included, which is scripted per room and records every request it gets.
- **The people:** test users are real Matrix clients registered on that Synapse.

```bash
scripts/install-synapse.sh        # once; it does nothing when Synapse is already installed
go test -tags e2e ./e2e/...
```

The tests check what the model was shown (`e.Model.Requests`, `e.Model.Saw`), what the room got, and what the bot stored (`e.Query`). Writing one:
- **Rooms of its own:** each test has rooms and users of its own, so tests can run in parallel. A test scripts the model only for its own rooms.
- **Unique text:** `marker` makes text no other message has, to look for in what the model saw.
- **Final checks:** `conclude` addresses the bot once more and waits for the answer, so everything sent before has been handled before a test checks what the model saw.
- **Waiting:** a test waits for the bot with `WaitForBot` and `WaitForBotMessage`, never with a fixed sleep, except to check that something doesn't happen. When a message may get more than one answer, such as a command, it waits for the one it wants by its text.
- **Races:** a test of something happening while a turn runs makes the race certain instead of timing it:
  - `heldModel` holds the model's answer until the test releases it;
  - `waitForTheTurn` waits until the turn is waiting for that answer;
  - `settle` waits until the bot has handled everything sent so far.
- **Encryption:** `User.Encrypted` gives a test user end-to-end encryption, and `WithEncryption` creates an encrypted room.
- **Restarts:** `StopBot` and `StartBot` stop and restart the bot, and `RestartBotWith` restarts it with more environment until the test ends. Every test shares it, so a test that stops it doesn't run in parallel, and nor does one that changes the chat settings (`changeSettings`).
- **Other hosts:** `e.net.Handle` serves a host of the internet for a test, as `TestFMITool` does FMI's. Only one test at a time may serve a host.
- **Known bugs:** a test for a known open bug calls `knownbug.Skip` (`testutil/knownbug`, `knownBug` in `e2e`) with the bug's ID, which skips it. Set `SIIKABOT_KNOWN_BUGS=1` to run those tests, unit and end-to-end alike, and see which bugs are still there. The fix removes the call.
- **Logs and coverage:** a failing test logs the end of the bot's log and Synapse's errors. `SIIKABOT_E2E_LOGDIR` keeps both logs whole in a directory, with the browser tests' traces, and `SIIKABOT_E2E_COVERDIR` collects the bot's coverage for `go tool covdata`.

#### The Dashboard in a Browser, and the API's Contract

`TestDashboardInABrowser` (`e2e/browser_test.go`) makes a world of data through the bot, builds the dashboard with Vite and serves it with the API from the bot, and runs Playwright's tests (`web_frontend/e2e/`) against it. They log in through `!auth`, sending the command as the person's Matrix client would, and check what each card shows. The Go test tells them about the world in a file (`SIIKABOT_E2E_WORLD`), so they run only through it:

```bash
cd web_frontend && npm ci          # once; the browser tests need the dashboard's packages
go test -tags e2e ./e2e -run TestDashboardInABrowser
```

- **The browser:** Playwright's own Chromium when `npx playwright install chromium` has put it in place, as in CI, and the system's Chromium otherwise. `SIIKABOT_E2E_CHROMIUM` names another.
- **Known bugs:** a browser test for a known open bug calls `knownBug`, and runs with `SIIKABOT_KNOWN_BUGS=1` like the Go ones.

`TestAPIContract` (`e2e/contract_test.go`) checks the API's contract with the dashboard. A real response of each endpoint the dashboard calls is saved in `web_frontend/src/api/contract/`, as a TypeScript file that gives it its type from `types.ts`:
- **`tsc` checks the types:** a field the type doesn't have, a required one missing, or a value of the wrong type fails `npx tsc -b`, at any depth.
- **The Go test checks the responses:** it fails when a response no longer has its fixture's shape, its fields and the kinds of their values, and says which fields changed.
- **Saving them again:** `SIIKABOT_E2E_UPDATE_FIXTURES=1` saves the fixtures whose shape changed, and only those, so their IDs and times don't churn. Never edit one by hand.

#### Frontend Tests
```bash
# Run all frontend tests
cd web_frontend
npm test

# Run tests in watch mode during development
npm test -- --watch

# Run with coverage
npm test -- --coverage

# Type-check, the API's contract fixtures included; vitest doesn't
npx tsc -b
```

**When to run tests:**
- After modifying any Go code → Run `go test ./...`
- After modifying any TypeScript/React code → Run `cd web_frontend && npm test`
- After modifying API types or interfaces → Run both Go and frontend tests, `TestAPIContract`, and `npx tsc -b`
- Before creating a commit → Always run relevant tests
- After updating dependencies → Run all tests

### Building

```bash
# Build the Go backend
go build -o siikabot

# Build the React frontend
cd web_frontend
npm install
npm run build
```

### Docker Build

```bash
docker build -t siikabot .
```

### Environment Variables

All configuration is via environment variables (no config files). See `config/config.go` for required variables. A required variable that is missing or invalid stops the bot at startup, with an error naming it.

`SIIKABOT_HTTP_ADDR` is where the HTTP server listens, `:8080` unless it's set.

`SIIKABOT_SOURCE_URL` and `SIIKABOT_FRONTEND_URL` are where the bot's source code repository and web dashboard are. The chat model gives them out as links, and reads the bot's own source code from the repository.

The exception is the chat settings (the model, which has to take images as well as text, the context window budget, response and tool limits, how much of a web page one tool call returns, how long a turn and each model call in it may take, with how many attempts a model call gets, and how long a room has to be quiet before the bot thinks back over it). They are global and live in the append-only `chat_config` table, where the latest row is the configuration in effect and older rows are its history. `!chat config` lists them, and the admin changes them with `!chat set <key> <value>`, each change appending a new row. `!chat config history` lists the changes, and `!chat config revert <id>` goes back to an earlier row by appending a copy of it, checked by today's rules. Migration `0019__global-chat-config.sql` seeds the first row.

Each column of `chat_config` is a setting, declared in `commands/chat/settings.go` under the column's name, which is also its key in the commands and the logs. The declaration says what the setting means, the values it takes and how it's shown, and the listing, the help and the checks all come from it. A new column needs its declaration there, which a test checks. The turn hands its configuration to the tools in the context, so a tool never reads one of its own.

### Database Migrations

Migrations in `db/migrations/` run automatically on startup, in filename order. Name new migrations with the next four-digit number, two underscores and a kebab-case description, like `0018__remove-grafana.sql`.

#### Clearing the chat history

**Clear the chat history rather than work around it.** The chat history is short-lived context, not data anyone keeps. When a change would leave stored rows that the new code can't treat exactly like new ones, the change's migration empties the history. Never add code to handle the old rows: no fallbacks for an old format, no aliases for an old tool name, no special cases, and nothing left inconsistent "for now". This isn't limited to schema changes. It covers, for example:
- a new column that existing rows can't fill
- a tool that is renamed, removed or merged into another, or whose arguments or results change, because stored turns hold calls to it and their results
- a change to what a stored row means, or to how a turn is rebuilt from the history

Empty `chat_history` together with every table that hangs off it, in one statement: `TRUNCATE chat_history, chat_references, chat_images, chat_context_anchors;`. PostgreSQL refuses to truncate a table that other tables reference unless they are truncated with it. Every room then starts over once, as if `!chat reset` had been run everywhere. New columns can be `NOT NULL` or covered by CHECK constraints, so the code can rely on every row being complete.

Keep the rows only when they are already right under the change, such as a new nullable column whose NULL is what every existing row means.

This applies to the chat context only. Data people save on purpose, like memories and reminders, is never dropped this way: give existing rows an explicit value in the migration instead. The bot's memories of rooms, and `room_memory_progress`, stay as well. `chat_usage` is accounting rather than context, and stays too.

Never add `RESTART IDENTITY` to the truncate. `room_memory_progress` records how far the bot has thought back over each room as a `chat_history` id, which relies on ids never being reused.
