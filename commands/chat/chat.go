package chat

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/Scrin/siikabot/tracing"

	"github.com/Scrin/siikabot/aigateway"
	"github.com/Scrin/siikabot/config"
	"github.com/Scrin/siikabot/db"
	"github.com/Scrin/siikabot/llmtools"
	"github.com/Scrin/siikabot/matrix"
	"github.com/Scrin/siikabot/metrics"
	"github.com/Scrin/siikabot/sourcecode"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
)

// imageDetailAuto omits the detail field, leaving the choice to the provider
const imageDetailAuto = "auto"

// requestImageDetail turns the configured image detail into the value sent with a request, where an
// empty value omits the field
func requestImageDetail(detail string) string {
	if detail == imageDetailAuto {
		return ""
	}
	return detail
}

// How long to keep chat history before cleaning it up
const chatHistoryRetention = 7 * 24 * time.Hour // 7 days

// How long to keep usage accounting records. Longer than the history itself: the conversations are
// transient, but the point of the accounting is to show cost trends over weeks.
const chatUsageRetention = 90 * 24 * time.Hour // 90 days

// toolRegistry holds all available tools
var toolRegistry *aigateway.ToolRegistry

// Init initializes the chat module
func Init(ctx context.Context) {
	// Initialize the tool registry
	toolRegistry = aigateway.NewToolRegistry()

	// Register the tool implementations from the chat package
	toolRegistry.RegisterTool(llmtools.ElectricityPricesToolDefinition)
	toolRegistry.RegisterTool(llmtools.FMIToolDefinition)
	toolRegistry.RegisterTool(llmtools.NewsToolDefinition)
	toolRegistry.RegisterTool(llmtools.WebSearchToolDefinition)
	toolRegistry.RegisterTool(llmtools.ReminderToolDefinition)
	toolRegistry.RegisterTool(llmtools.GitHubIssueToolDefinition)
	toolRegistry.RegisterTool(llmtools.FingridToolDefinition)
	toolRegistry.RegisterTool(llmtools.WebToolDefinition)
	toolRegistry.RegisterTool(llmtools.WhoisToolDefinition)
	toolRegistry.RegisterTool(llmtools.GitHubStatusToolDefinition)
	toolRegistry.RegisterTool(llmtools.DNSToolDefinition)
	toolRegistry.RegisterTool(llmtools.ExchangeRatesToolDefinition)
	toolRegistry.RegisterTool(llmtools.TimezoneToolDefinition)
	toolRegistry.RegisterTool(llmtools.WikipediaToolDefinition)
	toolRegistry.RegisterTool(llmtools.MemoryToolDefinition)
	toolRegistry.RegisterTool(llmtools.RoomMembersToolDefinition)
	toolRegistry.RegisterTool(llmtools.MessageStatsToolDefinition)
	toolRegistry.RegisterTool(llmtools.SourceCodeToolDefinition)

	for _, s := range settings {
		metrics.InitializeChatConfigKey(s.key)
	}

	// The source code can be looked up only in a repository on GitHub. Elsewhere the model still links to
	// the repository, but the tool tells it that it can't read anything there.
	if _, err := sourcecode.ParseRepository(config.SourceURL); err != nil {
		log.Warn().Ctx(ctx).Err(err).Str("source_url", config.SourceURL).Msg("The chat can't look up the bot's source code")
	}

	// Start a goroutine to periodically clean up old chat history
	go func() {
		ticker := time.NewTicker(1 * time.Hour)
		defer ticker.Stop()

		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				// Each tick is its own root span: there is no request to inherit from, and
				// attaching to the bot's context would produce a span that never ends
				tracing.Run(ctx, tracer, "chat.cleanup", func(tickCtx context.Context) {
					cleanupChatHistory(tickCtx)
					cleanupChatUsage(tickCtx)
				})
			}
		}
	}()
}

// cleanupChatHistory removes old chat history entries
func cleanupChatHistory(ctx context.Context) {
	count, err := db.CleanupOldChatHistory(ctx, chatHistoryRetention)
	if err != nil {
		log.Error().Ctx(ctx).Err(err).Msg("Failed to clean up old chat history")
		return
	}
	if count > 0 {
		log.Info().Ctx(ctx).Int64("removed_count", count).Msg("Cleaned up old chat history")
	}
}

// cleanupChatUsage removes old usage accounting records. Kept far longer than the chat history
// itself, since the point of the accounting is to show trends over weeks.
func cleanupChatUsage(ctx context.Context) {
	count, err := db.CleanupOldChatUsage(ctx, chatUsageRetention)
	if err != nil {
		log.Error().Ctx(ctx).Err(err).Msg("Failed to clean up old chat usage")
		return
	}
	if count > 0 {
		log.Info().Ctx(ctx).Int64("removed_count", count).Msg("Cleaned up old chat usage")
	}
}

