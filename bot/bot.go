package bot

import (
	"context"
	"regexp"
	"strings"

	"github.com/Scrin/siikabot/api"
	"github.com/Scrin/siikabot/auth"
	authcmd "github.com/Scrin/siikabot/commands/auth"
	"github.com/Scrin/siikabot/commands/chat"
	configcmd "github.com/Scrin/siikabot/commands/config"
	"github.com/Scrin/siikabot/commands/federation"
	"github.com/Scrin/siikabot/commands/grafana"
	"github.com/Scrin/siikabot/commands/ping"
	"github.com/Scrin/siikabot/commands/remind"
	"github.com/Scrin/siikabot/commands/ruuvi"
	"github.com/Scrin/siikabot/commands/stats"
	"github.com/Scrin/siikabot/commands/traceroute"
	"github.com/Scrin/siikabot/config"
	"github.com/Scrin/siikabot/db"
	"github.com/Scrin/siikabot/logging"
	"github.com/Scrin/siikabot/matrix"
	"github.com/Scrin/siikabot/metrics"
	"github.com/rs/zerolog/log"
	"maunium.net/go/mautrix/event"
)

// isCommandEnabled checks if a command is enabled for a room
// Most commands are enabled by default, except for specific commands that need to be explicitly enabled in the database
func isCommandEnabled(ctx context.Context, roomID string, command string) bool {
	// These commands are disabled by default and need to be explicitly enabled
	restrictedCommands := map[string]bool{
		"!ruuvi":   true,
		"!grafana": true,
	}

	// If the command is not restricted, it's always enabled
	if !restrictedCommands[command] {
		return true
	}

	// For restricted commands, check if they're explicitly enabled in the database
	enabled, err := db.IsCommandEnabled(ctx, roomID, command)
	if err != nil {
		log.Error().Ctx(ctx).Err(err).Str("room_id", roomID).Str("command", command).Msg("Failed to query enabled commands")
		return false
	}
	return enabled
}

