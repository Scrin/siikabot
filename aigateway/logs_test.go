package aigateway

import (
	"encoding/json"
	"testing"
	"time"
)

// TestGatewayLogParsing verifies a real logs API payload parses, including the optional cost
// field and the non-sortable hash ids.
func TestGatewayLogParsing(t *testing.T) {
	body := []byte(`{
		"success": true,
		"errors": [],
		"result": [
			{
				"id": "386e367f5df9fc5f06d759672c80d6601a641236830982985b6895c16d7b7ed6",
				"created_at": "2026-08-05T14:44:36.727Z",
				"model": "openai/gpt-4o-mini",
				"provider": "openai",
				"cost": 0.000027449999999999996,
				"custom_cost": false,
				"tokens_in": 87,
				"tokens_out": 24,
				"duration": 824,
				"cached": false,
				"success": true,
				"status_code": 200
			},
			{
				"id": "a1ce1c8691dd1ace6a8ce1c439505b6938fe5ef823042c70cff5786de351169e",
				"created_at": "2026-08-05T14:44:40.346Z",
				"model": "openai/definitely-not-a-real-model",
				"provider": "unknown",
				"tokens_in": 0,
				"tokens_out": 0,
				"duration": 0,
				"cached": false,
				"success": false,
				"status_code": 500
			}
		]
	}`)

	var resp logsResponse
	if err := json.Unmarshal(body, &resp); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(resp.Result) != 2 {
		t.Fatalf("expected 2 logs, got %d", len(resp.Result))
	}

	first := resp.Result[0]
	if first.Cost == nil {
		t.Fatal("expected cost to be populated")
	}
	if *first.Cost <= 0 {
		t.Errorf("expected a positive cost, got %v", *first.Cost)
	}
	if first.TokensIn != 87 || first.TokensOut != 24 {
		t.Errorf("unexpected tokens: in=%d out=%d", first.TokensIn, first.TokensOut)
	}
	if first.Duration != 824 {
		t.Errorf("expected duration 824, got %d", first.Duration)
	}
	if first.CreatedAt.IsZero() {
		t.Error("expected created_at to be parsed")
	}

	// An absent cost must stay nil rather than becoming a spurious zero-cost sample
	if resp.Result[1].Cost != nil {
		t.Errorf("expected a nil cost when absent, got %v", *resp.Result[1].Cost)
	}
	if resp.Result[1].Success {
		t.Error("expected the second entry to be a failure")
	}
}

func mustTime(t *testing.T, s string) time.Time {
	t.Helper()
	parsed, err := time.Parse(time.RFC3339Nano, s)
	if err != nil {
		t.Fatalf("bad time %q: %v", s, err)
	}
	return parsed
}