// describeWindowNow says how full the context window of a timeline is as it currently stands, since
// a token budget on its own is hard to picture.
//
// Deliberately reads the window without maintaining it: showing the configuration should not move
// the anchor as a side effect.
func describeWindowNow(ctx context.Context, timeline db.Timeline, sender string, cfg db.ChatConfig) string {
	window := currentContextWindow(ctx, timeline, threadSeed(ctx, timeline, sender), cfg.ContextHighTokens)
	images, _ := historyImages(window)
	where := "This room"
	if timeline.ThreadRootID != "" {
		where = "This thread"
	}
	return fmt.Sprintf("%s now: ~%d tokens in %d %s and %d %s", where, estimateHistoryTokens(window, time.Now()),
		len(window), plural(len(window), "message", "messages"), images, plural(images, "image", "images"))
}

// showConfig answers !chat config: the configuration in effect, and how full the context window of
// the timeline the command was sent in is
func showConfig(ctx context.Context, sender string, timeline db.Timeline, reply func(string)) {
	cfg, err := db.GetChatConfig(ctx)
	if err != nil {
		log.Error().Ctx(ctx).Err(err).Str("room_id", timeline.RoomID).Msg("Failed to load chat config")
		reply("Failed to load the chat configuration")
		return
	}
	reply(describeConfig(cfg, describeWindowNow(ctx, timeline, sender, cfg)))
}

// showSetting answers !chat config <key>: the setting the key names, in full, with since when it has
// had its value
func showSetting(ctx context.Context, roomID, key string, reply func(string)) {
	s, ok := settingByKey(key)
	if !ok {
		reply(unknownSetting(key).Error())
		return
	}

	history, err := db.GetChatConfigHistory(ctx)
	if err != nil {
		log.Error().Ctx(ctx).Err(err).Str("room_id", roomID).Str("key", s.key).Msg("Failed to load chat config history")
		reply("Failed to load the chat configuration")
		return
	}
	reply(describeSetting(s, history))
}

// showHistory answers !chat config history [n]: the latest n configurations, each with what it
// changed, given as the words that follow "history"
func showHistory(ctx context.Context, roomID string, words []string, reply func(string)) {
	count := defaultHistoryLength
	if len(words) > 0 {
		n, err := strconv.Atoi(words[0])
		if err != nil || n < 1 || n > maxHistoryLength || len(words) > 1 {
			reply(historyUsage)
			return
		}
		count = n
	}

	history, err := db.GetChatConfigHistory(ctx)
	if err != nil {
		log.Error().Ctx(ctx).Err(err).Str("room_id", roomID).Msg("Failed to load chat config history")
		reply("Failed to load the chat configuration")
		return
	}
	reply(describeHistory(history, count))
}

// setConfig answers !chat set: the settings given as key and value pairs in words are changed
// together, as one new configuration
func setConfig(ctx context.Context, roomID string, words []string, reply func(string)) {
	changes, err := parseChanges(words)
	if err != nil {
		reply(err.Error())
		return
	}

	updateConfig(ctx, roomID, reply,
		func(cfg *db.ChatConfig) error {
			before := *cfg
			if err := applyChanges(cfg, changes); err != nil {
				return err
			}
			if len(changedSettings(before, *cfg)) == 0 {
				return unchanged(before, changes)
			}
			return nil
		},
		func(event *zerolog.Event) *zerolog.Event { return withChanges(event, changes) },
		describeChanges)
}

// revertConfig answers !chat config revert <id>, with the id as the words that follow "revert": the
// configuration with the id becomes the one in effect again, as a new configuration that copies it,
// so the one it replaces stays on record as well
func revertConfig(ctx context.Context, roomID string, words []string, reply func(string)) {
	if len(words) != 1 {
		reply(revertUsage)
		return
	}
	id, err := strconv.ParseInt(strings.TrimPrefix(words[0], "#"), 10, 64)
	if err != nil {
		reply(revertUsage)
		return
	}

	target, found, err := db.GetChatConfigByID(ctx, id)
	if err != nil {
		log.Error().Ctx(ctx).Err(err).Str("room_id", roomID).Int64("config_id", id).Msg("Failed to load chat config")
		reply("Failed to load the chat configuration")
		return
	}
	if !found {
		reply(fmt.Sprintf("There's no configuration #%d. `!chat config history` lists them.", id))
		return
	}

	updateConfig(ctx, roomID, reply,
		func(cfg *db.ChatConfig) error {
			before := *cfg
			if err := applyRevert(cfg, target); err != nil {
				return err
			}
			if len(changedSettings(before, *cfg)) == 0 {
				return settingErrorf("No change: the configuration in effect is already as #%d has it", target.ID)
			}
			return nil
		},
		func(event *zerolog.Event) *zerolog.Event { return event.Int64("reverted_to_config_id", target.ID) },
		func(before, after db.ChatConfig) string {
			return fmt.Sprintf("Back to #%d, as #%d: %s", target.ID, after.ID, changeList(before, after))
		})
}

