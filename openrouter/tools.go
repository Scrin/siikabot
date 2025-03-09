package openrouter

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/Scrin/siikabot/metrics"
	"github.com/rs/zerolog/log"
)

// ToolDefinition represents a tool that can be used by the chat model
type ToolDefinition struct {
	Type     string         `json:"type"`
	Function FunctionSchema `json:"function"`
	Handler  ToolHandler    `json:"-"`
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

// HandleToolCallsIndividually processes multiple tool calls and returns individual responses
func (r *ToolRegistry) HandleToolCallsIndividually(ctx context.Context, toolCalls []ToolCall) ([]ToolResponse, error) {
	if len(toolCalls) == 0 {
		return []ToolResponse{}, nil
	}

	var responses []ToolResponse
	for _, call := range toolCalls {
		if call.Type != "function" {
			continue
		}

		handler, exists := r.handlers[call.Function.Name]
		if !exists {
			log.Warn().Ctx(ctx).
				Str("tool", call.Function.Name).
				Msg("Unknown tool called")
			metrics.RecordToolCall(call.Function.Name, false)
			responses = append(responses, ToolResponse{
				ToolCallID: call.ID,
				Response:   fmt.Sprintf("Unknown tool: %s", call.Function.Name),
			})
			continue
		}

		response, err := handler(ctx, call.Function.Arguments)
		if err != nil {
			log.Error().Ctx(ctx).Err(err).
				Str("tool", call.Function.Name).
				Str("arguments", call.Function.Arguments).
				Msg("Tool call failed")
			metrics.RecordToolCall(call.Function.Name, false)
			responses = append(responses, ToolResponse{
				ToolCallID: call.ID,
				Response:   fmt.Sprintf("Error executing %s: %s", call.Function.Name, err.Error()),
			})
		} else {
			log.Debug().Ctx(ctx).
				Str("tool", call.Function.Name).
				Str("arguments", call.Function.Arguments).
				Int("response_length", len(response)).
				Msg("Tool call succeeded")
			metrics.RecordToolCall(call.Function.Name, true)
			responses = append(responses, ToolResponse{
				ToolCallID: call.ID,
				Response:   response,
			})
		}
	}

	return responses, nil
}

// HandleToolCalls processes multiple tool calls and returns a combined response
func (r *ToolRegistry) HandleToolCalls(ctx context.Context, toolCalls []ToolCall) (string, error) {
	if len(toolCalls) == 0 {
		return "", nil
	}

	var responses []string
	for _, call := range toolCalls {
		if call.Type != "function" {
			continue
		}

		handler, exists := r.handlers[call.Function.Name]
		if !exists {
			log.Warn().Ctx(ctx).
				Str("tool", call.Function.Name).
				Msg("Unknown tool called")
			metrics.RecordToolCall(call.Function.Name, false)
			responses = append(responses, fmt.Sprintf("Unknown tool: %s", call.Function.Name))
			continue
		}

		response, err := handler(ctx, call.Function.Arguments)
		if err != nil {
			log.Error().Ctx(ctx).Err(err).
				Str("tool", call.Function.Name).
				Str("arguments", call.Function.Arguments).
				Msg("Tool call failed")
			metrics.RecordToolCall(call.Function.Name, false)
			responses = append(responses, fmt.Sprintf("Error executing %s: %s", call.Function.Name, err.Error()))
		} else {
			log.Debug().Ctx(ctx).
				Str("tool", call.Function.Name).
				Str("arguments", call.Function.Arguments).
				Int("response_length", len(response)).
				Msg("Tool call succeeded")
			metrics.RecordToolCall(call.Function.Name, true)
			responses = append(responses, response)
		}
	}

	return strings.Join(responses, "\n\n"), nil
}
