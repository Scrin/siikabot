package aigateway

import (
	"context"
	"encoding/json"
	"fmt"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/Scrin/siikabot/metrics"
	"github.com/Scrin/siikabot/tracing"
	"github.com/rs/zerolog/log"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
)

// ToolDefinition represents a tool that can be used by the chat model
type ToolDefinition struct {
	Type             string         `json:"type"`
	Function         FunctionSchema `json:"function"`
	Handler          ToolHandler    `json:"-"`
	ValidityDuration time.Duration  `json:"-"`
}

// FunctionSchema defines the schema for a function tool
type FunctionSchema struct {
	Name        string          `json:"name"`
	Description string          `json:"description"`
	Parameters  json.RawMessage `json:"parameters"`
}

// ToolCall represents a call to a tool from the chat model
type ToolCall struct {
	ID       string       `json:"id"`
	Type     string       `json:"type"`
	Function ToolFunction `json:"function"`
}

// ToolFunction represents the function part of a tool call
type ToolFunction struct {
	Name      string `json:"name"`
	Arguments string `json:"arguments"`
}

// ToolHandler is a function that handles a specific tool call
type ToolHandler func(ctx context.Context, arguments string) (string, error)

// ToolRegistry stores all available tools and their handlers
type ToolRegistry struct {
	definitions map[string]ToolDefinition
	handlers    map[string]ToolHandler
}

// ToolResponse represents a response from a tool call
type ToolResponse struct {
	ToolCallID string
	Response   string
}

// NewToolRegistry creates a new tool registry
func NewToolRegistry() *ToolRegistry {
	return &ToolRegistry{
		definitions: make(map[string]ToolDefinition),
		handlers:    make(map[string]ToolHandler),
	}
}

// RegisterTool registers a tool with the registry
func (r *ToolRegistry) RegisterTool(definition ToolDefinition) {
	metrics.InitializeTool(definition.Function.Name)
	r.definitions[definition.Function.Name] = definition
	r.handlers[definition.Function.Name] = definition.Handler
}

// GetToolDefinitions returns all registered tool definitions, ordered by name.
//
// The ordering is not cosmetic. Tool definitions are the largest single part of every prompt and are
// sent on every call, so they sit inside the prefix providers cache on. Go randomises map iteration,
// which meant an identical set of tools serialised differently on every request and the prefix never
// repeated — the cache could only ever hit within a single turn, where one slice is reused across
// iterations. Sorting makes the prefix stable across turns, which is the whole point of keeping the
// volatile parts of the prompt at the end.
func (r *ToolRegistry) GetToolDefinitions() []ToolDefinition {
	definitions := make([]ToolDefinition, 0, len(r.definitions))
	for _, def := range r.definitions {
		definitions = append(definitions, def)
	}
	slices.SortFunc(definitions, func(a, b ToolDefinition) int {
		return strings.Compare(a.Function.Name, b.Function.Name)
	})
	return definitions
}

// HandleToolCallsIndividually processes multiple tool calls in parallel and returns individual responses
func (r *ToolRegistry) HandleToolCallsIndividually(ctx context.Context, toolCalls []ToolCall) ([]ToolResponse, error) {
	if len(toolCalls) == 0 {
		return []ToolResponse{}, nil
	}

	names := make([]string, 0, len(toolCalls))
	for _, call := range toolCalls {
		names = append(names, call.Function.Name)
	}

	ctx, batchSpan := tracer.Start(ctx, "tool.batch", trace.WithAttributes(
		attribute.Int("siikabot.tool.count", len(toolCalls)),
		attribute.StringSlice("siikabot.tool.names", names),
	))
	defer batchSpan.End()

	var (
		responses []ToolResponse
		wg        sync.WaitGroup
		mu        sync.Mutex
	)

	// Pre-allocate the responses slice to avoid reallocations
	responses = make([]ToolResponse, 0, len(toolCalls))

	for _, call := range toolCalls {
		if call.Type != "function" {
			continue
		}

		// Create local copies of variables for the goroutine
		currentCall := call

		wg.Add(1)
		go func() {
			defer wg.Done()

			ctx, span := tracer.Start(ctx, "tool."+currentCall.Function.Name, trace.WithAttributes(
				attribute.String("siikabot.tool.name", currentCall.Function.Name),
				attribute.String("siikabot.tool.call_id", currentCall.ID),
			))
			defer span.End()

			// A panicking tool handler used to take the whole process down with it. Recovering keeps
			// the bot alive, but the model requires a response for every tool call it asked for —
			// omitting one makes the *next* request fail with a confusing error about an unanswered
			// call — so a failure response is substituted in its place.
			//
			// Registered before the handler runs, and the handler runs outside the mutex, so this
			// can safely take the lock while unwinding.
			defer func() {
				if recovered := tracing.Recover(ctx, span); recovered != nil {
					metrics.RecordToolCall(currentCall.Function.Name, false)

					mu.Lock()
					responses = append(responses, ToolResponse{
						ToolCallID: currentCall.ID,
						Response:   fmt.Sprintf("Tool %s failed unexpectedly", currentCall.Function.Name),
					})
					mu.Unlock()
				}
			}()

			handler, exists := r.handlers[currentCall.Function.Name]
			if !exists {
				log.Warn().Ctx(ctx).
					Str("tool", currentCall.Function.Name).
					Msg("Unknown tool called")
				span.SetStatus(codes.Error, "unknown tool")
				metrics.RecordToolCall(currentCall.Function.Name, false)

				mu.Lock()
				responses = append(responses, ToolResponse{
					ToolCallID: currentCall.ID,
					Response:   fmt.Sprintf("Unknown tool: %s", currentCall.Function.Name),
				})
				mu.Unlock()
				return
			}

			startTime := time.Now()
			response, err := handler(ctx, currentCall.Function.Arguments)
			executionTime := time.Since(startTime).Seconds()

			mu.Lock()
			defer mu.Unlock()

			if err != nil {
				errorKind := ClassifyToolError(err)
				span.RecordError(err)
				span.SetStatus(codes.Error, err.Error())
				span.SetAttributes(attribute.String("siikabot.tool.error_kind", string(errorKind)))
				log.Error().Ctx(ctx).Err(err).
					Str("tool", currentCall.Function.Name).
					Str("arguments", currentCall.Function.Arguments).
					Str("error_kind", string(errorKind)).
					Float64("execution_time_sec", executionTime).
					Msg("Tool call failed")
				metrics.RecordToolCall(currentCall.Function.Name, false)
				metrics.RecordToolError(currentCall.Function.Name, string(errorKind))
				responses = append(responses, ToolResponse{
					ToolCallID: currentCall.ID,
					Response:   fmt.Sprintf("Error executing %s: %s", currentCall.Function.Name, err.Error()),
				})
			} else {
				log.Debug().Ctx(ctx).
					Str("tool", currentCall.Function.Name).
					Str("arguments", currentCall.Function.Arguments).
					Int("response_length", len(response)).
					Float64("execution_time_sec", executionTime).
					Msg("Tool call succeeded")
				span.SetAttributes(attribute.Int("siikabot.tool.response_bytes", len(response)))
				metrics.RecordToolCall(currentCall.Function.Name, true)
				metrics.RecordToolLatency(currentCall.Function.Name, executionTime)
				responses = append(responses, ToolResponse{
					ToolCallID: currentCall.ID,
					Response:   response,
				})
			}
		}()
	}

	// Wait for all goroutines to complete
	wg.Wait()

	return responses, nil
}
