package chat

import (
	"math"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/Scrin/siikabot/db"
	"github.com/Scrin/siikabot/metrics"
	"github.com/prometheus/client_golang/prometheus"
)

// seededConfig is the configuration the chat_config table was seeded with
func seededConfig() db.ChatConfig {
	return db.ChatConfig{
		ID:                1,
		CreatedAt:         time.Date(2026, 9, 25, 12, 27, 0, 0, time.UTC),
		Model:             "openrouter/deepseek/deepseek-v4.1-flash",
		ContextHighTokens: 16384,
		ContextLowTokens:  8192,
		MaxToolIterations: 5,
		MaxResponseTokens: 2048,
		ImageDetail:       "low",
		WebPartBytes:      10240,
	}
}

// TestEverySettingIsAColumn checks that the settings and the columns of chat_config match, under the
// same names: a column without a setting would be missing from !chat config and !chat set, and a
// setting without a column would have nowhere to be stored
func TestEverySettingIsAColumn(t *testing.T) {
	columns := make(map[string]bool)
	fields := reflect.TypeFor[db.ChatConfig]()
	for i := range fields.NumField() {
		if column := fields.Field(i).Tag.Get("db"); column != "id" && column != "created_at" {
			columns[column] = true
		}
	}

	keys := make(map[string]bool)
	for _, s := range settings {
		if keys[s.key] {
			t.Errorf("setting %s is declared twice", s.key)
		}
		keys[s.key] = true
		if !columns[s.key] {
			t.Errorf("setting %s has no column", s.key)
		}
	}
	for column := range columns {
		if !keys[column] {
			t.Errorf("column %s has no setting", column)
		}
	}
}

// TestEverySettingIsDescribed checks that every setting says what it is and how to change it, and
// that the example the help gives is a value the setting takes
func TestEverySettingIsDescribed(t *testing.T) {
	groups := make(map[string]bool)
	for _, group := range settingGroups {
		groups[group.name] = true
	}

	for _, s := range settings {
		if !groups[s.group] {
			t.Errorf("setting %s is in group %q, which isn't listed", s.key, s.group)
		}
		if s.summary == "" || s.help == "" || s.example == "" || s.allowed == nil || s.allowed(seededConfig()) == "" {
			t.Errorf("setting %s is missing its summary, help, example or allowed values", s.key)
		}

		cfg := seededConfig()
		if err := s.set(&cfg, s.example); err != nil {
			t.Errorf("the example of %s is refused: %v", s.key, err)
		} else if err := checkConfig(cfg); err != nil {
			t.Errorf("the example of %s makes an invalid configuration: %v", s.key, err)
		}
	}
}

func TestSettingValues(t *testing.T) {
	for _, tt := range []struct {
		key, value string
		// want is the value stored, empty when the value is refused
		want string
	}{
		{"max_tool_iterations", "8", "8"},
		{"max_tool_iterations", "1", "1"},
		{"max_tool_iterations", "30", "30"},
		{"max_tool_iterations", "0", ""},
		{"max_tool_iterations", "31", ""},
		// A plain count takes no multiples
		{"max_tool_iterations", "1k", ""},
		// What fmt.Sscanf used to read as 5, 16, 2048 and 1
		{"max_tool_iterations", "5x", ""},
		{"context_high_tokens", "16x", ""},
		{"max_response_tokens", "2048.5", ""},
		{"context_high_tokens", "1e3", ""},
		{"context_high_tokens", "32768", "32768"},
		{"context_high_tokens", "32k", "32768"},
		{"context_high_tokens", "100K", "102400"},
		{"context_high_tokens", "1M", "1048576"},
		{"context_high_tokens", "2M", ""},
		// A lowercase m would read as minutes as easily as millions
		{"context_high_tokens", "1m", ""},
		{"context_high_tokens", "k", ""},
		{"context_high_tokens", "99999999999999999999k", ""},
		{"context_low_tokens", "-1", ""},
		{"max_response_tokens", "4096", "4096"},
		{"max_response_tokens", "0", ""},
		{"web_part_bytes", "10k", "10240"},
		{"web_part_bytes", "1023", ""},
		{"web_part_bytes", "256k", "262144"},
		{"web_part_bytes", "257k", ""},
		{"image_detail", "high", "high"},
		{"image_detail", "AUTO", "auto"},
		{"image_detail", "medium", ""},
		{"model", "openrouter/openai/gpt-5.6-luna", "openrouter/openai/gpt-5.6-luna"},
	} {
		s, ok := settingByKey(tt.key)
		if !ok {
			t.Fatalf("no setting %s", tt.key)
		}
		cfg := seededConfig()
		err := s.set(&cfg, tt.value)
		switch {
		case tt.want == "" && err == nil:
			t.Errorf("%s %q was taken as %s, want it refused", tt.key, tt.value, s.show(cfg))
		case tt.want != "" && err != nil:
			t.Errorf("%s %q was refused: %v", tt.key, tt.value, err)
		case tt.want != "" && s.show(cfg) != tt.want:
			t.Errorf("%s %q was taken as %s, want %s", tt.key, tt.value, s.show(cfg), tt.want)
		}
		if err != nil && !strings.Contains(err.Error(), "`"+tt.key+"`") {
			t.Errorf("refusing %s %q doesn't name the setting: %v", tt.key, tt.value, err)
		}
	}
}

