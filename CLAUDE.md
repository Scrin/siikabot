# SiikaBot - Project Documentation

This document provides context for working with SiikaBot using Claude Code CLI.

## Project Overview

**SiikaBot** is a multi-purpose Matrix (matrix.org) bot written in Go, using PostgreSQL for database and deployed as a Docker image. It's a personal/experimental bot with diverse features for a single user/admin, not intended for public use.

### Key Features
- **AI Chat** - LLM-powered conversations with tool calling (weather, web search, GitHub, news, electricity prices)
- **Network Utilities** - Ping and traceroute commands
- **Reminders** - Time-based reminder system with natural language parsing
- **Alerting** - Alertmanager webhook relayed into Matrix rooms
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

**Non-negotiable.** The chat model may only see room content that was addressed to the bot, plus the one message that such a message explicitly replies to.

- A message is addressed to the bot if it mentions the bot, opens with its name, replies to one of its messages, or is the first message of a thread on one of its messages.
- The replied-to message is a real `m.in_reply_to`, or the thread root for the first message of a thread. A thread fallback never counts, and any nested reply fallback is stripped.
- Nothing else from a room may be passed to the model, persisted in `chat_history` or exposed through an LLM tool, and no tool may read room events.
- Metadata that is not message content (member list, room name, message counts) is allowed.
- Redacted messages must leave the chat history.

The referenced event is resolved once, in `bot/reply.go`, and handed to the chat turn in `chat.Trigger`. The chat package never fetches room events itself.

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

- **`./api/`** - HTTP API endpoints
  - `healthcheck.go` - `/api/healthcheck` endpoint

- **`./config/`** - Environment variable loading and configuration

- **`./logging/`** - Centralized logging setup

- **`./metrics/`** - Prometheus metrics collection

- **`./web_frontend/`** - React SPA for system monitoring
  - Built with Vite, embedded in Go binary via `bot/static.go`

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

What the dashboard shows, and where the data comes from:

| Component | API handler | Data, and what writes it |
|-----------|-------------|--------------------------|
| `MemoriesCard` | `api/memories.go` | `user_memory`, written by the memory tool (`llmtools/memory.go`) |
| `RemindersCard` | `api/reminders.go` | `reminders`, written by `!remind` and the reminder tool (`llmtools/reminder.go`) |
| `RoomsCard`, `AdminRoomsCard` | `api/rooms.go`, `api/admin.go` | `room_members`, kept by the Matrix state store (`matrix/state_store.go`) |
| `ChatUsageCard` | `api/chat_usage.go` | `chat_usage`, written at the end of every chat turn (`commands/chat/turn_stats.go`) |
| `SystemStatusCard` | `api/healthcheck.go`, `api/metrics.go` | Process and connection pool stats |

When a dashboard component or API endpoint is added, add it to this table.

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

#### Frontend Tests
```bash
# Run all frontend tests
cd web_frontend
npm test

# Run tests in watch mode during development
npm test -- --watch

# Run with coverage
npm test -- --coverage
```

**When to run tests:**
- After modifying any Go code → Run `go test ./...`
- After modifying any TypeScript/React code → Run `cd web_frontend && npm test`
- After modifying API types or interfaces → Run both Go and frontend tests
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

All configuration is via environment variables (no config files). See `config/config.go` for required variables.

The exception is the chat parameters (models, context window budget, response and tool limits, web content size). They are global and live in the append-only `chat_config` table, where the latest row is the configuration in effect and older rows are its history. The admin changes them with the `!chat` subcommands, and each change appends a new row. Migration `0019__global-chat-config.sql` seeds the first row.

### Database Migrations

Migrations in `db/migrations/` run automatically on startup, in filename order. Name new migrations with the next four-digit number, two underscores and a kebab-case description, like `0018__remove-grafana.sql`.

A change to what `chat_history` stores empties the table (and `chat_context_anchors`, whose anchors point into it) instead of adding code that handles rows in the old format. The chat context is short-lived anyway, so every room simply starts over once. New columns can then be `NOT NULL`, or covered by CHECK constraints, so the code can rely on every row being complete. Data people save on purpose, like memories, is never dropped this way: give existing rows an explicit value in the migration instead.
