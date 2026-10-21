package chat

import (
	"math"
	"reflect"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/Scrin/siikabot/aigateway"
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
		TurnTimeout:       3 * time.Minute,
		ModelTimeout:      45 * time.Second,
		ModelAttempts:     3,
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
		{"turn_timeout", "10m", "10m"},
		{"turn_timeout", "90s", "1m30s"},
		{"turn_timeout", "20m", "20m"},
		{"turn_timeout", "25m", ""},
		{"turn_timeout", "20s", ""},
		{"turn_timeout", "-3m", ""},
		{"turn_timeout", "3", ""},
		{"turn_timeout", "3M", ""},
		{"model_timeout", "2m", "2m"},
		{"model_timeout", "10m", "10m"},
		{"model_timeout", "11m", ""},
		{"model_timeout", "5s", ""},
		{"model_attempts", "5", "5"},
		{"model_attempts", "6", ""},
		{"model_attempts", "0", ""},
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
		"- `turn_timeout`: 3m — ",
		"- `model_timeout`: 45s — ",
		"- `model_attempts`: 3 — ",
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
	if window < strings.Index(listing, "`context_low_tokens`") || window > strings.Index(listing, "**Time**") {
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
	metrics.RecordChatRequestDuration("test", 0)
	metrics.RecordChatTurnPhase("test", phaseModel, 0)
	metrics.RecordChatAPICallDuration("test", 0)

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

	for name, bound := range map[string]float64{
		"siikabot_chat_tool_iterations":             maxToolIterationsBound,
		"siikabot_chat_context_window_tokens":       maxContextTokensBound,
		"siikabot_chat_prompt_component_tokens":     maxContextTokensBound,
		"siikabot_chat_request_duration_seconds":    maxTurnTimeout.Seconds(),
		"siikabot_chat_turn_phase_duration_seconds": maxTurnTimeout.Seconds(),
		"siikabot_chat_api_call_duration_seconds":   aigateway.MaxCallTimeout.Seconds(),
	} {
		if top, ok := tops[name]; !ok || top < bound {
			t.Errorf("the buckets of %s reach %v, short of the bound of %v", name, top, bound)
		}
	}
}

// TestModelCallHasToFitInTheTurn verifies a model call with every attempt it may get, and the waits
// between them, has to fit in the turn, whichever of the three settings a command changes. The advice
// given with a refusal has to work as it stands.
func TestModelCallHasToFitInTheTurn(t *testing.T) {
	for _, words := range []string{
		"model_timeout 2m turn_timeout 10m",
		"model_attempts 1 model_timeout 10m turn_timeout 11m",
		"turn_timeout 2m30s",
	} {
		changes, err := parseChanges(strings.Fields(words))
		if err != nil {
			t.Fatalf("%s: %v", words, err)
		}
		cfg := seededConfig()
		if err := applyChanges(&cfg, changes); err != nil {
			t.Errorf("%s was refused: %v", words, err)
		}
	}

	for _, tt := range []struct {
		words string
		// advice is the command the refusal gives, empty for none
		advice string
	}{
		{"model_timeout 4m", "!chat set model_timeout 4m model_attempts 3 turn_timeout 13m"},
		{"turn_timeout 2m", "!chat set model_timeout 45s model_attempts 3 turn_timeout 3m"},
		{"model_attempts 5", "!chat set model_timeout 45s model_attempts 5 turn_timeout 4m"},
		// Exactly as long as the worst case isn't enough: the turn has to have more
		{"model_attempts 1 turn_timeout 45s", "!chat set model_timeout 45s model_attempts 1 turn_timeout 1m"},
		// Longer than any turn may be, so only lowering the others can make it fit
		{"model_timeout 10m", ""},
	} {
		changes, err := parseChanges(strings.Fields(tt.words))
		if err != nil {
			t.Fatalf("%s: %v", tt.words, err)
		}
		cfg := seededConfig()
		err = applyChanges(&cfg, changes)
		if err == nil {
			t.Errorf("%s was taken, want it refused", tt.words)
			continue
		}
		refusal := err.Error()
		if !strings.Contains(refusal, "`turn_timeout`") || !strings.Contains(refusal, "`model_timeout`") {
			t.Errorf("refusing %s doesn't name the settings involved: %s", tt.words, refusal)
		}
		if tt.advice == "" {
			if strings.Contains(refusal, "!chat set") {
				t.Errorf("refusing %s advises a command no turn could take: %s", tt.words, refusal)
			}
			continue
		}
		if !strings.Contains(refusal, "`"+tt.advice+"`") {
			t.Errorf("refusing %s advises something else: %s, want `%s`", tt.words, refusal, tt.advice)
			continue
		}
		advised, err := parseChanges(strings.Fields(strings.TrimPrefix(tt.advice, "!chat set ")))
		if err != nil {
			t.Fatalf("the advice %q doesn't parse: %v", tt.advice, err)
		}
		fixed := seededConfig()
		if err := applyChanges(&fixed, advised); err != nil {
			t.Errorf("the advice %q is refused in turn: %v", tt.advice, err)
		}
	}
}

// TestHelpExamplesWork verifies every command the help texts give as an example is one !chat set
// takes, so the help doesn't tell anyone to run a command that would be refused
func TestHelpExamplesWork(t *testing.T) {
	examples := regexp.MustCompile("`!chat set ([^`<]+)`")
	found := 0
	for _, s := range settings {
		for _, example := range examples.FindAllStringSubmatch(s.help, -1) {
			found++
			changes, err := parseChanges(strings.Fields(example[1]))
			if err != nil {
				t.Errorf("the help of %s gives %q, which doesn't parse: %v", s.key, example[0], err)
				continue
			}
			cfg := seededConfig()
			if err := applyChanges(&cfg, changes); err != nil {
				t.Errorf("the help of %s gives %q, which is refused: %v", s.key, example[0], err)
			}
		}
	}
	if found == 0 {
		t.Error("found no examples in the help texts")
	}
}

func TestFormatDuration(t *testing.T) {
	for d, want := range map[time.Duration]string{
		3 * time.Minute:                "3m",
		20 * time.Minute:               "20m",
		45 * time.Second:               "45s",
		10 * time.Second:               "10s",
		90 * time.Second:               "1m30s",
		time.Hour:                      "1h",
		time.Hour + 30*time.Minute:     "1h30m",
		2250 * time.Millisecond:        "2.25s",
		2*time.Minute + 17*time.Second: "2m17s",
	} {
		if got := formatDuration(d); got != want {
			t.Errorf("formatDuration(%s) = %q, want %q", d, got, want)
		}
		// What the listing shows, !chat set takes back
		if parsed, err := time.ParseDuration(formatDuration(d)); err != nil || parsed != d {
			t.Errorf("formatDuration(%s) = %q, which reads back as %s, %v", d, formatDuration(d), parsed, err)
		}
	}
}

// TestRedactionMemoryOutlastsTurns ties how long a redaction is remembered to how long turns may take.
// A turn still running, or queued behind others, when the redaction of its trigger is forgotten would
// write the redacted message back into the history, which the chat privacy invariant forbids.
func TestRedactionMemoryOutlastsTurns(t *testing.T) {
	if redactionMemory < 10*maxTurnTimeout {
		t.Errorf("a redaction is remembered for %s, less than ten turns of the longest %s allowed",
			redactionMemory, maxTurnTimeout)
	}
}

// configHistory is a history of three configurations, newest first: the seeded one, one that
// lengthened the turns, and one that changed the model and the rounds of tool calls
func configHistory() []db.ChatConfig {
	first := seededConfig()
	second := first
	second.ID, second.CreatedAt, second.TurnTimeout = 2, first.CreatedAt.Add(time.Hour), 10*time.Minute
	third := second
	third.ID, third.CreatedAt, third.Model, third.MaxToolIterations = 3, second.CreatedAt.Add(time.Hour), "openrouter/openai/gpt-5.6-luna", 8
	return []db.ChatConfig{third, second, first}
}

func TestDescribeHistory(t *testing.T) {
	listing := describeHistory(configHistory(), defaultHistoryLength)
	for _, want := range []string{
		"- #3, 2026-09-25 14:27: `model` openrouter/deepseek/deepseek-v4.1-flash → openrouter/openai/gpt-5.6-luna, `max_tool_iterations` 5 → 8\n",
		"- #2, 2026-09-25 13:27: `turn_timeout` 3m → 10m\n",
		"- #1, 2026-09-25 12:27: the first configuration\n",
		"`!chat config revert <id>`",
	} {
		if !strings.Contains(listing, want) {
			t.Errorf("the history doesn't have %q:\n%s", want, listing)
		}
	}
	if strings.Contains(listing, "earlier") {
		t.Errorf("the history says there's more when it lists everything:\n%s", listing)
	}

	// Cut short, the last configuration listed still says what it changed, and the history says how
	// to see the rest
	short := describeHistory(configHistory(), 2)
	if strings.Contains(short, "#1") || !strings.Contains(short, "`turn_timeout` 3m → 10m") ||
		!strings.Contains(short, "1 earlier configuration, which `!chat config history 3` lists") {
		t.Errorf("the history of the latest two:\n%s", short)
	}
}

// TestApplyRevert verifies going back to an earlier configuration takes every one of its settings,
// and only if they pass today's rules: a bound tightened since then refuses it
func TestApplyRevert(t *testing.T) {
	history := configHistory()

	cfg := history[0]
	if err := applyRevert(&cfg, history[2]); err != nil {
		t.Fatalf("going back to #1 was refused: %v", err)
	}
	if changed := changedSettings(history[2], cfg); len(changed) != 0 {
		t.Errorf("going back to #1 left %d settings as they were", len(changed))
	}

	tooMany := history[2]
	tooMany.ID, tooMany.MaxToolIterations = 9, 50
	cfg = history[0]
	err := applyRevert(&cfg, tooMany)
	if err == nil || !strings.Contains(err.Error(), "Can't go back to #9") || !strings.Contains(err.Error(), "`max_tool_iterations` takes 1 to 30") {
		t.Errorf("going back to a configuration over today's bounds: %v", err)
	}

	tooSlow := history[2]
	tooSlow.ID, tooSlow.ModelTimeout = 9, 2*time.Minute
	cfg = history[0]
	if err := applyRevert(&cfg, tooSlow); err == nil || !strings.Contains(err.Error(), "doesn't fit in `turn_timeout`") {
		t.Errorf("going back to a configuration whose model call doesn't fit its turn: %v", err)
	}
}

// TestSettingsCommandsWork verifies every command the dashboard shows is one !chat set takes
func TestSettingsCommandsWork(t *testing.T) {
	infos := Settings(seededConfig())
	if len(infos) != len(settings) {
		t.Fatalf("Settings() describes %d settings, want all %d", len(infos), len(settings))
	}
	for _, info := range infos {
		changes, err := parseChanges(strings.Fields(strings.TrimPrefix(info.Command, "!chat set ")))
		if err != nil {
			t.Errorf("the command for %s, %q, doesn't parse: %v", info.Key, info.Command, err)
			continue
		}
		cfg := seededConfig()
		if err := applyChanges(&cfg, changes); err != nil {
			t.Errorf("the command for %s, %q, is refused: %v", info.Key, info.Command, err)
		}
	}
}

func TestChatHelpListsEverySubcommand(t *testing.T) {
	for _, subcommand := range []string{"`!chat config`", "`!chat config <key>`", "`!chat config history [n]`",
		"`!chat set <key>", "`!chat config revert <id>`", "`!chat reset`"} {
		if !strings.Contains(chatHelp, subcommand) {
			t.Errorf("the help doesn't list %s", subcommand)
		}
	}
}
