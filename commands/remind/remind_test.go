package remind

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/Scrin/siikabot/config"
	"github.com/Scrin/siikabot/db"
)

func TestMain(m *testing.M) {
	// Set timezone for tests
	config.Timezone = "Europe/Helsinki"
	m.Run()
}

func TestRemindDuration(t *testing.T) {
	now := time.Date(2024, 1, 15, 12, 0, 0, 0, time.UTC)

	tests := []struct {
		name        string
		param       string
		wantErr     bool
		errContains string
		checkFunc   func(t *testing.T, result time.Time)
	}{
		{
			name:    "valid 1 hour",
			param:   "1h",
			wantErr: false,
			checkFunc: func(t *testing.T, result time.Time) {
				expected := now.Add(1 * time.Hour)
				if !result.Equal(expected) {
					t.Errorf("got %v, want %v", result, expected)
				}
			},
		},
		{
			name:    "valid 30 minutes",
			param:   "30m",
			wantErr: false,
			checkFunc: func(t *testing.T, result time.Time) {
				expected := now.Add(30 * time.Minute)
				if !result.Equal(expected) {
					t.Errorf("got %v, want %v", result, expected)
				}
			},
		},
		{
			name:    "valid combined duration",
			param:   "1h30m",
			wantErr: false,
			checkFunc: func(t *testing.T, result time.Time) {
				expected := now.Add(90 * time.Minute)
				if !result.Equal(expected) {
					t.Errorf("got %v, want %v", result, expected)
				}
			},
		},
		{
			name:    "valid complex duration",
			param:   "2h45m30s",
			wantErr: false,
			checkFunc: func(t *testing.T, result time.Time) {
				expected := now.Add(2*time.Hour + 45*time.Minute + 30*time.Second)
				if !result.Equal(expected) {
					t.Errorf("got %v, want %v", result, expected)
				}
			},
		},
		{
			name:    "minimum valid duration 1s",
			param:   "1s",
			wantErr: false,
			checkFunc: func(t *testing.T, result time.Time) {
				expected := now.Add(1 * time.Second)
				if !result.Equal(expected) {
					t.Errorf("got %v, want %v", result, expected)
				}
			},
		},
		{
			name:        "duration too short 500ms",
			param:       "500ms",
			wantErr:     true,
			errContains: "at least 1s",
		},
		{
			name:        "zero duration",
			param:       "0s",
			wantErr:     true,
			errContains: "at least 1s",
		},
		{
			name:        "negative duration",
			param:       "-1h",
			wantErr:     true,
			errContains: "at least 1s",
		},
		{
			name:    "invalid format",
			param:   "abc",
			wantErr: true,
		},
		{
			name:    "invalid unit",
			param:   "1x",
			wantErr: true,
		},
		{
			name:    "empty string",
			param:   "",
			wantErr: true,
		},
		{
			name:    "valid large duration",
			param:   "24h",
			wantErr: false,
			checkFunc: func(t *testing.T, result time.Time) {
				expected := now.Add(24 * time.Hour)
				if !result.Equal(expected) {
					t.Errorf("got %v, want %v", result, expected)
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := RemindDuration(now, tt.param)

			if tt.wantErr {
				if err == nil {
					t.Errorf("RemindDuration(%q) expected error, got nil", tt.param)
				} else if tt.errContains != "" && !strings.Contains(err.Error(), tt.errContains) {
					t.Errorf("RemindDuration(%q) error = %v, want error containing %q", tt.param, err, tt.errContains)
				}
				return
			}

			if err != nil {
				t.Errorf("RemindDuration(%q) unexpected error: %v", tt.param, err)
				return
			}

			if tt.checkFunc != nil {
				tt.checkFunc(t, result)
			}
		})
	}
}

func TestRemindTime(t *testing.T) {
	// Use a fixed reference time for testing
	// January 15, 2024, 12:00:00 in Europe/Helsinki
	loc, _ := time.LoadLocation("Europe/Helsinki")
	now := time.Date(2024, 1, 15, 12, 0, 0, 0, loc)

	tests := []struct {
		name        string
		param       string
		wantErr     bool
		errContains string
		checkFunc   func(t *testing.T, result time.Time)
	}{
		// DateTime formats without timezone (uses configured timezone)
		{
			name:    "datetime DD.MM.YYYY-HH:MM",
			param:   "16.1.2024-14:30",
			wantErr: false,
			checkFunc: func(t *testing.T, result time.Time) {
				expected := time.Date(2024, 1, 16, 14, 30, 0, 0, loc)
				if !result.Equal(expected) {
					t.Errorf("got %v, want %v", result, expected)
				}
			},
		},
		{
			name:    "datetime HH:MM-DD.MM.YYYY",
			param:   "14:30-16.1.2024",
			wantErr: false,
			checkFunc: func(t *testing.T, result time.Time) {
				expected := time.Date(2024, 1, 16, 14, 30, 0, 0, loc)
				if !result.Equal(expected) {
					t.Errorf("got %v, want %v", result, expected)
				}
			},
		},
		{
			name:    "datetime YYYY-MM-DD-HH:MM",
			param:   "2024-01-16-14:30",
			wantErr: false,
			checkFunc: func(t *testing.T, result time.Time) {
				expected := time.Date(2024, 1, 16, 14, 30, 0, 0, loc)
				if !result.Equal(expected) {
					t.Errorf("got %v, want %v", result, expected)
				}
			},
		},
		{
			name:    "datetime with seconds",
			param:   "16.1.2024-14:30:45",
			wantErr: false,
			checkFunc: func(t *testing.T, result time.Time) {
				expected := time.Date(2024, 1, 16, 14, 30, 45, 0, loc)
				if !result.Equal(expected) {
					t.Errorf("got %v, want %v", result, expected)
				}
			},
		},
		// Underscore replacement
		{
			name:    "underscore replaced with dash",
			param:   "2024-01-16_14:30",
			wantErr: false,
			checkFunc: func(t *testing.T, result time.Time) {
				expected := time.Date(2024, 1, 16, 14, 30, 0, 0, loc)
				if !result.Equal(expected) {
					t.Errorf("got %v, want %v", result, expected)
				}
			},
		},
		// Time-only formats (should be later today or tomorrow)
		{
			name:    "time only future today",
			param:   "14:30",
			wantErr: false,
			checkFunc: func(t *testing.T, result time.Time) {
				// Should be today at 14:30 since now is 12:00
				expected := time.Date(2024, 1, 15, 14, 30, 0, 0, loc)
				if !result.Equal(expected) {
					t.Errorf("got %v, want %v", result, expected)
				}
			},
		},
		{
			name:    "time only with seconds",
			param:   "14:30:45",
			wantErr: false,
			checkFunc: func(t *testing.T, result time.Time) {
				expected := time.Date(2024, 1, 15, 14, 30, 45, 0, loc)
				if !result.Equal(expected) {
					t.Errorf("got %v, want %v", result, expected)
				}
			},
		},
		// Date-only formats (should default to 09:00)
		{
			name:    "date only DD.MM.YYYY",
			param:   "16.1.2024",
			wantErr: false,
			checkFunc: func(t *testing.T, result time.Time) {
				expected := time.Date(2024, 1, 16, 9, 0, 0, 0, loc)
				if !result.Equal(expected) {
					t.Errorf("got %v, want %v", result, expected)
				}
			},
		},
		{
			name:    "date only YYYY-M-D",
			param:   "2024-1-16",
			wantErr: false,
			checkFunc: func(t *testing.T, result time.Time) {
				expected := time.Date(2024, 1, 16, 9, 0, 0, 0, loc)
				if !result.Equal(expected) {
					t.Errorf("got %v, want %v", result, expected)
				}
			},
		},
		// RFC3339 format (with timezone)
		{
			name:    "RFC3339 format",
			param:   "2024-01-16T14:30:00Z",
			wantErr: false,
			checkFunc: func(t *testing.T, result time.Time) {
				// Should preserve the UTC timezone from the input
				expected := time.Date(2024, 1, 16, 14, 30, 0, 0, time.UTC)
				if !result.Equal(expected) {
					t.Errorf("got %v, want %v", result, expected)
				}
			},
		},
		// Error cases
		{
			name:        "past datetime",
			param:       "14.1.2024-10:00", // January 14, 2024, 10:00 - before now
			wantErr:     true,
			errContains: "must be in future",
		},
		{
			name:    "invalid format",
			param:   "not-a-date",
			wantErr: true,
		},
		{
			name:    "empty string",
			param:   "",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := RemindTime(now, tt.param)

			if tt.wantErr {
				if err == nil {
					t.Errorf("RemindTime(%q) expected error, got nil", tt.param)
				} else if tt.errContains != "" && !strings.Contains(err.Error(), tt.errContains) {
					t.Errorf("RemindTime(%q) error = %v, want error containing %q", tt.param, err, tt.errContains)
				}
				return
			}

			if err != nil {
				t.Errorf("RemindTime(%q) unexpected error: %v", tt.param, err)
				return
			}

			if tt.checkFunc != nil {
				tt.checkFunc(t, result)
			}
		})
	}
}

