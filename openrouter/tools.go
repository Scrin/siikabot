package openrouter

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/Scrin/siikabot/metrics"
	"github.com/rs/zerolog/log"
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

// GetToolDefinitions returns all registered tool definitions
func (r *ToolRegistry) GetToolDefinitions() []ToolDefinition {
	definitions := make([]ToolDefinition, 0, len(r.definitions))
	for _, def := range r.definitions {
		definitions = append(definitions, def)
	}
	return definitions
}

// HandleToolCallsIndividually processes multiple tool calls in parallel and returns individual responses
func (r *ToolRegistry) HandleToolCallsIndividually(ctx context.Context, toolCalls []ToolCall) ([]ToolResponse, error) {
	if len(toolCalls) == 0 {
		return []ToolResponse{}, nil
	}

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

			handler, exists := r.handlers[currentCall.Function.Name]
			if !exists {
				log.Warn().Ctx(ctx).
					Str("tool", currentCall.Function.Name).
					Msg("Unknown tool called")
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
				log.Error().Ctx(ctx).Err(err).
					Str("tool", currentCall.Function.Name).
					Str("arguments", currentCall.Function.Arguments).
					Float64("execution_time_sec", executionTime).
					Msg("Tool call failed")
				metrics.RecordToolCall(currentCall.Function.Name, false)
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
