package config

import "testing"

func TestBoolToYesNo(t *testing.T) {
	tests := []struct {
		name     string
		input    bool
		expected string
	}{
		{
			name:     "true returns yes",
			input:    true,
			expected: "yes",
		},
		{
			name:     "false returns no",
			input:    false,
			expected: "no",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := boolToYesNo(tt.input)
			if result != tt.expected {
				t.Errorf("boolToYesNo(%v) = %q, want %q", tt.input, result, tt.expected)
			}
		})
	}
}

func TestResolveUserID(t *testing.T) {
	tests := []struct {
		name           string
		arg            string
		mentionedUsers []string
		expected       string
	}{
		{
			name:           "valid user ID format",
			arg:            "@user:example.com",
			mentionedUsers: nil,
			expected:       "@user:example.com",
		},
		{
			name:           "valid user ID with subdomain",
			arg:            "@alice:matrix.example.org",
			mentionedUsers: nil,
			expected:       "@alice:matrix.example.org",
		},
		{
			name:           "missing @ prefix falls back to mentions",
			arg:            "user:example.com",
			mentionedUsers: []string{"@fallback:user.com"},
			expected:       "@fallback:user.com",
		},
		{
			name:           "missing colon falls back to mentions",
			arg:            "@user",
			mentionedUsers: []string{"@fallback:user.com"},
			expected:       "@fallback:user.com",
		},
		{
			name:           "display name with mentions fallback",
			arg:            "Alice",
			mentionedUsers: []string{"@alice:example.com"},
			expected:       "@alice:example.com",
		},
		{
			name:           "invalid format with no mentions returns empty",
			arg:            "invalid",
			mentionedUsers: nil,
			expected:       "",
		},
		{
			name:           "invalid format with empty mentions returns empty",
			arg:            "invalid",
			mentionedUsers: []string{},
			expected:       "",
		},
		{
			name:           "multiple mentions uses first one",
			arg:            "display-name",
			mentionedUsers: []string{"@first:user.com", "@second:user.com"},
			expected:       "@first:user.com",
		},
		{
			name:           "valid user ID ignores mentions",
			arg:            "@valid:user.com",
			mentionedUsers: []string{"@other:user.com"},
			expected:       "@valid:user.com",
		},
		{
			name:           "empty argument with mentions uses first mention",
			arg:            "",
			mentionedUsers: []string{"@mentioned:user.com"},
			expected:       "@mentioned:user.com",
		},
		{
			name:           "empty argument with no mentions returns empty",
			arg:            "",
			mentionedUsers: nil,
			expected:       "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := resolveUserID(tt.arg, tt.mentionedUsers)
			if result != tt.expected {
				t.Errorf("resolveUserID(%q, %v) = %q, want %q", tt.arg, tt.mentionedUsers, result, tt.expected)
			}
		})
	}
}