func TestRemindTimeRollsOverToNextDay(t *testing.T) {
	loc, _ := time.LoadLocation("Europe/Helsinki")
	// Set now to 14:00, so "10:00" should roll over to next day
	now := time.Date(2024, 1, 15, 14, 0, 0, 0, loc)

	result, err := RemindTime(now, "10:00")
	if err != nil {
		t.Fatalf("RemindTime unexpected error: %v", err)
	}

	// Should be tomorrow at 10:00
	expected := time.Date(2024, 1, 16, 10, 0, 0, 0, loc)
	if !result.Equal(expected) {
		t.Errorf("got %v, want %v (next day)", result, expected)
	}
}

// The undo command posted when the chat deletes a reminder must set the same reminder again: the
// same time, to the second, and the same message, when it is sent as plain text
func TestUndoCommandSetsTheSameReminderAgain(t *testing.T) {
	loc := Location()
	// Winter time, so that a reminder in summer time is set across a change of offset
	now := time.Date(2026, 1, 15, 12, 0, 0, 0, loc)

	tests := []struct {
		name string
		at   time.Time
		msg  string
	}{
		{"set with a duration, with a fraction of a second", now.Add(90*time.Minute + 734512*time.Microsecond), "Dentist"},
		{"set for a date", time.Date(2026, 2, 1, 9, 0, 0, 0, loc), "Pay the rent"},
		{"in summer time", time.Date(2026, 7, 1, 9, 30, 15, 0, loc), "Midsummer sauna"},
		// 01:30 UTC is the second 03:30 of 25.10.2026 in Helsinki, after the clocks went back
		{"in the hour that repeats", time.Date(2026, 10, 25, 1, 30, 0, 0, time.UTC), "Second 03:30"},
		{"with HTML", now.Add(time.Hour), "Line one<br>line two with <em>emphasis</em> &amp; more"},
		{"with characters HTML escapes", now.Add(time.Hour), `Milk & eggs <3 "quoted"`},
		{"with spaces in a row", now.Add(time.Hour), "two  spaces   three"},
		{"with Markdown characters", now.Add(time.Hour), "*not* _emphasis_ `code`"},
		{"with non-ASCII text", now.Add(time.Hour), "Hämeenlinna 🐟"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rem := db.Reminder{RemindTime: tt.at, Message: tt.msg}
			cmd := UndoCommand(rem)

			// As bot.go routes it and Handle reads it
			params := strings.SplitN(cmd, " ", 3)
			if params[0] != "!remind" || len(params) < 3 {
				t.Fatalf("UndoCommand() = %q, not a !remind command", cmd)
			}
			got, err := ParseTime(now, params[1])
			if err != nil {
				t.Fatalf("the time of %q doesn't parse: %v", cmd, err)
			}
			if want := tt.at.Truncate(time.Second); !got.Equal(want) {
				t.Errorf("%q sets the reminder for %v, want %v", cmd, got, want)
			}
			if got := messageText(cmd, "", ""); got != tt.msg {
				t.Errorf("%q sets the message %q, want %q", cmd, got, tt.msg)
			}
		})
	}
}

