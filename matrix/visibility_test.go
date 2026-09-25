package matrix

import (
	"testing"

	"maunium.net/go/mautrix/event"
)

// Only a room that shows members its whole history lets anyone refer to a message from before they
// joined. Anything else, including a setting the bot doesn't know, is checked against when they did.
func TestHistoryIsShared(t *testing.T) {
	tests := []struct {
		visibility event.HistoryVisibility
		want       bool
	}{
		{event.HistoryVisibilityShared, true},
		{event.HistoryVisibilityWorldReadable, true},
		{event.HistoryVisibilityJoined, false},
		{event.HistoryVisibilityInvited, false},
		{"", false},
		{"members_only", false},
	}
	for _, tt := range tests {
		if got := historyIsShared(tt.visibility); got != tt.want {
			t.Errorf("historyIsShared(%q) = %v, want %v", tt.visibility, got, tt.want)
		}
	}
}
