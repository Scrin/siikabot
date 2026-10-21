package api

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/Scrin/siikabot/db"
)

// configHistory is a history of three configurations, newest first: the first, one that lengthened
// the turns, and one that changed the model and the rounds of tool calls
func configHistory() []db.ChatConfig {
	first := db.ChatConfig{
		ID: 1, CreatedAt: time.Date(2026, 9, 25, 12, 27, 0, 0, time.UTC),
		Model: "first-model", ContextHighTokens: 16384, ContextLowTokens: 8192, MaxToolIterations: 5,
		MaxResponseTokens: 2048, ImageDetail: "low", WebPartBytes: 10240,
		TurnTimeout: 3 * time.Minute, ModelTimeout: 45 * time.Second, ModelAttempts: 3,
	}
	second := first
	second.ID, second.CreatedAt, second.TurnTimeout = 2, first.CreatedAt.Add(time.Hour), 10*time.Minute
	third := second
	third.ID, third.CreatedAt, third.Model, third.MaxToolIterations = 3, second.CreatedAt.Add(time.Hour), "second-model", 8
	return []db.ChatConfig{third, second, first}
}

func TestChatConfigResponse(t *testing.T) {
	response := chatConfigResponse(configHistory())

	if response.ID != 3 {
		t.Errorf("the configuration in effect is #%d, want #3", response.ID)
	}

	// The settings are those in effect, each with how to change it
	settings := make(map[string]ChatSettingEntry)
	for _, setting := range response.Settings {
		settings[setting.Key] = setting
	}
	for key, want := range map[string]string{"model": "second-model", "max_tool_iterations": "8", "turn_timeout": "10m", "context_high_tokens": "16384"} {
		if got := settings[key].Value; got != want {
			t.Errorf("%s = %q, want %q", key, got, want)
		}
	}
	if got := settings["context_high_tokens"].Unit; got != "tokens" {
		t.Errorf("the unit of context_high_tokens is %q, want tokens", got)
	}
	for _, setting := range response.Settings {
		if !strings.HasPrefix(setting.Command, "!chat set "+setting.Key+" ") || setting.Summary == "" || setting.Group == "" {
			t.Errorf("setting %s is listed as %+v, without what it is or how to change it", setting.Key, setting)
		}
	}

	// Each configuration says what it changed from the one before it
	if len(response.History) != 3 {
		t.Fatalf("the history lists %d configurations, want 3", len(response.History))
	}
	changed := func(version ChatConfigVersion) string {
		var changes []string
		for _, change := range version.Changes {
			changes = append(changes, change.Key+" "+change.From+" → "+change.To)
		}
		return strings.Join(changes, ", ")
	}
	if got, want := changed(response.History[0]), "model first-model → second-model, max_tool_iterations 5 → 8"; got != want {
		t.Errorf("#3 changed %q, want %q", got, want)
	}
	if got, want := changed(response.History[1]), "turn_timeout 3m → 10m"; got != want {
		t.Errorf("#2 changed %q, want %q", got, want)
	}
	if first := response.History[2]; !first.First || len(first.Changes) != 0 {
		t.Errorf("#1 is listed as %+v, want the first configuration, which changed nothing", first)
	}

	// The first configuration has an empty list of changes rather than none
	encoded, err := json.Marshal(response.History[2])
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(encoded), `"changes":[],"first":true`) {
		t.Errorf("the first configuration encodes as %s", encoded)
	}
}

// Only the latest configurations are listed, and the last one listed still says what it changed,
// since the one before it is on record too
func TestChatConfigResponseListsTheLatest(t *testing.T) {
	var history []db.ChatConfig
	for id := int64(30); id >= 1; id-- {
		cfg := configHistory()[2]
		cfg.ID, cfg.MaxToolIterations = id, int(id)
		history = append(history, cfg)
	}

	response := chatConfigResponse(history)

	if len(response.History) != chatConfigHistoryLength {
		t.Fatalf("the history lists %d configurations, want %d", len(response.History), chatConfigHistoryLength)
	}
	if last := response.History[len(response.History)-1]; last.First || len(last.Changes) != 1 {
		t.Errorf("the last listed configuration is %+v, want it to say what it changed", last)
	}
}