// updateConfig applies a change to the chat configuration as one new row, which keeps the previous
// configuration on record, and tells the room how it went. The change is applied and checked under
// the table's lock, on the configuration as it then is, and refuses with a settingError, which the
// room is told. asked adds what the command asked for to the log, and done says what the change did.
func updateConfig(ctx context.Context, roomID string, reply func(string), change func(*db.ChatConfig) error,
	asked func(*zerolog.Event) *zerolog.Event, done func(before, after db.ChatConfig) string) {
	var before db.ChatConfig
	after, err := db.UpdateChatConfig(ctx, func(cfg *db.ChatConfig) error {
		before = *cfg
		return change(cfg)
	})

	var refused settingError
	if errors.As(err, &refused) {
		asked(log.Debug().Ctx(ctx).Str("room_id", roomID).Str("reason", refused.Error())).
			Msg("Chat config change refused")
		reply(refused.Error())
		return
	}
	if err != nil {
		asked(log.Error().Ctx(ctx).Err(err).Str("room_id", roomID)).Msg("Failed to change chat config")
		reply("Failed to change the chat configuration")
		return
	}

	changed := changedSettings(before, after)
	keys := make([]string, 0, len(changed))
	for _, s := range changed {
		keys = append(keys, s.key)
		metrics.RecordChatConfigChange(s.key)
	}
	withSettings(asked(log.Info().Ctx(ctx).Str("room_id", roomID).Int64("config_id", after.ID).Strs("changed_keys", keys)), after).
		Msg("Chat config changed")
	reply(done(before, after))
}

// Handle handles the !chat command. The command acts on the timeline it was sent in, the main
// timeline or a thread, which eventID and threadRootID say, and is answered there.
func Handle(ctx context.Context, roomID, sender, eventID, threadRootID, msg string) {
	words := strings.Fields(msg)

	timeline := db.Timeline{RoomID: roomID, ThreadRootID: threadRootID}
	// Where the command was sent. It is answered there, so that a reset in a thread, say, says so in
	// the thread.
	here := matrix.Target{ThreadRootID: threadRootID, InReplyTo: eventID}
	reply := func(text string) {
		matrix.SendMarkdownFormattedNoticeTo(ctx, roomID, here, text, nil, nil)
	}

	var subcommand string
	if len(words) > 1 {
		subcommand = words[1]
	}

	switch subcommand {
	case "reset":
		count, err := db.ResetChatTimeline(ctx, timeline)
		if err != nil {
			log.Error().Ctx(ctx).Err(err).Str("room_id", roomID).Str("thread_root_id", threadRootID).Msg("Failed to reset chat history")
			matrix.SendMessageTo(ctx, roomID, here, "Failed to reset chat history", nil)
			return
		}
		log.Info().Ctx(ctx).
			Str("room_id", roomID).
			Str("thread_root_id", threadRootID).
			Int64("deleted_count", count).
			Msg("Chat history reset")
		what := "Chat history"
		if threadRootID != "" {
			what = "Chat history of this thread"
		}
		matrix.SendMessageTo(ctx, roomID, here, fmt.Sprintf("%s reset (%d messages deleted)", what, count), nil)
	case "config":
		switch {
		case len(words) > 2 && words[2] == "history":
			showHistory(ctx, roomID, words[3:], reply)
		case len(words) > 2 && words[2] == "revert":
			if sender != config.Admin {
				reply("Only the admin can change the chat configuration")
				return
			}
			revertConfig(ctx, roomID, words[3:], reply)
		case len(words) > 2:
			showSetting(ctx, roomID, words[2], reply)
		default:
			showConfig(ctx, sender, timeline, reply)
		}
	case "set":
		switch {
		case len(words) == 3:
			// A key without a value asks what the setting is, as !chat config <key> does
			showSetting(ctx, roomID, words[2], reply)
		case sender != config.Admin:
			reply("Only the admin can change the chat configuration")
		default:
			setConfig(ctx, roomID, words[2:], reply)
		}
	default:
		reply(chatHelp)
	}
}
