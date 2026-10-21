package chat

import (
	"fmt"
	"math"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/Scrin/siikabot/aigateway"
	"github.com/Scrin/siikabot/db"
	"github.com/rs/zerolog"
)

// The chat settings, as !chat config shows them and !chat set changes them. Each one is declared once
// here, under the key it goes by everywhere: in the commands, as its column of chat_config, and in the
// logs. The listing, the help, the parsing and the checks all come from these declarations, so a
// setting can't be added without saying what it means and how to change it.

// setting is one chat setting: the key it goes by, what it means, and how a value for it is read,
// checked and shown
type setting struct {
	key   string
	group string
	// unit follows the value wherever it is shown: tokens or bytes, or nothing for a count or a name
	unit string
	// summary is the one line the listing gives the setting, and help its full explanation
	summary string
	help    string
	// allowed says which values the setting takes, which may depend on the rest of the configuration
	allowed func(db.ChatConfig) string
	// example is a value for the example command in the help. It is valid with the configuration the
	// table was seeded with.
	example string
	// show renders the setting's value in a configuration, without its unit
	show func(db.ChatConfig) string
	// set parses a value into a configuration, checking the bounds the setting has on its own. The
	// rules that tie settings to each other are in checkConfig.
	set func(cfg *db.ChatConfig, value string) error
}

// settingGroup is a heading settings are listed under, with a note on how they work together
type settingGroup struct {
	name string
	note string
}

// windowGroup is the group the listing says how full the current context window is under
const windowGroup = "Context window"

// settingGroups are listed in this order
var settingGroups = []settingGroup{
	{name: "Model"},
	{name: windowGroup, note: "The history of a room, and of each of its threads, grows to the high mark. " +
		"Its oldest turns are then dropped until it's under the low mark."},
	{name: "Time"},
	{name: "Tools"},
	{name: "Images"},
}

// The upper bounds of the settings that have them. They guard against typos rather than set what the
// bot can run with, which the table's checks do, so they are generous: each lets through any value
// that could make sense.
const (
	maxResponseTokensBound = 128 << 10 // 131072
	maxContextTokensBound  = 1 << 20   // 1048576, for the models with the largest context windows
	maxToolIterationsBound = 30
	minWebPartBytes        = 1 << 10   // 1024
	maxWebPartBytes        = 256 << 10 // 262144
	maxModelAttemptsBound  = 5
)

// The bounds of the settings of time. The most model_timeout can be is aigateway.MaxCallTimeout,
// which the client's own backstop is set above.
const (
	minTurnTimeout  = 30 * time.Second
	maxTurnTimeout  = 20 * time.Minute
	minModelTimeout = 10 * time.Second
)

// imageDetails are the image details a configuration can have
var imageDetails = []string{"low", "high", imageDetailAuto}

