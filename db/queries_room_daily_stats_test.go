package db

import (
	"testing"
	"time"
)

// A message is counted on its date in the bot's time zone, which can be a different date than in UTC
func TestStatDateFollowsTheBotsTimeZone(t *testing.T) {
	ahead := time.FixedZone("UTC+3", 3*60*60)
	behind := time.FixedZone("UTC-5", -5*60*60)

	tests := []struct {
		name string
		sent time.Time
		loc  *time.Location
		want string
	}{
		{"after midnight ahead of UTC", time.Date(2026, 10, 9, 22, 30, 0, 0, time.UTC), ahead, "2026-10-10"},
		{"before midnight behind UTC", time.Date(2026, 10, 10, 1, 30, 0, 0, time.UTC), behind, "2026-10-09"},
		{"the same date in both", time.Date(2026, 10, 10, 12, 0, 0, 0, time.UTC), ahead, "2026-10-10"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := statDate(tt.sent, tt.loc)
			if got.Format(time.DateOnly) != tt.want {
				t.Errorf("statDate() = %s, want %s", got.Format(time.DateOnly), tt.want)
			}
			// Midnight UTC, which is how a stat_date reads back, so the two compare equal
			if got.Location() != time.UTC || got.Hour() != 0 || got.Minute() != 0 {
				t.Errorf("statDate() = %v, want midnight UTC", got)
			}
		})
	}
}

// A stat date begins at midnight in the bot's time zone, which lines it up with records kept by
// timestamp
func TestStatDayStartIsMidnightInTheBotsTimeZone(t *testing.T) {
	ahead := time.FixedZone("UTC+3", 3*60*60)
	date := time.Date(2026, 10, 10, 0, 0, 0, 0, time.UTC)

	got := statDayStart(date, ahead)
	if want := time.Date(2026, 10, 9, 21, 0, 0, 0, time.UTC); !got.Equal(want) {
		t.Errorf("statDayStart() = %v, want %v", got, want)
	}
	// A message sent at that moment is counted on that date
	if counted := statDate(got, ahead); !counted.Equal(date) {
		t.Errorf("a message sent at the start of the day is counted on %v", counted)
	}
}
