package aigateway

import (
	"encoding/json"
	"testing"
)

// TestRunRequestEnvelope verifies the /ai/run body shape: the model at the top level and
// everything else nested under "input".
func TestRunRequestEnvelope(t *testing.T) {
	maxTokens := 512
	data, err := json.Marshal(runRequest{
		Model: "openai/gpt-4o-mini",
		Input: chatInput{
			Messages: []Message{
				{Role: "system", Content: "You are a bot."},
				{Role: "user", Content: "Hello"},
			},
			Tools: []ToolDefinition{{
				Type:     "function",
				Function: FunctionSchema{Name: "get_weather", Parameters: json.RawMessage(`{"type":"object"}`)},
			}},
			MaxTokens: &maxTokens,
		},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var got map[string]any
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if got["model"] != "openai/gpt-4o-mini" {
		t.Errorf("expected model at the top level, got %v", got["model"])
	}

	input, ok := got["input"].(map[string]any)
	if !ok {
		t.Fatalf("expected an input object, got %T", got["input"])
	}

	for _, key := range []string{"messages", "tools", "max_tokens"} {
		if _, ok := input[key]; !ok {
			t.Errorf("expected %q inside input", key)
		}
	}

	// The model must not be duplicated inside input
	if _, ok := input["model"]; ok {
		t.Error("model should not appear inside input")
	}

	messages, ok := input["messages"].([]any)
	if !ok || len(messages) != 2 {
		t.Fatalf("expected 2 messages, got %v", input["messages"])
	}
	if first := messages[0].(map[string]any); first["role"] != "system" {
		t.Errorf("expected the system message to be preserved, got %v", first["role"])
	}
}

// TestRunRequestOmitsEmptyOptionalFields verifies tools and max_tokens are omitted when
// unset, since max_tokens is deliberately not defaulted.
func TestRunRequestOmitsEmptyOptionalFields(t *testing.T) {
	data, err := json.Marshal(runRequest{
		Model: "google/gemini-3-flash",
		Input: chatInput{Messages: []Message{{Role: "user", Content: "Hello"}}},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var got struct {
		Input map[string]any `json:"input"`
	}
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if _, ok := got.Input["tools"]; ok {
		t.Error("tools should be omitted when empty")
	}
	if _, ok := got.Input["max_tokens"]; ok {
		t.Error("max_tokens should be omitted when unset")
	}
}

// TestToolCallMessageMarshalling verifies the assistant-with-tool-calls and tool-result
// messages keep the shape the gateway accepts: an empty string content on the assistant
// message, and tool_call_id on the reply.
func TestToolCallMessageMarshalling(t *testing.T) {
	data, err := json.Marshal([]Message{
		{Role: "assistant", Content: "", ToolCalls: []ToolCall{{
			ID:       "call_1",
			Type:     "function",
			Function: ToolFunction{Name: "get_weather", Arguments: `{"location":"Oulu"}`},
		}}},
		{Role: "tool", Content: "-7 C", ToolCallID: "call_1"},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var got []map[string]any
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if got[0]["content"] != "" {
		t.Errorf("expected an empty string content on the assistant message, got %v", got[0]["content"])
	}
	if _, ok := got[0]["tool_calls"]; !ok {
		t.Error("expected tool_calls on the assistant message")
	}
	if got[1]["tool_call_id"] != "call_1" {
		t.Errorf("expected tool_call_id on the tool message, got %v", got[1]["tool_call_id"])
	}
	if _, ok := got[1]["tool_calls"]; ok {
		t.Error("tool_calls should be omitted on the tool message")
	}
}

// TestRunResponseUnwrapping verifies a successful response is read out of the envelope's
// result field rather than the top level.
func TestRunResponseUnwrapping(t *testing.T) {
	body := []byte(`{
		"result": {
			"id": "chatcmpl-1",
			"object": "chat.completion",
			"model": "gpt-4o-mini-2024-07-18",
			"choices": [{"index":0,"message":{"role":"assistant","content":"Red"},"finish_reason":"stop"}],
			"usage": {"prompt_tokens":8518,"completion_tokens":1,"total_tokens":8519},
			"gatewayMetadata": {"keySource":"Unified"}
		},
		"success": true,
		"errors": [],
		"messages": []
	}`)

	var resp runResponse
	if err := json.Unmarshal(body, &resp); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !resp.Success {
		t.Error("expected success")
	}
	if len(resp.Result.Choices) != 1 {
		t.Fatalf("expected 1 choice, got %d", len(resp.Result.Choices))
	}
	if content, ok := resp.Result.Choices[0].Message.Content.(string); !ok || content != "Red" {
		t.Errorf("expected content %q, got %v", "Red", resp.Result.Choices[0].Message.Content)
	}
	if resp.Result.Usage == nil || resp.Result.Usage.PromptTokens != 8518 {
		t.Errorf("expected usage to be parsed, got %+v", resp.Result.Usage)
	}
}

// TestRunResponseToolCallUnwrapping verifies a tool call survives the envelope.
func TestRunResponseToolCallUnwrapping(t *testing.T) {
	body := []byte(`{
		"result": {
			"choices": [{"index":0,"message":{"role":"assistant","content":null,"tool_calls":[
				{"id":"call_1","type":"function","function":{"name":"get_weather","arguments":"{\"location\":\"Oulu\"}"}}
			]},"finish_reason":"tool_calls"}]
		},
		"success": true,
		"errors": []
	}`)

	var resp runResponse
	if err := json.Unmarshal(body, &resp); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	choice := resp.Result.Choices[0]
	if choice.FinishReason != "tool_calls" {
		t.Errorf("expected finish_reason tool_calls, got %q", choice.FinishReason)
	}
	if len(choice.Message.ToolCalls) != 1 {
		t.Fatalf("expected 1 tool call, got %d", len(choice.Message.ToolCalls))
	}
	if name := choice.Message.ToolCalls[0].Function.Name; name != "get_weather" {
		t.Errorf("expected get_weather, got %q", name)
	}
}

// TestErrorEnvelope verifies the Cloudflare error envelope is parsed, including the
// numeric code. This is the shape real failures arrive in.
func TestErrorEnvelope(t *testing.T) {
	body := []byte(`{"errors":[{"message":"Model not found: openai/nope","code":7003}],"success":false,"result":{},"messages":[]}`)

	var resp runResponse
	if err := json.Unmarshal(body, &resp); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if resp.Success {
		t.Error("expected success to be false")
	}

	code, message := firstError(resp.Errors)
	if code != 7003 {
		t.Errorf("expected code 7003, got %d", code)
	}
	if message != "Model not found: openai/nope" {
		t.Errorf("unexpected message: %q", message)
	}
}

func TestFirstErrorEmpty(t *testing.T) {
	code, message := firstError(nil)
	if code != 0 || message != "" {
		t.Errorf("expected a zero code and empty message, got %d %q", code, message)
	}
}

// TestImageContentPartMarshalling verifies an image message keeps the base64 data URI in
// image_url, which is the only form the gateway accepts.
func TestImageContentPartMarshalling(t *testing.T) {
	const dataURI = "data:image/png;base64,iVBORw0KGgo="

	data, err := json.Marshal(Message{Role: "user", Content: []ContentPart{
		{Type: "text", Text: "What is this?"},
		{Type: "image_url", ImageURL: &struct {
			URL string `json:"url"`
		}{URL: dataURI}},
	}})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var got struct {
		Content []map[string]any `json:"content"`
	}
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(got.Content) != 2 {
		t.Fatalf("expected 2 content parts, got %d", len(got.Content))
	}
	if got.Content[1]["type"] != "image_url" {
		t.Errorf("expected an image_url part, got %v", got.Content[1]["type"])
	}
	imageURL, ok := got.Content[1]["image_url"].(map[string]any)
	if !ok {
		t.Fatalf("expected an image_url object, got %T", got.Content[1]["image_url"])
	}
	if imageURL["url"] != dataURI {
		t.Errorf("expected the data URI to be preserved, got %v", imageURL["url"])
	}
}
