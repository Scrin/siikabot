package logging

import (
	"context"
	"testing"
)

func TestContextWithStr(t *testing.T) {
	tests := []struct {
		name     string
		key      string
		value    string
		checkKey string
	}{
		{
			name:     "add string field",
			key:      "user_id",
			value:    "@user:example.com",
			checkKey: "user_id",
		},
		{
			name:     "empty key",
			key:      "",
			value:    "value",
			checkKey: "",
		},
		{
			name:     "empty value",
			key:      "key",
			value:    "",
			checkKey: "key",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()
			newCtx := ContextWithStr(ctx, tt.key, tt.value)

			// Verify context is not nil
			if newCtx == nil {
				t.Fatal("ContextWithStr returned nil context")
			}

			// Verify the field was added
			fctx := getFieldContext(newCtx)
			if got, ok := fctx.strValues[tt.checkKey]; !ok || got != tt.value {
				t.Errorf("ContextWithStr did not add field correctly: got %q, want %q", got, tt.value)
			}
		})
	}
}

func TestContextWithInt(t *testing.T) {
	tests := []struct {
		name     string
		key      string
		value    int
		checkKey string
	}{
		{
			name:     "add int field",
			key:      "count",
			value:    42,
			checkKey: "count",
		},
		{
			name:     "zero value",
			key:      "zero",
			value:    0,
			checkKey: "zero",
		},
		{
			name:     "negative value",
			key:      "negative",
			value:    -10,
			checkKey: "negative",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()
			newCtx := ContextWithInt(ctx, tt.key, tt.value)

			// Verify context is not nil
			if newCtx == nil {
				t.Fatal("ContextWithInt returned nil context")
			}

			// Verify the field was added
			fctx := getFieldContext(newCtx)
			if got, ok := fctx.intValues[tt.checkKey]; !ok || got != tt.value {
				t.Errorf("ContextWithInt did not add field correctly: got %d, want %d", got, tt.value)
			}
		})
	}
}

func TestContextChaining(t *testing.T) {
	ctx := context.Background()

	// Chain multiple string fields
	ctx = ContextWithStr(ctx, "key1", "value1")
	ctx = ContextWithStr(ctx, "key2", "value2")

	fctx := getFieldContext(ctx)
	if fctx.strValues["key1"] != "value1" {
		t.Errorf("key1 not preserved after chaining: got %q, want %q", fctx.strValues["key1"], "value1")
	}
	if fctx.strValues["key2"] != "value2" {
		t.Errorf("key2 not set: got %q, want %q", fctx.strValues["key2"], "value2")
	}
}

func TestContextMixedFields(t *testing.T) {
	ctx := context.Background()

	// Add both string and int fields
	ctx = ContextWithStr(ctx, "room_id", "!room:example.com")
	ctx = ContextWithInt(ctx, "message_count", 100)
	ctx = ContextWithStr(ctx, "user_id", "@user:example.com")
	ctx = ContextWithInt(ctx, "retry_count", 3)

	fctx := getFieldContext(ctx)

	// Verify string fields
	if fctx.strValues["room_id"] != "!room:example.com" {
		t.Errorf("room_id incorrect: got %q", fctx.strValues["room_id"])
	}
	if fctx.strValues["user_id"] != "@user:example.com" {
		t.Errorf("user_id incorrect: got %q", fctx.strValues["user_id"])
	}

	// Verify int fields
	if fctx.intValues["message_count"] != 100 {
		t.Errorf("message_count incorrect: got %d", fctx.intValues["message_count"])
	}
	if fctx.intValues["retry_count"] != 3 {
		t.Errorf("retry_count incorrect: got %d", fctx.intValues["retry_count"])
	}
}

func TestContextOverwrite(t *testing.T) {
	ctx := context.Background()

	// Add a field then overwrite it
	ctx = ContextWithStr(ctx, "status", "pending")
	ctx = ContextWithStr(ctx, "status", "completed")

	fctx := getFieldContext(ctx)
	if fctx.strValues["status"] != "completed" {
		t.Errorf("status not overwritten: got %q, want %q", fctx.strValues["status"], "completed")
	}
}

func TestGetFieldContextFromEmptyContext(t *testing.T) {
	ctx := context.Background()
	fctx := getFieldContext(ctx)

	// Should return initialized maps, not nil
	if fctx.strValues == nil {
		t.Error("strValues should not be nil for empty context")
	}
	if fctx.intValues == nil {
		t.Error("intValues should not be nil for empty context")
	}

	// Maps should be empty
	if len(fctx.strValues) != 0 {
		t.Errorf("strValues should be empty, got %d entries", len(fctx.strValues))
	}
	if len(fctx.intValues) != 0 {
		t.Errorf("intValues should be empty, got %d entries", len(fctx.intValues))
	}
}