// settings are listed, and their changes reported, in this order
var settings = []setting{
	{
		key:     "model",
		group:   "Model",
		summary: "the model that answers every turn",
		help: "The model that answers every turn, named as the AI Gateway knows it. It has to take images as well as " +
			"text: an image stays in the conversation after the turn it was sent in, so any turn may show the model one.",
		allowed: func(db.ChatConfig) string { return "a model name without spaces, of a model that takes images" },
		example: "<name>",
		show:    func(cfg db.ChatConfig) string { return cfg.Model },
		set: func(cfg *db.ChatConfig, value string) error {
			if value == "" {
				return settingError("`model` takes a model name")
			}
			cfg.Model = value
			return nil
		},
	},
	{
		key:     "max_response_tokens",
		group:   "Model",
		unit:    "tokens",
		summary: "the longest answer one model call may write",
		help: "Caps the length of one answer, so that a runaway generation isn't billed in full. It's generous " +
			"enough that ordinary answers aren't affected, since the system prompt already asks for short ones. A " +
			"model that reasons counts its reasoning against it too, and one that spends all of it reasoning gives " +
			"no answer at all.",
		allowed: func(db.ChatConfig) string { return fmt.Sprintf("1 to %d tokens", maxResponseTokensBound) },
		example: "4096",
		show:    func(cfg db.ChatConfig) string { return strconv.Itoa(cfg.MaxResponseTokens) },
		set: func(cfg *db.ChatConfig, value string) error {
			return setCount(&cfg.MaxResponseTokens, "max_response_tokens", "tokens", value, 1, maxResponseTokensBound)
		},
	},
	{
		key:     "context_high_tokens",
		group:   windowGroup,
		unit:    "tokens",
		summary: "the most history the window holds before it's trimmed",
		help: "How many tokens of history a context window holds before it's trimmed. A room has one window, and " +
			"each of its threads one of its own. When the history grows past this, its oldest turns are dropped " +
			"until it's under context_low_tokens. A tool result counts as the model sees it: cut short, or as a " +
			"note that it has expired.\n\n" +
			"The window, the system prompt, the tool definitions and max_response_tokens all have to fit in the " +
			"model's own context, which the bot can't check.",
		allowed: func(cfg db.ChatConfig) string {
			return fmt.Sprintf("above `context_low_tokens` (%d), up to %d tokens", cfg.ContextLowTokens, maxContextTokensBound)
		},
		example: "32k",
		show:    func(cfg db.ChatConfig) string { return strconv.Itoa(cfg.ContextHighTokens) },
		set: func(cfg *db.ChatConfig, value string) error {
			return setCount(&cfg.ContextHighTokens, "context_high_tokens", "tokens", value, 1, maxContextTokensBound)
		},
	},
	{
		key:     "context_low_tokens",
		group:   windowGroup,
		unit:    "tokens",
		summary: "what a trimmed window is brought under",
		help: "When the history grows past context_high_tokens, its oldest turns are dropped until it's under this. " +
			"Because of the gap between the two marks, the window is trimmed now and then rather than on every " +
			"turn. In between, the start of the prompt stays the same from one turn to the next, so the " +
			"provider's prompt cache keeps working.\n\n" +
			"To change both marks at once: `!chat set context_high_tokens 32k context_low_tokens 16k`",
		allowed: func(cfg db.ChatConfig) string {
			return fmt.Sprintf("at least 1, and below `context_high_tokens` (%d)", cfg.ContextHighTokens)
		},
		example: "4k",
		show:    func(cfg db.ChatConfig) string { return strconv.Itoa(cfg.ContextLowTokens) },
		set: func(cfg *db.ChatConfig, value string) error {
			return setCount(&cfg.ContextLowTokens, "context_low_tokens", "tokens", value, 1, maxContextTokensBound)
		},
	},
	{
		key:     "turn_timeout",
		group:   "Time",
		summary: "the longest one turn may take, tools included",
		help: "How long one turn may take, from starting on a message to its answer, with every model call and tool " +
			"call in between. A turn that runs out of time stops, and the room is told it took too long.\n\n" +
			"Turns run one at a time in each room, so a turn that gets stuck holds up the messages after it for this " +
			"long. More rounds of tool calls (max_tool_iterations) and slower models need more time. A model call " +
			"with every attempt it may get has to fit in it: model_attempts attempts of model_timeout, and the " +
			"waits between them.",
		allowed: func(cfg db.ChatConfig) string {
			return fmt.Sprintf("%s to %s, and longer than a model call can take with every attempt (%s)",
				formatDuration(minTurnTimeout), formatDuration(maxTurnTimeout),
				formatDuration(roundUpToSecond(aigateway.WorstCaseCall(cfg.ModelTimeout, cfg.ModelAttempts))))
		},
		example: "10m",
		show:    func(cfg db.ChatConfig) string { return formatDuration(cfg.TurnTimeout) },
		set: func(cfg *db.ChatConfig, value string) error {
			return setDuration(&cfg.TurnTimeout, "turn_timeout", value, minTurnTimeout, maxTurnTimeout)
		},
	},
	{
		key:     "model_timeout",
		group:   "Time",
		summary: "the longest one attempt at a model call may take",
		help: "How long one attempt at a model call may take. The AI Gateway is asked to give up at the same point. A " +
			"slow model, or one that reasons at length, needs more. An attempt that runs out of time is made again, " +
			"up to model_attempts times, and all of the attempts have to fit in turn_timeout.\n\n" +
			"To give a slow model more time: `!chat set model_timeout 2m turn_timeout 10m`",
		allowed: func(cfg db.ChatConfig) string {
			return fmt.Sprintf("%s to %s, with %d attempts of it fitting in `turn_timeout` (%s)",
				formatDuration(minModelTimeout), formatDuration(aigateway.MaxCallTimeout), cfg.ModelAttempts,
				formatDuration(cfg.TurnTimeout))
		},
		example: "30s",
		show:    func(cfg db.ChatConfig) string { return formatDuration(cfg.ModelTimeout) },
		set: func(cfg *db.ChatConfig, value string) error {
			return setDuration(&cfg.ModelTimeout, "model_timeout", value, minModelTimeout, aigateway.MaxCallTimeout)
		},
	},
	{
		key:     "model_attempts",
		group:   "Time",
		summary: "the attempts a model call gets when it fails for a passing reason",
		help: "How many times a model call is tried when it fails for a reason that may pass: the network, a timeout, " +
			"a rate limit, or the provider failing. A request the provider rejects isn't tried again, since it would " +
			"fail the same way. The wait before each retry starts at about half a second and doubles, up to a few " +
			"seconds, and all of the attempts have to fit in turn_timeout. The AI Gateway also retries a failed " +
			"request once by itself, before the bot sees the failure.",
		allowed: func(db.ChatConfig) string { return fmt.Sprintf("1 to %d", maxModelAttemptsBound) },
		example: "2",
		show:    func(cfg db.ChatConfig) string { return strconv.Itoa(cfg.ModelAttempts) },
		set: func(cfg *db.ChatConfig, value string) error {
			return setCount(&cfg.ModelAttempts, "model_attempts", "", value, 1, maxModelAttemptsBound)
		},
	},
	{
		key:     "max_tool_iterations",
		group:   "Tools",
		summary: "the rounds of tool calls one turn may make",
		help: "How many rounds of tool calls one turn may make. Each round is another model call carrying the whole " +
			"conversation, so more rounds cost more and take longer. Once they're used up, the model answers with " +
			"what it has found so far.",
		allowed: func(db.ChatConfig) string { return fmt.Sprintf("1 to %d", maxToolIterationsBound) },
		example: "8",
		show:    func(cfg db.ChatConfig) string { return strconv.Itoa(cfg.MaxToolIterations) },
		set: func(cfg *db.ChatConfig, value string) error {
			return setCount(&cfg.MaxToolIterations, "max_tool_iterations", "", value, 1, maxToolIterationsBound)
		},
	},
	{
		key:     "web_part_bytes",
		group:   "Tools",
		unit:    "bytes",
		summary: "the page text one get_web_content call returns",
		help: "How much of a page one get_web_content call returns. A longer page comes in parts, and the model " +
			"asks for the next part when it needs it.",
		allowed: func(db.ChatConfig) string { return fmt.Sprintf("%d to %d bytes", minWebPartBytes, maxWebPartBytes) },
		example: "16k",
		show:    func(cfg db.ChatConfig) string { return strconv.Itoa(cfg.WebPartBytes) },
		set: func(cfg *db.ChatConfig, value string) error {
			return setCount(&cfg.WebPartBytes, "web_part_bytes", "bytes", value, minWebPartBytes, maxWebPartBytes)
		},
	},
	{
		key:     "image_detail",
		group:   "Images",
		summary: "how closely images are shown to the model",
		help: "How closely images are shown to the model. low costs a small, flat number of tokens per image, where " +
			"the provider's own default tiles the image and can cost thousands of tokens for the same picture. high " +
			"shows more of an image, and auto leaves the choice to the provider.",
		allowed: func(db.ChatConfig) string { return "low, high or auto" },
		example: "high",
		show:    func(cfg db.ChatConfig) string { return cfg.ImageDetail },
		set: func(cfg *db.ChatConfig, value string) error {
			detail := strings.ToLower(value)
			if !slices.Contains(imageDetails, detail) {
				return settingErrorf("`image_detail` takes low, high or auto, not `%s`", value)
			}
			cfg.ImageDetail = detail
			return nil
		},
	},
}

