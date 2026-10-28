package webhooks

import (
	"errors"
	"strings"
	"testing"
)

func TestValidateNameTrimsAndChecks(t *testing.T) {
	tests := []struct {
		name string
		want string
		err  error
	}{
		{"hermes", "hermes", nil},
		{"  hermes \t", "hermes", nil},
		{"Backup job ✓", "Backup job ✓", nil},
		{strings.Repeat("ä", MaxNameLength), strings.Repeat("ä", MaxNameLength), nil},
		{"", "", ErrEmptyName},
		{"   ", "", ErrEmptyName},
		{strings.Repeat("a", MaxNameLength+1), "", ErrNameTooLong},
		{"two\nlines", "", ErrNameNotOneLine},
		{"tab\tinside", "", ErrNameNotOneLine},
	}
	for _, tt := range tests {
		got, err := ValidateName(tt.name)
		if got != tt.want || !errors.Is(err, tt.err) {
			t.Errorf("ValidateName(%q) = %q, %v; want %q, %v", tt.name, got, err, tt.want, tt.err)
		}
	}
}

func TestValidateTokenTakesHeaderSafeTokensOfTheRightLength(t *testing.T) {
	tests := []struct {
		token string
		err   error
	}{
		{strings.Repeat("a", MinTokenLength), nil},
		{strings.Repeat("a", MaxTokenLength), nil},
		{"Ab3_-~!#$%&'*+.^`|0123", nil},
		{strings.Repeat("a", MinTokenLength-1), ErrTokenLength},
		{strings.Repeat("a", MaxTokenLength+1), ErrTokenLength},
		{"with a space in the middle", ErrTokenCharacters},
		{"tab\tinside-the-token", ErrTokenCharacters},
		{"äääääääääääääääää", ErrTokenCharacters},
		{" leading-space-token", ErrTokenCharacters},
	}
	for _, tt := range tests {
		if err := ValidateToken(tt.token); !errors.Is(err, tt.err) {
			t.Errorf("ValidateToken(%q) = %v, want %v", tt.token, err, tt.err)
		}
	}
}

func TestTokenMatchesOnlyTheHashedToken(t *testing.T) {
	hash := HashToken("correct-horse-battery-staple")
	if !TokenMatches(hash, "correct-horse-battery-staple") {
		t.Error("the token doesn't match its own hash")
	}
	for _, token := range []string{"", "correct-horse-battery-stapl", "correct-horse-battery-staple ", "Correct-horse-battery-staple"} {
		if TokenMatches(hash, token) {
			t.Errorf("%q matches the hash of another token", token)
		}
	}
	if TokenMatches(nil, "") {
		t.Error("an empty token matches a missing hash")
	}
}

func TestNewIDIsRandomAndURLSafe(t *testing.T) {
	first, err := NewID()
	if err != nil {
		t.Fatal(err)
	}
	second, err := NewID()
	if err != nil {
		t.Fatal(err)
	}
	if first == second {
		t.Errorf("two IDs are both %q", first)
	}
	if len(first) != 22 || strings.ContainsAny(first, "+/=") {
		t.Errorf("ID %q isn't 16 bytes of base64url", first)
	}
}

func TestParseMessage(t *testing.T) {
	tests := []struct {
		name        string
		contentType string
		body        string
		want        string
		err         error
	}{
		{"plain text", "text/plain", "hello there", "hello there", nil},
		{"curl -d", "application/x-www-form-urlencoded", "hello there", "hello there", nil},
		{"no content type", "", "hello there", "hello there", nil},
		{"file with a trailing newline", "", "line one\nline two\n", "line one\nline two", nil},
		{"form-looking text is left alone", "application/x-www-form-urlencoded", "a=b&c=d", "a=b&c=d", nil},
		{"JSON", "application/json", `{"message": "hello there"}`, "hello there", nil},
		{"JSON with a charset", "application/json; charset=utf-8", `{"message": " hi\n"}`, "hi", nil},
		{"JSON with other fields", "application/json", `{"message": "hi", "level": "info"}`, "hi", nil},
		{"text that looks like JSON", "text/plain", `{"message": "hi"}`, `{"message": "hi"}`, nil},
		{"empty", "text/plain", "", "", ErrEmptyMessage},
		{"whitespace only", "text/plain", " \n\t ", "", ErrEmptyMessage},
		{"empty JSON message", "application/json", `{"message": "  "}`, "", ErrEmptyMessage},
		{"JSON without a message", "application/json", `{"text": "hi"}`, "", ErrInvalidJSON},
		{"JSON message that isn't a string", "application/json", `{"message": 5}`, "", ErrInvalidJSON},
		{"broken JSON", "application/json", `{"message": "hi"`, "", ErrInvalidJSON},
		{"invalid UTF-8", "text/plain", "caf\xe9", "", ErrInvalidUTF8},
		{"invalid UTF-8 in JSON", "application/json", "{\"message\": \"caf\xe9\"}", "", ErrInvalidUTF8},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ParseMessage(tt.contentType, []byte(tt.body))
			if got != tt.want || !errors.Is(err, tt.err) {
				t.Errorf("ParseMessage(%q, %q) = %q, %v; want %q, %v", tt.contentType, tt.body, got, err, tt.want, tt.err)
			}
		})
	}
}