func handleTextEvent(ctx context.Context, evt *event.Event) {
	if evt.Sender.String() == config.UserID {
		return
	}

	ctx = logging.ContextWithStr(ctx, "msg_room_id", evt.RoomID.String())
	ctx = logging.ContextWithStr(ctx, "msg_sender", evt.Sender.String())
	ctx = logging.ContextWithStr(ctx, "msg_event_id", evt.ID.String())

	msgtype := ""
	if m, ok := evt.Content.Raw["msgtype"].(string); ok {
		msgtype = m
	}

	if msgtype == "m.text" && evt.Sender.String() != config.UserID {
		msg := evt.Content.Raw["body"].(string)

		// Track message stats asynchronously
		go db.UpdateMessageStats(ctx, evt.RoomID.String(), evt.Sender.String(), msg)

		format, _ := evt.Content.Raw["format"].(string)
		formattedBody, _ := evt.Content.Raw["formatted_body"].(string)
		msgCommand := strings.Split(msg, " ")[0]
		isCommand := true

		if !isCommandEnabled(ctx, evt.RoomID.String(), msgCommand) {
			return
		}

		switch msgCommand {
		case "!ping":
			go ping.Handle(ctx, evt.RoomID.String(), msg)
		case "!traceroute":
			go traceroute.Handle(ctx, evt.RoomID.String(), msg)
		case "!ruuvi":
			go ruuvi.Handle(ctx, evt.RoomID.String(), evt.Sender.String(), msg)
		case "!grafana":
			go grafana.Handle(ctx, evt.RoomID.String(), evt.Sender.String(), msg)
		case "!remind":
			go remind.Handle(ctx, evt.RoomID.String(), evt.Sender.String(), msg, format, formattedBody)
		case "!chat":
			go chat.Handle(ctx, evt.RoomID.String(), evt.Sender.String(), msg)
		case "!servers":
			go federation.Handle(ctx, evt.RoomID.String(), msg)
		case "!config":
			mentionedUsers := extractMentionedUsers(evt)
			go configcmd.Handle(ctx, evt.RoomID.String(), evt.Sender.String(), msg, mentionedUsers)
		case "!auth":
			go authcmd.Handle(ctx, evt.RoomID.String(), evt.Sender.String(), msg)
		case "!stats":
			go stats.Handle(ctx, evt.RoomID.String(), evt.Sender.String(), msg)
		default:
			isCommand = false

			// Extract the m.relates_to field if it exists
			var relatesTo map[string]any
			if relates, ok := evt.Content.Raw["m.relates_to"].(map[string]any); ok {
				relatesTo = relates
			}

			// Check if the message is a reply to a message sent by the bot
			isReplyToBot := false
			if relatesTo != nil {
				if inReplyTo, ok := relatesTo["m.in_reply_to"].(map[string]any); ok {
					if replyEventID, ok := inReplyTo["event_id"].(string); ok {
						// Get the sender of the replied-to message
						repliedToSender, err := matrix.GetEventSender(ctx, evt.RoomID.String(), replyEventID)
						if err == nil && repliedToSender == config.UserID {
							isReplyToBot = true
						}
					}
				}
			}

			// Check if the message contains a mention of the bot
			if containsBotMention(msg, formattedBody) || isReplyToBot {
				// Extract the actual message content (remove the mention part if it's a mention)
				chatMsg := msg
				if containsBotMention(msg, formattedBody) {
					chatMsg = extractMessageContent(msg, formattedBody)
				}

				go chat.HandleMention(ctx, evt.RoomID.String(), evt.Sender.String(), chatMsg, evt.ID.String(), relatesTo)
				isCommand = true
				msgCommand = "mention"
				if isReplyToBot {
					msgCommand = "reply"
				}
			}
		}
		if isCommand {
			matrix.MarkRead(ctx, evt.RoomID.String(), evt.ID.String())
			log.Debug().
				Str("command", msgCommand).
				Str("room_id", evt.RoomID.String()).
				Str("sender", evt.Sender.String()).
				Msg("Handled command")
			metrics.RecordCommandHandled(msgCommand)
		}
	}
}

// extractMentionedUsers extracts user IDs from the m.mentions field of an event
func extractMentionedUsers(evt *event.Event) []string {
	mentions, ok := evt.Content.Raw["m.mentions"].(map[string]any)
	if !ok {
		return nil
	}

	userIDs, ok := mentions["user_ids"].([]any)
	if !ok {
		return nil
	}

	result := make([]string, 0, len(userIDs))
	for _, id := range userIDs {
		if strID, ok := id.(string); ok {
			result = append(result, strID)
		}
	}
	return result
}

// containsBotMention checks if the message contains a mention of the bot
func containsBotMention(plainMsg, formattedMsg string) bool {
	// Check for URLs in the message - both with and without protocol prefix
	// This pattern matches common URL formats including those without http/https prefix
	urlPattern := regexp.MustCompile(`(https?://|www\.)[^\s]+|[a-zA-Z0-9][-a-zA-Z0-9]*\.[a-zA-Z0-9][-a-zA-Z0-9]*\.[a-zA-Z]{2,}[^\s]*|[a-zA-Z0-9][-a-zA-Z0-9]*\.[a-zA-Z]{2,}[^\s]*`)
	urls := urlPattern.FindAllString(plainMsg, -1)

	// Create a copy of plainMsg with all URLs removed
	plainMsgWithoutUrls := plainMsg
	for _, url := range urls {
		plainMsgWithoutUrls = strings.Replace(plainMsgWithoutUrls, url, "", -1)
	}

	// Check for mention in plain text by display name
	botDisplayName := matrix.GetDisplayName(context.Background(), config.UserID)
	if botDisplayName != "" && strings.Contains(strings.ToLower(plainMsgWithoutUrls), strings.ToLower(botDisplayName)) {
		return true
	}

	// Check for mention in formatted text (Matrix uses <a href="https://matrix.to/#/@user:domain.com">@user</a> format)
	if formattedMsg != "" && strings.Contains(formattedMsg, "https://matrix.to/#/"+config.UserID) {
		return true
	}

	return false
}