// checkConfig checks the rules that tie settings to each other. It runs on a configuration with every
// change of one !chat set applied, so that settings that depend on each other can be changed together,
// in either order.
func checkConfig(cfg db.ChatConfig) error {
	// The window grows to the high mark and is then trimmed back to the low mark, so the marks have to
	// straddle a usable range or the window would be trimmed on every turn
	if cfg.ContextLowTokens >= cfg.ContextHighTokens {
		return settingErrorf("`context_low_tokens` (%d) has to be below `context_high_tokens` (%d). To change both "+
			"at once: `!chat set context_high_tokens 32k context_low_tokens 16k`", cfg.ContextLowTokens, cfg.ContextHighTokens)
	}

	// A model call with every attempt it may get, and the waits between them, has to fit in the turn,
	// or the retries meant to carry a call over a passing failure would never get to run. The advice
	// gives all three settings, so that it works as it stands whichever of them the command changed.
	if call := aigateway.WorstCaseCall(cfg.ModelTimeout, cfg.ModelAttempts); call >= cfg.TurnTimeout {
		shown := roundUpToSecond(call)
		advice := "Lower `model_timeout` or `model_attempts`"
		if fits := shown.Truncate(time.Minute) + time.Minute; fits <= maxTurnTimeout {
			advice = fmt.Sprintf("Change them together, such as `!chat set model_timeout %s model_attempts %d turn_timeout %s`, "+
				"or lower `model_timeout` or `model_attempts`", formatDuration(cfg.ModelTimeout), cfg.ModelAttempts, formatDuration(fits))
		}
		return settingErrorf("A model call can take up to %s: %d attempts of `model_timeout` (%s), and the waits between "+
			"them. That doesn't fit in `turn_timeout` (%s). %s.", formatDuration(shown), cfg.ModelAttempts,
			formatDuration(cfg.ModelTimeout), formatDuration(cfg.TurnTimeout), advice)
	}
	return nil
}

