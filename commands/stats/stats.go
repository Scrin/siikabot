package stats

import (
	"context"
	"fmt"
	"strings"

	"github.com/Scrin/siikabot/db"
	"github.com/Scrin/siikabot/matrix"
	"github.com/rs/zerolog/log"
)

func Handle(ctx context.Context, roomID, sender, msg string) {
	args := strings.Fields(msg)

	userID := sender
	if len(args) >= 2 {
		userID = args[1]
	}

	log.Debug().Ctx(ctx).
		Str("room_id", roomID).
		Str("target_user_id", userID).
		Msg("Executing stats command")

	stats, err := db.GetMessageStats(ctx, roomID, userID)
	if err != nil {
		log.Error().Ctx(ctx).Err(err).
			Str("room_id", roomID).
			Str("target_user_id", userID).
			Msg("Failed to get message stats")
		matrix.SendNotice(roomID, "Failed to retrieve stats: "+err.Error())
		return
	}

	if stats == nil {
		matrix.SendNotice(roomID, fmt.Sprintf("No stats found for user %s in this room", userID))
		return
	}

	displayName := matrix.GetDisplayName(ctx, userID)
	if displayName == "" {
		displayName = userID
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("Message statistics for %s:\n", displayName))
	sb.WriteString(fmt.Sprintf("- Messages: %d\n", stats.MessageCount))
	sb.WriteString(fmt.Sprintf("- Words: %d\n", stats.WordCount))
	sb.WriteString(fmt.Sprintf("- Characters: %d\n", stats.CharacterCount))
	sb.WriteString(fmt.Sprintf("- Last seen: %s\n", stats.LastSeen.Format("2006-01-02 15:04:05 MST")))

	if stats.MessageCount > 0 {
		avgWords := float64(stats.WordCount) / float64(stats.MessageCount)
		avgChars := float64(stats.CharacterCount) / float64(stats.MessageCount)
		sb.WriteString(fmt.Sprintf("- Avg words/message: %.1f\n", avgWords))
		sb.WriteString(fmt.Sprintf("- Avg chars/message: %.1f", avgChars))
	}

	matrix.SendNotice(roomID, sb.String())

	log.Debug().Ctx(ctx).
		Str("room_id", roomID).
		Str("target_user_id", userID).
		Int("message_count", stats.MessageCount).
		Msg("Stats command completed")
}
