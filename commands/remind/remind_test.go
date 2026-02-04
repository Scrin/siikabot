package remind

import (
	"strings"
	"testing"
	"time"

	"github.com/Scrin/siikabot/config"
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