// settingError is why a change to the configuration isn't made, in words for the room. It tells such
// a refusal apart from a failure of the database.
type settingError string

func (e settingError) Error() string { return string(e) }

// settingErrorf formats a settingError
func settingErrorf(format string, args ...any) error {
	return settingError(fmt.Sprintf(format, args...))
}

// setCount parses a whole number into field for the setting key, taking it only from least to most.
// Counts of tokens and bytes, which have a unit, may be written with k or M, multiples of 1024, as
// budgets usually are: 16k is 16384.
func setCount(field *int, key, unit, value string, least, most int) error {
	count, ok := parseCount(value, unit != "")
	if !ok {
		if unit == "" {
			return settingErrorf("`%s` takes a whole number, not `%s`", key, value)
		}
		return settingErrorf("`%s` takes a whole number of %s, such as 16384 or 16k, not `%s`", key, unit, value)
	}
	if count < least || count > most {
		return settingErrorf("`%s` takes %d to %d%s, not %d", key, least, most, withUnit("", unit), count)
	}
	*field = count
	return nil
}

// parseCount reads a whole number. With multiples, it may end in k or M, multiples of 1024.
func parseCount(value string, multiples bool) (int, bool) {
	multiplier := int64(1)
	if multiples {
		switch {
		case strings.HasSuffix(value, "k"), strings.HasSuffix(value, "K"):
			multiplier, value = 1<<10, value[:len(value)-1]
		case strings.HasSuffix(value, "M"):
			multiplier, value = 1<<20, value[:len(value)-1]
		}
	}
	count, err := strconv.ParseInt(value, 10, 64)
	if err != nil || count > math.MaxInt32/multiplier || count < math.MinInt32/multiplier {
		return 0, false
	}
	return int(count * multiplier), true
}

// setDuration parses a duration, such as 90s, 3m or 1m30s, into field for the setting key, taking it
// only from least to most
func setDuration(field *time.Duration, key, value string, least, most time.Duration) error {
	duration, err := time.ParseDuration(value)
	if err != nil {
		return settingErrorf("`%s` takes a duration, such as 90s, 3m or 1m30s, not `%s`", key, value)
	}
	if duration < least || duration > most {
		return settingErrorf("`%s` takes %s to %s, not %s", key, formatDuration(least), formatDuration(most),
			formatDuration(duration))
	}
	*field = duration
	return nil
}

// formatDuration renders a duration the way !chat set takes it: 3m rather than Go's 3m0s
func formatDuration(d time.Duration) string {
	s := d.String()
	if strings.HasSuffix(s, "m0s") {
		s = strings.TrimSuffix(s, "0s")
	}
	if strings.HasSuffix(s, "h0m") {
		s = strings.TrimSuffix(s, "0m")
	}
	return s
}

