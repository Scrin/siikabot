package chat

import (
	"strings"
	"testing"
)

func TestModelArgument(t *testing.T) {
	for _, tt := range []struct {
		msg   string
		model string
		ok    bool
	}{
		{"!chat model openrouter/deepseek/deepseek-v4.1-flash", "openrouter/deepseek/deepseek-v4.1-flash", true},
		{"!chat model  openrouter/deepseek/deepseek-v4.1-flash ", "openrouter/deepseek/deepseek-v4.1-flash", true},
		{"!chat model", "", false},
		{"!chat model ", "", false},
		// The old form, which named the kind of model first, would otherwise set the model to "text"
		{"!chat model text openrouter/deepseek/deepseek-v4.1-flash", "", false},
	} {
		model, ok := modelArgument(strings.Split(tt.msg, " "))
		if model != tt.model || ok != tt.ok {
			t.Errorf("modelArgument(%q) = %q, %v, want %q, %v", tt.msg, model, ok, tt.model, tt.ok)
		}
	}
}
