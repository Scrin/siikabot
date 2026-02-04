package openrouter

import (
	"context"
	"encoding/json"
	"testing"
	"time"
)

func TestNewToolRegistry(t *testing.T) {
	registry := NewToolRegistry()

	if registry == nil {
		t.Fatal("NewToolRegistry returned nil")
	}

	if registry.definitions == nil {
		t.Error("definitions map should be initialized")
	}

	if registry.handlers == nil {
		t.Error("handlers map should be initialized")
	}

	if len(registry.definitions) != 0 {
		t.Errorf("definitions should be empty, got %d entries", len(registry.definitions))
	}

	if len(registry.handlers) != 0 {
		t.Errorf("handlers should be empty, got %d entries", len(registry.handlers))
	}
}

func TestGetToolDefinitionsEmpty(t *testing.T) {
	registry := NewToolRegistry()

	definitions := registry.GetToolDefinitions()

	if definitions == nil {
		t.Fatal("GetToolDefinitions returned nil")
	}

	if len(definitions) != 0 {
		t.Errorf("expected empty slice, got %d definitions", len(definitions))
	}
}

func TestRegisterToolAndGetDefinitions(t *testing.T) {
	registry := NewToolRegistry()

	testHandler := func(ctx context.Context, arguments string) (string, error) {
		return "test response", nil
	}

	testTool := ToolDefinition{
		Type: "function",
		Function: FunctionSchema{
			Name:        "test_tool",
			Description: "A test tool",
			Parameters:  json.RawMessage(`{"type": "object"}`),
		},
		Handler:          testHandler,
		ValidityDuration: 5 * time.Minute,
	}

	registry.RegisterTool(testTool)

	definitions := registry.GetToolDefinitions()

	if len(definitions) != 1 {
		t.Fatalf("expected 1 definition, got %d", len(definitions))
	}

	def := definitions[0]
	if def.Function.Name != "test_tool" {
		t.Errorf("expected tool name 'test_tool', got %q", def.Function.Name)
	}

	if def.Function.Description != "A test tool" {
		t.Errorf("expected description 'A test tool', got %q", def.Function.Description)
	}

	if def.Type != "function" {
		t.Errorf("expected type 'function', got %q", def.Type)
	}
}

func TestRegisterMultipleTools(t *testing.T) {
	registry := NewToolRegistry()

	dummyHandler := func(ctx context.Context, arguments string) (string, error) {
		return "", nil
	}

	tools := []ToolDefinition{
		{
			Type:     "function",
			Function: FunctionSchema{Name: "tool_a", Description: "Tool A"},
			Handler:  dummyHandler,
		},
		{
			Type:     "function",
			Function: FunctionSchema{Name: "tool_b", Description: "Tool B"},
			Handler:  dummyHandler,
		},
		{
			Type:     "function",
			Function: FunctionSchema{Name: "tool_c", Description: "Tool C"},
			Handler:  dummyHandler,
		},
	}

	for _, tool := range tools {
		registry.RegisterTool(tool)
	}

	definitions := registry.GetToolDefinitions()

	if len(definitions) != 3 {
		t.Fatalf("expected 3 definitions, got %d", len(definitions))
	}

	// Check all tools are present (order not guaranteed from map)
	names := make(map[string]bool)
	for _, def := range definitions {
		names[def.Function.Name] = true
	}

	expectedNames := []string{"tool_a", "tool_b", "tool_c"}
	for _, name := range expectedNames {
		if !names[name] {
			t.Errorf("expected tool %q to be present", name)
		}
	}
}

func TestGetToolDefinitionsReturnsNewSlice(t *testing.T) {
	registry := NewToolRegistry()

	dummyHandler := func(ctx context.Context, arguments string) (string, error) {
		return "", nil
	}

	registry.RegisterTool(ToolDefinition{
		Type:     "function",
		Function: FunctionSchema{Name: "original_tool"},
		Handler:  dummyHandler,
	})

	definitions1 := registry.GetToolDefinitions()
	definitions2 := registry.GetToolDefinitions()

	// Modify first slice
	if len(definitions1) > 0 {
		definitions1[0].Function.Name = "modified"
	}

	// Second slice should be unaffected
	if len(definitions2) > 0 && definitions2[0].Function.Name == "modified" {
		t.Error("GetToolDefinitions should return independent slices")
	}
}