// roundUpToSecond rounds a duration up to a whole second, so that what is shown of it is never less
func roundUpToSecond(d time.Duration) time.Duration {
	if rounded := d.Truncate(time.Second); rounded < d {
		return rounded + time.Second
	}
	return d
}

// withUnit joins a value and its unit, if it has one
func withUnit(value, unit string) string {
	if unit == "" {
		return value
	}
	return value + " " + unit
}

// value renders the setting's value in a configuration, with its unit
func (s setting) value(cfg db.ChatConfig) string {
	return withUnit(s.show(cfg), s.unit)
}

// settingByKey returns the setting a key names, in any case
func settingByKey(key string) (setting, bool) {
	key = strings.ToLower(key)
	i := slices.IndexFunc(settings, func(s setting) bool { return s.key == key })
	if i < 0 {
		return setting{}, false
	}
	return settings[i], true
}

// unknownSetting is the answer to a key that names no setting: the settings it could have meant, or
// else every setting there is
func unknownSetting(key string) error {
	if similar := similarSettings(key); len(similar) > 0 {
		return settingErrorf("There's no setting `%s`. Did you mean %s?", key, joinCodes(similar, "or"))
	}
	keys := make([]string, 0, len(settings))
	for _, s := range settings {
		keys = append(keys, s.key)
	}
	return settingErrorf("There's no setting `%s`. The settings are %s.", key, joinCodes(keys, "and"))
}

// similarSettings returns the keys of the settings a mistyped key could have meant: those that hold
// it, or that it holds, ignoring case and underscores. A key of a letter or two is only matched as a
// whole, since nearly every key holds one.
func similarSettings(key string) []string {
	given := normalizeKey(key)
	var similar []string
	for _, s := range settings {
		known := normalizeKey(s.key)
		if (len(given) >= 3 && strings.Contains(known, given)) || strings.Contains(given, known) {
			similar = append(similar, s.key)
		}
	}
	return similar
}

// normalizeKey drops what a key may be mistyped in without becoming a different key: its case, and
// the characters between its words
func normalizeKey(key string) string {
	return strings.ToLower(strings.NewReplacer("_", "", "-", "", ".", "", " ", "").Replace(key))
}

// joinCodes lists keys as code, the last joined by the conjunction: "`a`, `b` or `c`"
func joinCodes(keys []string, conjunction string) string {
	codes := make([]string, len(keys))
	for i, key := range keys {
		codes[i] = "`" + key + "`"
	}
	if len(codes) == 1 {
		return codes[0]
	}
	return strings.Join(codes[:len(codes)-1], ", ") + " " + conjunction + " " + codes[len(codes)-1]
}

// settingChange is one key and value of a !chat set command
type settingChange struct {
	setting setting
	value   string
}

// setUsage is how !chat set is used
const setUsage = "Give each setting its value: `!chat set <key> <value> [<key> <value> …]`"

// parseChanges reads the keys and values of a !chat set command, given as the words that follow
// "set". Every key has to name a setting, once, and come with a value.
func parseChanges(words []string) ([]settingChange, error) {
	if len(words) == 0 {
		return nil, settingError(setUsage)
	}
	var changes []settingChange
	for i := 0; i < len(words); i += 2 {
		s, ok := settingByKey(words[i])
		if !ok {
			return nil, unknownSetting(words[i])
		}
		if i+1 == len(words) {
			return nil, settingErrorf("`%s` has no value. %s", s.key, setUsage)
		}
		if slices.ContainsFunc(changes, func(change settingChange) bool { return change.setting.key == s.key }) {
			return nil, settingErrorf("`%s` is given twice", s.key)
		}
		changes = append(changes, settingChange{setting: s, value: words[i+1]})
	}
	return changes, nil
}

// applyChanges applies the changes of a !chat set command to a configuration, then checks it as a
// whole, so that settings that depend on each other can be changed together
func applyChanges(cfg *db.ChatConfig, changes []settingChange) error {
	for _, change := range changes {
		if err := change.setting.set(cfg, change.value); err != nil {
			return err
		}
	}
	return checkConfig(*cfg)
}

// changedSettings returns the settings whose values differ between two configurations
func changedSettings(before, after db.ChatConfig) []setting {
	var changed []setting
	for _, s := range settings {
		if s.show(before) != s.show(after) {
			changed = append(changed, s)
		}
	}
	return changed
}

