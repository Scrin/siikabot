package aigateway

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/Scrin/siikabot/config"
)

// roundTripFunc stands in for the network, so a request can be inspected as it was actually built
// rather than as each helper would have built it in isolation
type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

// captureRequest runs sendOnce against a canned response and returns the request it produced
func captureRequest(t *testing.T, ctx context.Context, body string, status int) (*http.Request, *ChatResponse, error) {
	t.Helper()

	config.CloudflareAccountID = "acct123"
	config.CloudflareAIGatewayID = "siikabot"
	config.CloudflareAPIToken = "cf-token"

	var captured *http.Request
	original := httpClient
	httpClient = &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		captured = r.Clone(r.Context())
		if r.Body != nil {
			raw, _ := io.ReadAll(r.Body)
			captured.Body = io.NopCloser(strings.NewReader(string(raw)))
		}
		return &http.Response{
			StatusCode: status,
			Body:       io.NopCloser(strings.NewReader(body)),
			Header:     make(http.Header),
		}, nil
	})}
	t.Cleanup(func() { httpClient = original })

	resp, _, err := sendOnce(ctx, "openai/gpt-4o-mini",
		map[string]string{"room_id": "!room:example.org"},
		[]byte(`{"model":"openai/gpt-4o-mini","messages":[{"role":"user","content":"hi"}]}`), 1)
	return captured, resp, err
}

const okResponse = `{"id":"chatcmpl-1","model":"gpt-4o-mini-2024-07-18",
	"choices":[{"index":0,"message":{"role":"assistant","content":"hi"},"finish_reason":"stop"}],
	"usage":{"prompt_tokens":10,"completion_tokens":2,"total_tokens":12,
	         "prompt_tokens_details":{"cached_tokens":8}}}`

// TestSendOnceBuildsTheRequest is the test that matters for the endpoint move.
//
// The individual helpers are covered separately, but a helper that is never called passes those
// tests and still breaks everything: dropping setTraceHeaders from sendOnce would leave Cloudflare
// generating its own trace ids again, with no error anywhere to show for it. This asserts on the
// request as it actually goes out.
func TestSendOnceBuildsTheRequest(t *testing.T) {
	const traceHex = "bad5326e2fca4bafbead818b13dc7111"

	req, resp, err := captureRequest(t, ctxWithSpan(t, traceHex, "d271e004dacacb31"), okResponse, 200)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if req == nil {
		t.Fatal("no request was captured")
	}

	t.Run("targets the Unified API", func(t *testing.T) {
		want := "https://gateway.ai.cloudflare.com/v1/acct123/siikabot/compat/chat/completions"
		if got := req.URL.String(); got != want {
			t.Errorf("URL = %q, want %q", got, want)
		}
	})

	t.Run("sends the trace context", func(t *testing.T) {
		if got := req.Header.Get("cf-aig-otel-trace-id"); got != traceHex {
			t.Errorf("cf-aig-otel-trace-id = %q, want %q", got, traceHex)
		}
		// The parent is whichever span is current inside sendOnce, so its id is not asserted
		// exactly — only that a well-formed one was sent
		if got := req.Header.Get("cf-aig-otel-parent-span-id"); len(got) != 16 {
			t.Errorf("cf-aig-otel-parent-span-id = %q, want 16 hex characters", got)
		}
	})

	t.Run("authenticates as the gateway expects", func(t *testing.T) {
		if got := req.Header.Get("cf-aig-authorization"); got != "Bearer cf-token" {
			t.Errorf("cf-aig-authorization = %q", got)
		}
		if got := req.Header.Get("Authorization"); got != "" {
			t.Errorf("Authorization should be left for a provider key, got %q", got)
		}
	})

	t.Run("still sends the metadata", func(t *testing.T) {
		var metadata map[string]string
		if err := json.Unmarshal([]byte(req.Header.Get("cf-aig-metadata")), &metadata); err != nil {
			t.Fatalf("metadata header is not valid JSON: %v", err)
		}
		if metadata["room_id"] != "!room:example.org" {
			t.Errorf("room_id = %q", metadata["room_id"])
		}
		if metadata["siikabot_trace_id"] != traceHex {
			t.Errorf("siikabot_trace_id = %q, want %q", metadata["siikabot_trace_id"], traceHex)
		}
	})

	t.Run("reads the flat response", func(t *testing.T) {
		if resp == nil {
			t.Fatal("expected a response")
		}
		if resp.Model != "gpt-4o-mini-2024-07-18" {
			t.Errorf("expected the resolved model id, got %q", resp.Model)
		}
		if len(resp.Choices) != 1 {
			t.Fatalf("expected 1 choice, got %d", len(resp.Choices))
		}
		if got := resp.Usage.CachedPromptTokens(); got != 8 {
			t.Errorf("expected 8 cached tokens, got %d", got)
		}
	})
}

// TestSendOnceTreatsAProviderErrorAsAFailure covers the case the removed envelope used to make
// obvious: the gateway returns 200 because it did its job, and the failure is only in the body.
// Read as a success, this would surface to the user as an empty reply.
func TestSendOnceTreatsAProviderErrorAsAFailure(t *testing.T) {
	body := `{"error":{"message":"The model does not exist","type":"invalid_request_error","code":"model_not_found"}}`

	_, resp, err := captureRequest(t, ctxWithSpan(t, "bad5326e2fca4bafbead818b13dc7111", "d271e004dacacb31"), body, 200)
	if err == nil {
		t.Fatal("a provider error returned with HTTP 200 was treated as a success")
	}
	if resp != nil {
		t.Errorf("expected no response alongside the error, got %+v", resp)
	}
	if !strings.Contains(err.Error(), "The model does not exist") {
		t.Errorf("the provider's message should reach the caller, got: %v", err)
	}
}
