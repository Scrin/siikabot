package fakenet

import (
	"encoding/json"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"
)

// The hosts of the AI gateway: the Unified API the bot asks the model through, and the REST API whose logs it
// polls
const (
	GatewayHost       = "gateway.ai.cloudflare.com"
	CloudflareAPIHost = "api.cloudflare.com"
)

// DefaultAnswer is what the model answers when its room has no script
const DefaultAnswer = "This is the end-to-end test model."

// ModelRequest is a request the bot made to the model
type ModelRequest struct {
	Model     string            `json:"model"`
	Messages  []Message         `json:"messages"`
	Tools     []json.RawMessage `json:"tools"`
	MaxTokens int               `json:"max_tokens"`
	// ToolChoice is "none" when the model mustn't call tools, as past the tool limit
	ToolChoice json.RawMessage `json:"tool_choice"`
	// Metadata is what the bot sent in cf-aig-metadata: the room, the sender and the iteration
	Metadata map[string]string `json:"-"`
}

// Message is a message in a request to the model
type Message struct {
	Role       string          `json:"role"`
	Content    json.RawMessage `json:"content"`
	ToolCalls  []ToolCall      `json:"tool_calls,omitempty"`
	ToolCallID string          `json:"tool_call_id,omitempty"`
}

// Text is a message's text, without any images in it
func (m Message) Text() string {
	var text string
	if json.Unmarshal(m.Content, &text) == nil {
		return text
	}
	var parts []struct {
		Type string `json:"type"`
		Text string `json:"text"`
	}
	_ = json.Unmarshal(m.Content, &parts)
	var texts []string
	for _, part := range parts {
		if part.Type == "text" {
			texts = append(texts, part.Text)
		}
	}
	return strings.Join(texts, "\n")
}

// ToolCall is a call to a tool in a message
type ToolCall struct {
	ID       string `json:"id"`
	Type     string `json:"type"`
	Function struct {
		Name      string `json:"name"`
		Arguments string `json:"arguments"`
	} `json:"function"`
}

// Text is all the text the request shows the model: every message's, the system prompt's included
func (r ModelRequest) Text() string {
	var texts []string
	for _, m := range r.Messages {
		texts = append(texts, m.Text())
	}
	return strings.Join(texts, "\n")
}

// LastUserText is the text of the request's last user message, the turn's own
func (r ModelRequest) LastUserText() string {
	for i := len(r.Messages) - 1; i >= 0; i-- {
		if r.Messages[i].Role == "user" {
			return r.Messages[i].Text()
		}
	}
	return ""
}

// ToolsAllowed reports whether the request lets the model call tools
func (r ModelRequest) ToolsAllowed() bool {
	return len(r.Tools) > 0 && strings.Trim(string(r.ToolChoice), `"`) != "none"
}

// TurnToolResults are the results of the tool calls in the request's own turn, after its last user message, by tool
// call ID. The history before it can hold earlier turns' results.
func (r ModelRequest) TurnToolResults() map[string]string {
	results := map[string]string{}
	for _, m := range r.Messages {
		switch m.Role {
		case "user":
			results = map[string]string{}
		case "tool":
			results[m.ToolCallID] = m.Text()
		}
	}
	return results
}

// ImageCount is how many images the request shows the model
func (r ModelRequest) ImageCount() int {
	count := 0
	for _, m := range r.Messages {
		var parts []struct {
			Type string `json:"type"`
		}
		if json.Unmarshal(m.Content, &parts) == nil {
			for _, part := range parts {
				if part.Type == "image_url" {
					count++
				}
			}
		}
	}
	return count
}

// ToolResults are the results of the tool calls in the request, by tool call ID
func (r ModelRequest) ToolResults() map[string]string {
	results := map[string]string{}
	for _, m := range r.Messages {
		if m.Role == "tool" {
			results[m.ToolCallID] = m.Text()
		}
	}
	return results
}