// unchanged is the answer to a !chat set that gives every setting the value it already has. No row
// is added for it.
func unchanged(cfg db.ChatConfig, changes []settingChange) error {
	already := make([]string, 0, len(changes))
	for _, change := range changes {
		already = append(already, fmt.Sprintf("`%s` is already %s", change.setting.key, change.setting.value(cfg)))
	}
	return settingError("No change: " + strings.Join(already, ", "))
}

// describeConfig renders the configuration for !chat config: every setting under its heading, with
// its value and what it is, and how to change them. window says how full the context window of the
// timeline the command was sent in is, since a token budget on its own is hard to picture.
//
// The listing is markdown, which doesn't break lines within a paragraph, so each setting is a list
// item of its own.
func describeConfig(cfg db.ChatConfig, window string) string {
	var b strings.Builder
	fmt.Fprintf(&b, "**Chat configuration #%d**, set %s\n", cfg.ID, localTime(cfg.CreatedAt))
	for _, group := range settingGroups {
		b.WriteString("\n**" + group.name + "**")
		if group.note != "" {
			b.WriteString(": " + group.note)
		}
		b.WriteString("\n")
		for _, s := range settings {
			if s.group == group.name {
				fmt.Fprintf(&b, "- `%s`: %s — %s\n", s.key, s.value(cfg), s.summary)
			}
		}
		if group.name == windowGroup {
			b.WriteString("- " + window + "\n")
		}
	}
	b.WriteString("\nChange one or more: `!chat set <key> <value> [<key> <value> …]`, such as " +
		"`!chat set context_high_tokens 32k context_low_tokens 16k`. Counts of tokens and bytes can be written " +
		"with k and M, multiples of 1024.\n\n" +
		"More about one: `!chat config <key>`")
	return b.String()
}

// describeSetting renders one setting in full for !chat config <key>: its value and since when it has
// had it, what it means, the values it takes, and how to change it. history is every configuration on
// record, newest first, so the first is the one in effect.
func describeSetting(s setting, history []db.ChatConfig) string {
	cfg, since := history[0], lastChange(s, history)
	return fmt.Sprintf("**`%s`**: %s, since #%d on %s\n\n%s\n\n- Takes: %s\n- Change: `!chat set %s %s`",
		s.key, s.value(cfg), since.ID, localTime(since.CreatedAt), s.help, s.allowed(cfg), s.key, s.example)
}

// lastChange returns the configuration a setting got its current value in: the oldest of the run of
// configurations, newest first in history, that all have that value
func lastChange(s setting, history []db.ChatConfig) db.ChatConfig {
	since := history[0]
	for _, cfg := range history[1:] {
		if s.show(cfg) != s.show(since) {
			break
		}
		since = cfg
	}
	return since
}

// describeChanges renders what a !chat set did: each setting it changed, from what to what, and the
// configuration the changes made
func describeChanges(before, after db.ChatConfig) string {
	var changes []string
	for _, s := range changedSettings(before, after) {
		changes = append(changes, fmt.Sprintf("`%s` %s → %s", s.key, s.show(before), s.value(after)))
	}
	return fmt.Sprintf("Changed (#%d): %s", after.ID, strings.Join(changes, ", "))
}

// withSettings adds every setting of a configuration to a log event, each as a field named by its key
func withSettings(event *zerolog.Event, cfg db.ChatConfig) *zerolog.Event {
	for _, s := range settings {
		event = event.Str(s.key, s.show(cfg))
	}
	return event
}

// withChanges adds the changes of a !chat set command to a log event, each as a field named by the
// key of its setting
func withChanges(event *zerolog.Event, changes []settingChange) *zerolog.Event {
	for _, change := range changes {
		event = event.Str(change.setting.key, change.value)
	}
	return event
}

// chatHelp lists the !chat subcommands
const chatHelp = "**Chat commands**\n" +
	"- `!chat config`: the chat settings, and how to change them\n" +
	"- `!chat config <key>`: one setting in full\n" +
	"- `!chat set <key> <value> [<key> <value> …]`: change settings, for the admin only\n" +
	"- `!chat reset`: forget the conversation so far, in this room or in this thread"