func TestHandleToolCallsIndividuallyEmpty(t *testing.T) {
	registry := NewToolRegistry()

	responses, err := registry.HandleToolCallsIndividually(context.Background(), []ToolCall{})

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if responses == nil {
		t.Fatal("responses should not be nil")
	}

	if len(responses) != 0 {
		t.Errorf("expected empty responses, got %d", len(responses))
	}
}

func TestHandleToolCallsIndividuallyUnknownTool(t *testing.T) {
	registry := NewToolRegistry()

	toolCalls := []ToolCall{
		{
			ID:   "call_1",
			Type: "function",
			Function: ToolFunction{
				Name:      "nonexistent_tool",
				Arguments: "{}",
			},
		},
	}

	responses, err := registry.HandleToolCallsIndividually(context.Background(), toolCalls)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(responses) != 1 {
		t.Fatalf("expected 1 response, got %d", len(responses))
	}

	if responses[0].ToolCallID != "call_1" {
		t.Errorf("expected tool call ID 'call_1', got %q", responses[0].ToolCallID)
	}

	if responses[0].Response == "" {
		t.Error("expected error message in response")
	}
}

func TestHandleToolCallsIndividuallySuccess(t *testing.T) {
	registry := NewToolRegistry()

	testHandler := func(ctx context.Context, arguments string) (string, error) {
		return "success response", nil
	}

	registry.RegisterTool(ToolDefinition{
		Type:     "function",
		Function: FunctionSchema{Name: "test_tool"},
		Handler:  testHandler,
	})

	toolCalls := []ToolCall{
		{
			ID:   "call_1",
			Type: "function",
			Function: ToolFunction{
				Name:      "test_tool",
				Arguments: "{}",
			},
		},
	}

	responses, err := registry.HandleToolCallsIndividually(context.Background(), toolCalls)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(responses) != 1 {
		t.Fatalf("expected 1 response, got %d", len(responses))
	}

	if responses[0].ToolCallID != "call_1" {
		t.Errorf("expected tool call ID 'call_1', got %q", responses[0].ToolCallID)
	}

	if responses[0].Response != "success response" {
		t.Errorf("expected 'success response', got %q", responses[0].Response)
	}
}

func TestHandleToolCallsIndividuallySkipsNonFunction(t *testing.T) {
	registry := NewToolRegistry()

	toolCalls := []ToolCall{
		{
			ID:   "call_1",
			Type: "other_type", // Not "function"
			Function: ToolFunction{
				Name:      "test_tool",
				Arguments: "{}",
			},
		},
	}

	responses, err := registry.HandleToolCallsIndividually(context.Background(), toolCalls)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Non-function types should be skipped
	if len(responses) != 0 {
		t.Errorf("expected 0 responses for non-function type, got %d", len(responses))
	}
}

func TestHandleToolCallsIndividuallyMultipleCalls(t *testing.T) {
	registry := NewToolRegistry()

	handler1 := func(ctx context.Context, arguments string) (string, error) {
		return "response from tool_1", nil
	}
	handler2 := func(ctx context.Context, arguments string) (string, error) {
		return "response from tool_2", nil
	}

	registry.RegisterTool(ToolDefinition{
		Type:     "function",
		Function: FunctionSchema{Name: "tool_1"},
		Handler:  handler1,
	})
	registry.RegisterTool(ToolDefinition{
		Type:     "function",
		Function: FunctionSchema{Name: "tool_2"},
		Handler:  handler2,
	})

	toolCalls := []ToolCall{
		{ID: "call_1", Type: "function", Function: ToolFunction{Name: "tool_1", Arguments: "{}"}},
		{ID: "call_2", Type: "function", Function: ToolFunction{Name: "tool_2", Arguments: "{}"}},
	}

	responses, err := registry.HandleToolCallsIndividually(context.Background(), toolCalls)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(responses) != 2 {
		t.Fatalf("expected 2 responses, got %d", len(responses))
	}

	// Check both responses are present (order not guaranteed due to parallel execution)
	responseMap := make(map[string]string)
	for _, r := range responses {
		responseMap[r.ToolCallID] = r.Response
	}

	if responseMap["call_1"] != "response from tool_1" {
		t.Errorf("expected 'response from tool_1', got %q", responseMap["call_1"])
	}
	if responseMap["call_2"] != "response from tool_2" {
		t.Errorf("expected 'response from tool_2', got %q", responseMap["call_2"])
	}
}
