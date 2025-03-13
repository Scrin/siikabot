package llmtools

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/Scrin/siikabot/commands/remind"
	"github.com/Scrin/siikabot/config"
	"github.com/Scrin/siikabot/db"
	"github.com/Scrin/siikabot/matrix"
	"github.com/Scrin/siikabot/openrouter"
	"github.com/rs/zerolog/log"
)

// ReminderToolDefinition returns the tool definition for the reminder tool
var ReminderToolDefinition = openrouter.ToolDefinition{
	Type: "function",
	Function: openrouter.FunctionSchema{
		Name:        "create_reminder",
		Description: "Create a reminder that will trigger at a specified time or after a specified duration",
		Parameters: json.RawMessage(`{
				"type": "object",
				"properties": {
					"time": {
						"type": "string",
						"description": "The time to set the reminder for. Must be in the format 'DD.MM.YYYY-HH:MM:SS' (24 hour clock)"
					},
					"message": {
						"type": "string",
						"description": "The message to include with the reminder"
					}
				},
				"required": ["time", "message"]
			}`),
	},
	Handler: handleReminderToolCall,
}

// handleReminderToolCall handles reminder tool calls
func handleReminderToolCall(ctx context.Context, arguments string) (string, error) {
	// Parse the arguments
	var args struct {
		Time    string `json:"time"`
		Message string `json:"message"`
	}

	// Log the raw arguments for debugging
	log.Debug().Ctx(ctx).Str("arguments", arguments).Msg("Received reminder tool call")

	if err := json.Unmarshal([]byte(arguments), &args); err != nil {
		log.Error().Ctx(ctx).Err(err).Str("arguments", arguments).Msg("Failed to parse tool arguments")
		return "", fmt.Errorf("failed to parse arguments: %w", err)
	}

	if args.Time == "" {
		return "", errors.New("time is required")
	}

	if args.Message == "" {
		return "", errors.New("message is required")
	}

	// Get the room ID and sender from the context
	roomID, ok := ctx.Value("room_id").(string)
	if !ok || roomID == "" {
		return "", errors.New("room ID not found in context")
	}

	sender, ok := ctx.Value("sender").(string)
	if !ok || sender == "" {
		return "", errors.New("sender not found in context")
	}

	// Parse the reminder time using the functions from the remind package
	now := time.Now()
	reminderTime, durationErr := remind.RemindDuration(now, args.Time)
	var timeErr error
	if durationErr != nil {
		reminderTime, timeErr = remind.RemindTime(now, args.Time)
	}
	if timeErr != nil {
		return "", fmt.Errorf("invalid date/time or duration: %s (duration error: %s, date/time error: %s)",
			args.Time, durationErr.Error(), timeErr.Error())
	}

	// Format the message for HTML display
	reminderText := strings.Replace(args.Message, "\n", "<br>", -1)

	// Create the reminder
	rem := db.Reminder{
		RemindTime: reminderTime,
		UserID:     sender,
		RoomID:     roomID,
		Message:    reminderText,
	}

	// Add the reminder to the database
	id, err := db.AddReminder(ctx, rem)
	if err != nil {
		log.Error().Ctx(ctx).Err(err).
			Str("room_id", roomID).
			Str("sender", sender).
			Time("remind_time", reminderTime).
			Msg("Failed to save reminder")
		return "", fmt.Errorf("failed to save reminder: %w", err)
	}
	rem.ID = id

	// Start the reminder using the function from the remind package
	remind.StartReminder(ctx, rem)

	// Calculate the duration until the reminder
	duration := reminderTime.Sub(now).Truncate(time.Second)

	// Get the timezone
	loc, _ := time.LoadLocation(config.Timezone)

	matrix.SendFormattedNotice(roomID, "[AI tool call] Reminding at "+reminderTime.In(loc).Format("15:04:05 on 2.1.2006")+" (in "+duration.String()+"): "+reminderText)

	log.Info().
		Str("room_id", roomID).
		Str("sender", sender).
		Time("remind_time", reminderTime).
		Str("duration", duration.String()).
		Msg("Reminder set via tool")

	// Return a formatted response
	return fmt.Sprintf("Reminder set for %s (in %s): %s",
		reminderTime.In(loc).Format("15:04:05 on 2.1.2006"),
		duration.String(),
		args.Message), nil
}
