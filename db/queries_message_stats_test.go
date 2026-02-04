package db

import "testing"

func TestCountWords(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected int
	}{
		{
			name:     "single word",
			input:    "hello",
			expected: 1,
		},
		{
			name:     "multiple words",
			input:    "hello world",
			expected: 2,
		},
		{
			name:     "empty string",
			input:    "",
			expected: 0,
		},
		{
			name:     "only whitespace",
			input:    "   ",
			expected: 0,
		},
		{
			name:     "multiple spaces between words",
			input:    "hello    world",
			expected: 2,
		},
		{
			name:     "tabs and newlines",
			input:    "hello\tworld\nfoo",
			expected: 3,
		},
		{
			name:     "leading and trailing whitespace",
			input:    "  hello world  ",
			expected: 2,
		},
		{
			name:     "unicode words",
			input:    "こんにちは 世界",
			expected: 2,
		},
		{
			name:     "mixed whitespace types",
			input:    "one\ttwo\nthree four",
			expected: 4,
		},
		{
			name:     "single character words",
			input:    "a b c d e",
			expected: 5,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := countWords(tt.input)
			if result != tt.expected {
				t.Errorf("countWords(%q) = %d, want %d", tt.input, result, tt.expected)
			}
		})
	}
}
