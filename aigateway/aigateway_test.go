package aigateway

import (
	"encoding/json"
	"testing"

	"github.com/Scrin/siikabot/config"
)

// TestChatRequestBodyShape verifies the compat endpoint's flat OpenAI body: everything at the top
// level, with no Cloudflare "input" wrapper around it.
//
// The body is produced by marshalling ChatRequest directly, so the struct tags are the whole
// contract. A stray tag would silently send a shape the endpoint does not understand.
func TestChatRequestBodyShape(t *testing.T) {
	maxTokens := 512
	data, err := json.Marshal(ChatRequest{
		Model: "openai/gpt-4o-mini",
		Messages: []Message{
			{Role: "system", Content: "You are a bot."},
			{Role: "user", Content: "Hello"},
		},
		Tools: []ToolDefinition{{
			Type:     "function",
			Function: FunctionSchema{Name: "get_weather", Parameters: json.RawMessage(`{"type":"object"}`)},
		}},
		MaxTokens: &maxTokens,
		Metadata:  map[string]string{"room_id": "!room:example.org"},
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
	for _, key := range []string{"messages", "tools", "max_tokens"} {
		if _, ok := got[key]; !ok {
			t.Errorf("expected %q at the top level", key)
		}
	}

	// The old /ai/run nesting must not reappear; the compat endpoint would ignore the whole request
	if _, ok := got["input"]; ok {
		t.Error("the body should not be nested under input")
	}

	// Metadata travels as a header. In the body it would be an unrecognised field carrying a room
	// id and a user id into the provider's request.
	if _, ok := got["Metadata"]; ok {
		t.Error("metadata should not be serialised into the request body")
	}
	if _, ok := got["metadata"]; ok {
		t.Error("metadata should not be serialised into the request body")
	}

	messages, ok := got["messages"].([]any)
	if !ok || len(messages) != 2 {
		t.Fatalf("expected 2 messages, got %v", got["messages"])
	}
	if first := messages[0].(map[string]any); first["role"] != "system" {
		t.Errorf("expected the system message to be preserved, got %v", first["role"])
	}
}

// TestChatRequestOmitsEmptyOptionalFields verifies tools and max_tokens are omitted when unset,
// since max_tokens is deliberately not defaulted
func TestChatRequestOmitsEmptyOptionalFields(t *testing.T) {
	data, err := json.Marshal(ChatRequest{
		Model:    "google/gemini-3-flash",
		Messages: []Message{{Role: "user", Content: "Hello"}},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var got map[string]any
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if _, ok := got["tools"]; ok {
		t.Error("tools should be omitted when empty")
	}
	if _, ok := got["max_tokens"]; ok {
		t.Error("max_tokens should be omitted when unset")
	}
}

// TestChatCompletionsURL verifies the inference URL targets the Unified API, which is the only
// endpoint that honours the OTel trace headers
func TestChatCompletionsURL(t *testing.T) {
	config.CloudflareAccountID = "acct123"
	config.CloudflareAIGatewayID = "siikabot"

	want := "https://gateway.ai.cloudflare.com/v1/acct123/siikabot/compat/chat/completions"
	if got := chatCompletionsURL(); got != want {
		t.Errorf("chatCompletionsURL() = %q, want %q", got, want)
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

// TestChatCompletionResponseParsing verifies a successful response is read from the top level,
// where the compat endpoint puts it, rather than out of a "result" field
func TestChatCompletionResponseParsing(t *testing.T) {
	body := []byte(`{
		"id": "chatcmpl-1",
		"object": "chat.completion",
		"model": "gpt-4o-mini-2024-07-18",
		"choices": [{"index":0,"message":{"role":"assistant","content":"Red"},"finish_reason":"stop"}],
		"usage": {"prompt_tokens":8518,"completion_tokens":1,"total_tokens":8519,
		          "prompt_tokens_details":{"cached_tokens":8192}}
	}`)

	var resp chatCompletionResponse
	if err := json.Unmarshal(body, &resp); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if resp.failed() {
		t.Error("a successful response should not report a failure")
	}
	if len(resp.Choices) != 1 {
		t.Fatalf("expected 1 choice, got %d", len(resp.Choices))
	}
	if content, ok := resp.Choices[0].Message.Content.(string); !ok || content != "Red" {
		t.Errorf("expected content %q, got %v", "Red", resp.Choices[0].Message.Content)
	}
	if resp.Model != "gpt-4o-mini-2024-07-18" {
		t.Errorf("expected the resolved model id, got %q", resp.Model)
	}
	if resp.Usage == nil || resp.Usage.PromptTokens != 8518 {
		t.Fatalf("expected usage to be parsed, got %+v", resp.Usage)
	}
	// The measurement the whole prompt-caching effort is judged by, so it has to survive the move
	if got := resp.Usage.CachedPromptTokens(); got != 8192 {
		t.Errorf("expected 8192 cached tokens, got %d", got)
	}
}

// TestChatCompletionResponseToolCall verifies a tool call parses from the top-level shape
func TestChatCompletionResponseToolCall(t *testing.T) {
	body := []byte(`{
		"choices": [{"index":0,"message":{"role":"assistant","content":null,"tool_calls":[
			{"id":"call_1","type":"function","function":{"name":"get_weather","arguments":"{\"location\":\"Oulu\"}"}}
		]},"finish_reason":"tool_calls"}]
	}`)

	var resp chatCompletionResponse
	if err := json.Unmarshal(body, &resp); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	choice := resp.Choices[0]
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

// TestProviderErrorShape verifies OpenAI's error object is recognised.
//
// This is the shape a provider failure arrives in, and it can come back with HTTP 200 when the
// gateway itself succeeded. Missing it would mean returning an empty completion as a success.
func TestProviderErrorShape(t *testing.T) {
	body := []byte(`{"error":{"message":"The model does not exist","type":"invalid_request_error","code":"model_not_found"}}`)

	var resp chatCompletionResponse
	if err := json.Unmarshal(body, &resp); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !resp.failed() {
		t.Fatal("an error object should be reported as a failure")
	}
	if _, message := firstError(resp.apiErrors()); message != "The model does not exist" {
		t.Errorf("unexpected message: %q", message)
	}
	if got := resp.errorCode(); got != "model_not_found" {
		t.Errorf("expected the provider string code, got %q", got)
	}
	// Classification has to reach the same verdict from the normalised form
	if kind := classifyResponseError(200, resp.apiErrors()); kind != ErrorKindProviderError {
		t.Errorf("expected provider_error, got %q", kind)
	}
}

// TestGatewayErrorShape verifies Cloudflare's own error array is still recognised, since the
// gateway can fail before the provider is ever reached
func TestGatewayErrorShape(t *testing.T) {
	body := []byte(`{"errors":[{"message":"Model not found: openai/nope","code":7003}]}`)

	var resp chatCompletionResponse
	if err := json.Unmarshal(body, &resp); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !resp.failed() {
		t.Fatal("an errors array should be reported as a failure")
	}
	if _, message := firstError(resp.apiErrors()); message != "Model not found: openai/nope" {
		t.Errorf("unexpected message: %q", message)
	}
	if got := resp.errorCode(); got != "7003" {
		t.Errorf("expected the gateway numeric code, got %q", got)
	}
}

// TestSuccessfulResponseIsNotMistakenForAFailure guards the absent success flag: the compat
// endpoint has none, so a response is a failure only if it says so
func TestSuccessfulResponseIsNotMistakenForAFailure(t *testing.T) {
	var resp chatCompletionResponse
	if err := json.Unmarshal([]byte(`{"choices":[{"message":{"role":"assistant","content":"hi"}}]}`), &resp); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.failed() {
		t.Error("a response with no error fields should not be a failure")
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
		{Type: "image_url", ImageURL: &ImageURL{URL: dataURI, Detail: "low"}},
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
	if imageURL["detail"] != "low" {
		t.Errorf("expected the detail level to be sent, got %v", imageURL["detail"])
	}
}

// TestImageDetailOmittedWhenEmpty verifies an unset detail level is left out of the request rather
// than sent as an empty string, so the provider applies its own default
func TestImageDetailOmittedWhenEmpty(t *testing.T) {
	data, err := json.Marshal(ContentPart{
		Type:     "image_url",
		ImageURL: &ImageURL{URL: "data:image/png;base64,iVBORw0KGgo="},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var got struct {
		ImageURL map[string]any `json:"image_url"`
	}
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if _, ok := got.ImageURL["detail"]; ok {
		t.Error("detail should be omitted when empty")
	}
}
