package aigateway

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"testing"
)

// TestClassifyResponseErrorFromRealFailures uses the error shapes actually observed from the
// gateway, so the classifier is pinned to reality rather than to guesses about it.
func TestClassifyResponseErrorFromRealFailures(t *testing.T) {
	cases := []struct {
		name       string
		statusCode int
		errs       []apiError
		want       ErrorKind
	}{
		{
			name:       "unknown model",
			statusCode: http.StatusNotFound,
			errs:       []apiError{{Code: 7003, Message: "Model not found: openai/definitely-not-a-real-model"}},
			want:       ErrorKindModelNotFound,
		},
		{
			name:       "token missing the AI Gateway permission",
			statusCode: http.StatusForbidden,
			errs:       []apiError{{Code: 10000, Message: "Authentication error"}},
			want:       ErrorKindAuth,
		},
		{
			name:       "wholesale throttling",
			statusCode: http.StatusOK,
			errs:       []apiError{{Code: 7003, Message: "Wholesale rate limit exceeded for this gateway. Please reduce request rate or use BYOK."}},
			want:       ErrorKindRateLimited,
		},
		{
			name:       "explicit rate limit status",
			statusCode: http.StatusTooManyRequests,
			want:       ErrorKindRateLimited,
		},
		{
			name:       "rejected request body",
			statusCode: http.StatusBadRequest,
			errs:       []apiError{{Code: 7003, Message: "Model execution failed (User Input Error): Unknown parameter: 'image'."}},
			want:       ErrorKindBadRequest,
		},
		{
			name:       "provider outage",
			statusCode: http.StatusBadGateway,
			errs:       []apiError{{Code: 1, Message: "upstream failure"}},
			want:       ErrorKindProviderError,
		},
		{
			name:       "gateway timeout",
			statusCode: http.StatusGatewayTimeout,
			want:       ErrorKindTimeout,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := classifyResponseError(tc.statusCode, tc.errs); got != tc.want {
				t.Errorf("classifyResponseError(%d) = %q, want %q", tc.statusCode, got, tc.want)
			}
		})
	}
}

func TestClassifyTransportError(t *testing.T) {
	if got := classifyTransportError(context.DeadlineExceeded); got != ErrorKindTimeout {
		t.Errorf("expected a deadline to classify as timeout, got %q", got)
	}
	if got := classifyTransportError(context.Canceled); got != ErrorKindCancelled {
		t.Errorf("expected a cancellation to classify as cancelled, got %q", got)
	}
	// Wrapped, as it arrives through fmt.Errorf in the call path
	wrapped := fmt.Errorf("failed to send chat request: %w", context.DeadlineExceeded)
	if got := classifyTransportError(wrapped); got != ErrorKindTimeout {
		t.Errorf("expected a wrapped deadline to classify as timeout, got %q", got)
	}
	netErr := &net.OpError{Op: "dial", Err: errors.New("connection refused")}
	if got := classifyTransportError(netErr); got != ErrorKindNetwork {
		t.Errorf("expected a dial failure to classify as network, got %q", got)
	}
	if got := classifyTransportError(errors.New("something else")); got != ErrorKindUnknown {
		t.Errorf("expected an unrecognised error to classify as unknown, got %q", got)
	}
}

func TestClassifyToolError(t *testing.T) {
	// Tools wrap their argument parsing failures, which is how the cause stays reachable
	var target struct{ Location string }
	parseErr := json.Unmarshal([]byte(`{"location": 42}`), &target)
	wrapped := fmt.Errorf("failed to parse arguments: %w", parseErr)
	if got := ClassifyToolError(wrapped); got != ErrorKindParseError {
		t.Errorf("expected bad tool arguments to classify as parse_error, got %q", got)
	}

	if got := ClassifyToolError(fmt.Errorf("fetch failed: %w", context.DeadlineExceeded)); got != ErrorKindTimeout {
		t.Errorf("expected a tool timeout to classify as timeout, got %q", got)
	}

	if got := ClassifyToolError(errors.New("the weather API returned 500")); got != ErrorKindProviderError {
		t.Errorf("expected an unrecognised tool failure to classify as provider_error, got %q", got)
	}

	if got := ClassifyToolError(nil); got != ErrorKindUnknown {
		t.Errorf("expected a nil error to classify as unknown, got %q", got)
	}
}

// TestErrorKindsAreBounded guards the property that matters for a public metrics endpoint: the label
// values are a fixed set, never anything derived from a message body.
func TestErrorKindsAreBounded(t *testing.T) {
	known := map[ErrorKind]bool{
		ErrorKindTimeout: true, ErrorKindNetwork: true, ErrorKindCancelled: true,
		ErrorKindRateLimited: true, ErrorKindAuth: true, ErrorKindModelNotFound: true,
		ErrorKindBadRequest: true, ErrorKindProviderError: true, ErrorKindParseError: true,
		ErrorKindUnknown: true,
	}

	// A message carrying something room-shaped must not leak into the label
	got := classifyResponseError(http.StatusBadRequest, []apiError{
		{Code: 1, Message: "failure in room !secretroom:example.org for user @someone:example.org"},
	})

	if !known[got] {
		t.Errorf("classifier produced an unbounded label value: %q", got)
	}
}

// TestCachedPromptTokens covers the accessor used for the cache hit rate, including the shapes where
// the provider reports no cache information at all
func TestCachedPromptTokens(t *testing.T) {
	var nilUsage *Usage
	if got := nilUsage.CachedPromptTokens(); got != 0 {
		t.Errorf("expected 0 for nil usage, got %d", got)
	}

	if got := (&Usage{PromptTokens: 100}).CachedPromptTokens(); got != 0 {
		t.Errorf("expected 0 when details are absent, got %d", got)
	}

	usage := &Usage{PromptTokens: 100, PromptTokensDetails: &PromptTokensDetails{CachedTokens: 64}}
	if got := usage.CachedPromptTokens(); got != 64 {
		t.Errorf("expected 64, got %d", got)
	}
}

// TestUsageParsesCachedTokens verifies the field is read from a real response body
func TestUsageParsesCachedTokens(t *testing.T) {
	body := []byte(`{
		"result": {"choices": [{"message": {"role": "assistant", "content": "hi"}}],
			"usage": {"prompt_tokens": 8518, "completion_tokens": 1, "total_tokens": 8519,
				"prompt_tokens_details": {"cached_tokens": 8192, "audio_tokens": 0}}},
		"success": true, "errors": []
	}`)

	var resp runResponse
	if err := json.Unmarshal(body, &resp); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if got := resp.Result.Usage.CachedPromptTokens(); got != 8192 {
		t.Errorf("expected 8192 cached tokens, got %d", got)
	}
}
