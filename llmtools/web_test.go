package llmtools

import (
	"context"
	"strings"
	"testing"
)

// TestWebToolTakesTheTurnsConfiguration verifies that the tool takes the size of a part from the
// configuration of the turn it runs in, rather than reading one of its own, which may have changed
// since the turn began
func TestWebToolTakesTheTurnsConfiguration(t *testing.T) {
	_, err := handleWebToolCall(context.Background(), `{"url":"https://example.com"}`)
	if err == nil || !strings.Contains(err.Error(), "chat configuration") {
		t.Errorf("without the turn's configuration: %v, want an error saying it's missing", err)
	}
}