// TestContextMarksAreCheckedTogether verifies that the marks can be moved together in one command,
// in either order, even when either one alone would cross the other
func TestContextMarksAreCheckedTogether(t *testing.T) {
	for _, words := range [][]string{
		{"context_high_tokens", "4k", "context_low_tokens", "2k"},
		{"context_low_tokens", "2k", "context_high_tokens", "4k"},
		{"context_low_tokens", "64k", "context_high_tokens", "128k"},
	} {
		changes, err := parseChanges(words)
		if err != nil {
			t.Fatalf("%v: %v", words, err)
		}
		cfg := seededConfig()
		if err := applyChanges(&cfg, changes); err != nil {
			t.Errorf("%v was refused: %v", words, err)
		}
	}

	for _, words := range [][]string{
		{"context_high_tokens", "4k"},
		{"context_low_tokens", "16k"},
		{"context_high_tokens", "8k", "context_low_tokens", "8k"},
	} {
		changes, _ := parseChanges(words)
		cfg := seededConfig()
		err := applyChanges(&cfg, changes)
		if err == nil {
			t.Errorf("%v was taken, want it refused", words)
			continue
		}
		if !strings.Contains(err.Error(), "`context_low_tokens`") || !strings.Contains(err.Error(), "`context_high_tokens`") {
			t.Errorf("refusing %v doesn't name both marks: %v", words, err)
		}
	}
}

func TestParseChanges(t *testing.T) {
	for _, tt := range []struct {
		words string
		// count is how many changes are read, and refusal part of the answer when they aren't
		count   int
		refusal string
	}{
		{"model openrouter/openai/gpt-5.6-luna", 1, ""},
		{"context_high_tokens 32k context_low_tokens 16k", 2, ""},
		{"CONTEXT_HIGH_TOKENS 32k", 1, ""},
		{"", 0, "!chat set <key> <value>"},
		{"model", 0, "`model` has no value"},
		{"model x max_tool_iterations", 0, "`max_tool_iterations` has no value"},
		{"max_tool_iterations 5 max_tool_iterations 6", 0, "given twice"},
		// The old form of !chat model, which named the kind of model first
		{"model text openrouter/deepseek/deepseek-v4.1-flash", 0, "There's no setting `openrouter/deepseek/deepseek-v4.1-flash`"},
		{"context_hi 32k", 0, "Did you mean `context_high_tokens`?"},
	} {
		changes, err := parseChanges(strings.Fields(tt.words))
		if tt.refusal == "" {
			if err != nil || len(changes) != tt.count {
				t.Errorf("parseChanges(%q) = %d changes, %v, want %d", tt.words, len(changes), err, tt.count)
			}
			continue
		}
		if err == nil || !strings.Contains(err.Error(), tt.refusal) {
			t.Errorf("parseChanges(%q) refused with %v, want %q", tt.words, err, tt.refusal)
		}
	}
}

func TestSimilarSettings(t *testing.T) {
	for given, want := range map[string]string{
		"context_hi":       "context_high_tokens",
		"Context-High":     "context_high_tokens",
		"context":          "context_high_tokens context_low_tokens",
		"tokens":           "max_response_tokens context_high_tokens context_low_tokens",
		"imagedetail":      "image_detail",
		"iterations":       "max_tool_iterations",
		"model_name":       "model",
		"web":              "web_part_bytes",
		"xy":               "",
		"temperature":      "",
		"max_tokens_count": "",
	} {
		if got := strings.Join(similarSettings(given), " "); got != want {
			t.Errorf("similarSettings(%q) = %q, want %q", given, got, want)
		}
	}

	// With nothing similar, the answer lists every setting there is
	refusal := unknownSetting("temperature").Error()
	for _, s := range settings {
		if !strings.Contains(refusal, "`"+s.key+"`") {
			t.Errorf("the answer to an unknown setting doesn't list %s: %s", s.key, refusal)
		}
	}
}