// extractMessageContent removes the bot mention from the message
func extractMessageContent(plainMsg, formattedMsg string) string {
	// Get bot identifiers
	botUserName := strings.Split(config.UserID, ":")[0][1:] // Remove @ and domain part
	botDisplayName := matrix.GetDisplayName(context.Background(), config.UserID)

	// Try to extract content after user ID mention
	if idx := strings.Index(strings.ToLower(plainMsg), "@"+strings.ToLower(botUserName)); idx >= 0 {
		// Find the end of the mention (space or colon typically follows the mention)
		endIdx := idx + len(botUserName) + 1 // +1 for the @ symbol
		for endIdx < len(plainMsg) && plainMsg[endIdx] != ' ' && plainMsg[endIdx] != ':' {
			endIdx++
		}

		// If there's content after the mention, extract it
		if endIdx < len(plainMsg) {
			// Skip any colon or space after the mention
			for endIdx < len(plainMsg) && (plainMsg[endIdx] == ' ' || plainMsg[endIdx] == ':') {
				endIdx++
			}
			return strings.TrimSpace(plainMsg[endIdx:])
		}
	}

	// Try to extract content after display name mention
	if botDisplayName != "" {
		if idx := strings.Index(strings.ToLower(plainMsg), strings.ToLower(botDisplayName)); idx >= 0 {
			// Find the end of the mention
			endIdx := idx + len(botDisplayName)

			// If there's content after the mention, extract it
			if endIdx < len(plainMsg) {
				// Skip any colon or space after the mention
				for endIdx < len(plainMsg) && (plainMsg[endIdx] == ' ' || plainMsg[endIdx] == ':') {
					endIdx++
				}
				return strings.TrimSpace(plainMsg[endIdx:])
			}
		}
	}

	// If we can't extract a clean message, try to remove the bot name from the message
	cleanedMsg := plainMsg
	if botDisplayName != "" {
		cleanedMsg = strings.ReplaceAll(strings.ToLower(cleanedMsg), strings.ToLower(botDisplayName), "")
	}
	cleanedMsg = strings.ReplaceAll(strings.ToLower(cleanedMsg), "@"+strings.ToLower(botUserName), "")

	return strings.TrimSpace(cleanedMsg)
}

func handleMemberEvent(ctx context.Context, evt *event.Event) {
	if evt.Content.Raw["membership"] == "invite" && evt.GetStateKey() == config.UserID {
		matrix.JoinRoom(ctx, evt.RoomID.String())
		log.Info().
			Str("room_id", evt.RoomID.String()).
			Str("inviter", evt.Sender.String()).
			Msg("Joined room from invite")
	}
}

func handleEvent(ctx context.Context, evt *event.Event, wasEncrypted bool) {
	switch evt.Type {
	case event.EventMessage:
		handleTextEvent(ctx, evt)
	case event.StateMember:
		handleMemberEvent(ctx, evt)
	}
	subtype := ""
	if m, ok := evt.Content.Raw["msgtype"].(string); ok {
		subtype = m
	}
	metrics.RecordEventHandled(evt.Type.String(), subtype, wasEncrypted)
}

func Init(ctx context.Context) error {
	if err := db.Init(); err != nil {
		return err
	}
	if err := matrix.Init(ctx, handleEvent); err != nil {
		return err
	}

	resp := matrix.InitialSync(ctx)
	for roomID := range resp.Rooms.Invite {
		matrix.JoinRoom(ctx, roomID.String())
		log.Info().
			Str("room_id", roomID.String()).
			Msg("Joined room during initial sync")
	}

	remind.Init(ctx)
	chat.Init(ctx)
	auth.Init()
	api.Init()
	initHTTP()

	return nil
}

func Run() error {
	return matrix.Sync()
}