// Reply is what the model answers: text, tool calls, or both, and why it stopped
type Reply struct {
	Text         string
	ToolCalls    []ToolCall
	FinishReason string
	// Delay holds the answer back, for a model slower than the bot waits for
	Delay time.Duration
	// Hold holds the answer back until it's closed, for a test to do something while the turn waits
	Hold <-chan struct{}
	// Status answers with an HTTP error instead, when it isn't 0
	Status int
}

// Answer is a reply with text
func Answer(text string) Reply {
	return Reply{Text: text}
}

// CallTool is a reply that calls a tool
func CallTool(id, name, arguments string) Reply {
	call := ToolCall{ID: id, Type: "function"}
	call.Function.Name, call.Function.Arguments = name, arguments
	return Reply{ToolCalls: []ToolCall{call}, FinishReason: "tool_calls"}
}

// Script decides the model's reply to a request
type Script func(req ModelRequest) Reply

// Model stands in for the AI gateway. Requests are kept by the room in their metadata, and each room can have
// a script of its own, so tests running at the same time in rooms of their own never see each other's.
type Model struct {
	mu       sync.Mutex
	scripts  map[string]Script
	requests map[string][]ModelRequest
}

func newModel() *Model {
	return &Model{scripts: map[string]Script{}, requests: map[string][]ModelRequest{}}
}

// Script makes a room's requests answered by script until t ends
func (m *Model) Script(t testing.TB, roomID string, script Script) {
	t.Helper()
	m.mu.Lock()
	defer m.mu.Unlock()
	m.scripts[roomID] = script
	t.Cleanup(func() {
		m.mu.Lock()
		defer m.mu.Unlock()
		delete(m.scripts, roomID)
	})
}

// Requests returns the requests the bot made to the model for a room
func (m *Model) Requests(roomID string) []ModelRequest {
	m.mu.Lock()
	defer m.mu.Unlock()
	return append([]ModelRequest(nil), m.requests[roomID]...)
}

// Saw reports whether any request for a room showed the model text
func (m *Model) Saw(roomID, text string) bool {
	for _, req := range m.Requests(roomID) {
		if strings.Contains(req.Text(), text) {
			return true
		}
	}
	return false
}

func (m *Model) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Host == CloudflareAPIHost || strings.Contains(r.URL.Path, "/logs") {
		// The log poller's page of gateway logs: none
		writeJSON(w, map[string]any{"success": true, "errors": []any{}, "result": []any{}})
		return
	}
	if !strings.HasSuffix(r.URL.Path, "/compat/chat/completions") {
		http.Error(w, "fakenet: the model doesn't serve "+r.URL.Path, http.StatusNotFound)
		return
	}

	var req ModelRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "fakenet: "+err.Error(), http.StatusBadRequest)
		return
	}
	_ = json.Unmarshal([]byte(r.Header.Get("cf-aig-metadata")), &req.Metadata)
	room := req.Metadata["room_id"]

	m.mu.Lock()
	m.requests[room] = append(m.requests[room], req)
	script := m.scripts[room]
	m.mu.Unlock()

	reply := Answer(DefaultAnswer)
	if script != nil {
		reply = script(req)
	}
	if reply.Hold != nil {
		select {
		case <-reply.Hold:
		case <-r.Context().Done():
			return
		}
	}
	if reply.Delay > 0 {
		select {
		case <-time.After(reply.Delay):
		case <-r.Context().Done():
			return
		}
	}
	if reply.Status != 0 {
		http.Error(w, `{"error":{"message":"fakenet: scripted failure"}}`, reply.Status)
		return
	}

	finish := reply.FinishReason
	if finish == "" {
		finish = "stop"
	}
	message := map[string]any{"role": "assistant", "content": reply.Text}
	if len(reply.ToolCalls) > 0 {
		message["tool_calls"] = reply.ToolCalls
	}
	writeJSON(w, map[string]any{
		"id":      "e2e",
		"object":  "chat.completion",
		"model":   req.Model,
		"created": time.Now().Unix(),
		"choices": []any{map[string]any{"index": 0, "message": message, "finish_reason": finish}},
		"usage":   map[string]any{"prompt_tokens": 100, "completion_tokens": 10, "total_tokens": 110},
	})
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(v)
}