func TestDescribeConfig(t *testing.T) {
	cfg := seededConfig()
	cfg.ID = 27
	listing := describeConfig(cfg, "This thread now: ~5210 tokens in 14 messages and 1 image")

	for _, want := range []string{
		"**Chat configuration #27**, set 2026-09-25 12:27",
		"- `model`: openrouter/deepseek/deepseek-v4.1-flash — ",
		"- `max_response_tokens`: 2048 tokens — ",
		"- `context_high_tokens`: 16384 tokens — ",
		"- `context_low_tokens`: 8192 tokens — ",
		"- `max_tool_iterations`: 5 — ",
		"- `web_part_bytes`: 10240 bytes — ",
		"- `image_detail`: low — ",
		"`!chat set <key> <value> [<key> <value> …]`",
		"`!chat config <key>`",
	} {
		if !strings.Contains(listing, want) {
			t.Errorf("the listing doesn't have %q:\n%s", want, listing)
		}
	}

	// The window's fill is listed with the marks it is measured against
	window := strings.Index(listing, "This thread now")
	if window < strings.Index(listing, "`context_low_tokens`") || window > strings.Index(listing, "**Tools**") {
		t.Errorf("the window's fill isn't listed with the context window settings:\n%s", listing)
	}
}

func TestDescribeSetting(t *testing.T) {
	first := seededConfig()
	second := first
	second.ID, second.CreatedAt, second.ContextLowTokens = 2, first.CreatedAt.Add(time.Hour), 4096
	third := second
	third.ID, third.CreatedAt, third.Model = 3, second.CreatedAt.Add(time.Hour), "openrouter/openai/gpt-5.6-luna"
	history := []db.ChatConfig{third, second, first}

	low, _ := settingByKey("context_low_tokens")
	help := describeSetting(low, history)
	for _, want := range []string{
		"**`context_low_tokens`**: 4096 tokens, since #2 on 2026-09-25 13:27",
		"below `context_high_tokens` (16384)",
		"`!chat set context_low_tokens 4k`",
	} {
		if !strings.Contains(help, want) {
			t.Errorf("the help doesn't have %q:\n%s", want, help)
		}
	}

	for key, since := range map[string]*db.ChatConfig{"model": &third, "image_detail": &first} {
		s, _ := settingByKey(key)
		if got := lastChange(s, history); got.ID != since.ID {
			t.Errorf("%s last changed in #%d, want #%d", key, got.ID, since.ID)
		}
	}
}

func TestDescribeChanges(t *testing.T) {
	before := seededConfig()
	after := before
	after.ID, after.MaxToolIterations, after.ContextHighTokens = 28, 12, 32768

	if got, want := describeChanges(before, after),
		"Changed (#28): `context_high_tokens` 16384 → 32768 tokens, `max_tool_iterations` 5 → 12"; got != want {
		t.Errorf("describeChanges() = %q, want %q", got, want)
	}

	changes, _ := parseChanges([]string{"max_tool_iterations", "5"})
	if got, want := unchanged(before, changes).Error(), "No change: `max_tool_iterations` is already 5"; got != want {
		t.Errorf("unchanged() = %q, want %q", got, want)
	}
}

// TestMetricBucketsCoverTheSettings checks that the histograms of what the settings bound reach the
// top of each setting's range, so that every value a setting allows lands in a bucket of its own
// rather than in the overflow
func TestMetricBucketsCoverTheSettings(t *testing.T) {
	// A histogram with labels has nothing to gather until it has a value
	metrics.RecordChatToolIterations(0)
	metrics.RecordChatContextWindowTokens("test", 0)
	metrics.RecordChatPromptComponent(componentHistory, 0)

	families, err := prometheus.DefaultGatherer.Gather()
	if err != nil {
		t.Fatalf("failed to gather the metrics: %v", err)
	}
	tops := make(map[string]float64)
	for _, family := range families {
		for _, metric := range family.GetMetric() {
			for _, bucket := range metric.GetHistogram().GetBucket() {
				if bound := bucket.GetUpperBound(); !math.IsInf(bound, 1) {
					tops[family.GetName()] = max(tops[family.GetName()], bound)
				}
			}
		}
	}

	for name, bound := range map[string]int{
		"siikabot_chat_tool_iterations":         maxToolIterationsBound,
		"siikabot_chat_context_window_tokens":   maxContextTokensBound,
		"siikabot_chat_prompt_component_tokens": maxContextTokensBound,
	} {
		if top, ok := tops[name]; !ok || top < float64(bound) {
			t.Errorf("the buckets of %s reach %v, short of the bound of %d", name, top, bound)
		}
	}
}