func TestUndoCommandGivesTheTimeWithItsOffset(t *testing.T) {
	rem := db.Reminder{RemindTime: time.Date(2026, 11, 20, 7, 0, 0, 0, time.UTC), Message: "Dentist"}
	if got, want := UndoCommand(rem), "!remind 2026-11-20T09:00:00+02:00 Dentist"; got != want {
		t.Errorf("UndoCommand() = %q, want %q", got, want)
	}
}

// The messages that create a reminder read as they did before Describe
func TestDescribeReadsLikeTheCreationMessage(t *testing.T) {
	loc := Location()
	now := time.Date(2026, 11, 20, 7, 30, 0, 250*int(time.Millisecond), loc)
	rem := db.Reminder{RemindTime: time.Date(2026, 11, 20, 9, 0, 0, 0, loc), Message: "Dentist<br>bring the card"}

	if got, want := "Reminding "+Describe(rem, now), "Reminding at 09:00:00 on 20.11.2026 (in 1h29m59s): Dentist<br>bring the card"; got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestDeletionNoticeShowsTheUndoCommandAsWritten(t *testing.T) {
	loc := Location()
	now := time.Date(2026, 11, 13, 12, 0, 0, 0, loc)
	rem := db.Reminder{RemindTime: time.Date(2026, 11, 20, 9, 0, 0, 0, loc), Message: "Dentist<br>bring the <em>card</em> & pen"}

	want := "[AI tool call] Deleted the reminder at 09:00:00 on 20.11.2026 (in 165h0m0s): Dentist<br>bring the <em>card</em> & pen" +
		"<br>To undo this, send this message: <code>!remind 2026-11-20T09:00:00+02:00 Dentist&lt;br&gt;bring the &lt;em&gt;card&lt;/em&gt; &amp; pen</code>"
	if got := DeletionNotice(rem, now); got != want {
		t.Errorf("DeletionNotice() =\n%s\nwant\n%s", got, want)
	}
}

func TestParseTimeExplainsBothWaysOfReadingIt(t *testing.T) {
	_, err := ParseTime(time.Now(), "someday")
	timeErr, ok := errors.AsType[*TimeError](err)
	if !ok || timeErr.DurationErr == nil || timeErr.TimeErr == nil || timeErr.Param != "someday" {
		t.Errorf("ParseTime() error = %#v, want a TimeError with both reasons", err)
	}
}
