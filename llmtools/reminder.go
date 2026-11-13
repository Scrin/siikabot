package llmtools

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/Scrin/siikabot/aigateway"
	"github.com/Scrin/siikabot/commands/remind"
	"github.com/Scrin/siikabot/db"
	"github.com/Scrin/siikabot/matrix"
	"github.com/rs/zerolog/log"
)

// ManageRemindersToolDefinition creates, lists and deletes the reminders of whoever is speaking.
//
// It sees only the reminders they set in the room the chat turn happens in, which it takes from the
// turn, never from its arguments: a reminder set in one room can't be listed or deleted from a
// conversation in another. When it deletes one, the bot posts a notice in the room with the command
// that sets it again, since a reminder may matter and the model may have been misled into deleting
// it.
var ManageRemindersToolDefinition = aigateway.ToolDefinition{
	Type: "function",
	Function: aigateway.FunctionSchema{
		Name: "manage_reminders",
		Description: "Manage the reminders of the author of the latest message in this room. " +
			"'create' sets a reminder that mentions them when it triggers at a specified time or after a specified duration; it can't remind anyone else. " +
			"'list' shows the reminders they have set in this room that are still to go off, and 'delete' deletes one of them by ID. " +
			"Only reminders set in this room can be listed or deleted. Reminders set in other rooms can't be seen from here: they can be managed in their own room or on the web dashboard.",
		Parameters: json.RawMessage(`{
			"type": "object",
			"properties": {
				"action": {
					"type": "string",
					"enum": ["create", "list", "delete"],
					"description": "The action to perform: 'create' to set a new reminder, 'list' to list their reminders in this room, 'delete' to delete one of those by ID"
				},
				"time": {
					"type": "string",
					"description": "The time to set the reminder for (required for 'create'). Must be in the format 'DD.MM.YYYY-HH:MM:SS' (24 hour clock)"
				},
				"message": {
					"type": "string",
					"description": "The message to include with the reminder (required for 'create')"
				},
				"reminder_id": {
					"type": "integer",
					"description": "The ID of the reminder to delete, as 'list' shows it (required for 'delete')"
				}
			},
			"required": ["action"]
		}`),
	},
	Handler: handleManageRemindersToolCall,
	Trusted: true,
}

func handleManageRemindersToolCall(ctx context.Context, arguments string) (string, error) {
	var args struct {
		Action     string `json:"action"`
		Time       string `json:"time"`
		Message    string `json:"message"`
		ReminderID *int64 `json:"reminder_id"`
	}

	log.Debug().Ctx(ctx).Str("arguments", arguments).Msg("Received manage reminders tool call")

	if err := json.Unmarshal([]byte(arguments), &args); err != nil {
		log.Error().Ctx(ctx).Err(err).Str("arguments", arguments).Msg("Failed to parse tool arguments")
		return "", fmt.Errorf("failed to parse arguments: %w", err)
	}

	// The turn's room and author, never the arguments: this is what keeps the tool to the
	// reminders they set in this room
	roomID, ok := ctx.Value("room_id").(string)
	if !ok || roomID == "" {
		return "", errors.New("room ID not found in context")
	}
	sender, ok := ctx.Value("sender").(string)
	if !ok || sender == "" {
		return "", errors.New("sender not found in context")
	}

	switch args.Action {
	case "create":
		return createReminder(ctx, roomID, sender, args.Time, args.Message)
	case "list":
		return listReminders(ctx, roomID, sender)
	case "delete":
		return deleteReminder(ctx, roomID, sender, args.ReminderID)
	default:
		return "", fmt.Errorf("unknown action: %s", args.Action)
	}
}

func createReminder(ctx context.Context, roomID, sender, timeParam, message string) (string, error) {
	if timeParam == "" {
		return "", errors.New("time is required for the create action")
	}
	if message == "" {
		return "", errors.New("message is required for the create action")
	}

	now := time.Now()
	reminderTime, err := remind.ParseTime(now, timeParam)
	if err != nil {
		return "", err
	}

	rem := db.Reminder{
		RemindTime: reminderTime,
		UserID:     sender,
		RoomID:     roomID,
		// Formatted for HTML display
		Message: strings.Replace(message, "\n", "<br>", -1),
	}

	id, err := remind.Create(ctx, rem)
	if err != nil {
		log.Error().Ctx(ctx).Err(err).
			Str("room_id", roomID).
			Str("sender", sender).
			Time("remind_time", reminderTime).
			Msg("Failed to save reminder")
		return "", fmt.Errorf("failed to save reminder: %w", err)
	}
	rem.ID = id

	duration := reminderTime.Sub(now).Truncate(time.Second)

	// Posted where the conversation is, in the thread if the turn is in one
	target, _ := ctx.Value("reply_target").(matrix.Target)
	matrix.SendFormattedNoticeTo(ctx, roomID, target, "[AI tool call] Reminding "+remind.Describe(rem, now))

	log.Info().
		Ctx(ctx).
		Int64("reminder_id", id).
		Str("room_id", roomID).
		Str("sender", sender).
		Time("remind_time", reminderTime).
		Str("duration", duration.String()).
		Msg("Reminder set via tool")

	return fmt.Sprintf("Reminder %d set for %s (in %s): %s",
		id,
		reminderTime.In(remind.Location()).Format("15:04:05 on 2.1.2006"),
		duration.String(),
		message), nil
}

func listReminders(ctx context.Context, roomID, sender string) (string, error) {
	reminders, err := db.GetUpcomingRemindersIn(ctx, sender, roomID)
	if err != nil {
		return "", fmt.Errorf("failed to list reminders: %w", err)
	}
	return formatReminderList(sender, reminders, time.Now()), nil
}

// otherRoomsNote follows every listing, so that the model doesn't take a short list for all of
// someone's reminders
const otherRoomsNote = "Reminders set in other rooms aren't shown here. They can be seen on the web dashboard."

// formatReminderList lists the reminders author set in this room. A listing stays in the history,
// so it says when it was made.
func formatReminderList(author string, reminders []db.Reminder, now time.Time) string {
	asOf := now.In(remind.Location()).Format("15:04:05 on 2.1.2006")
	if len(reminders) == 0 {
		return fmt.Sprintf("As of %s, %s has no reminders set in this room that are still to go off. %s", asOf, author, otherRoomsNote)
	}

	var b strings.Builder
	fmt.Fprintf(&b, "Reminders of %s set in this room, as of %s:\n", author, asOf)
	for _, rem := range reminders {
		fmt.Fprintf(&b, "- [ID: %d] %s\n", rem.ID, remind.Describe(rem, now))
	}
	if len(reminders) >= db.MaxListedReminders {
		fmt.Fprintf(&b, "Only the first %d are listed.\n", db.MaxListedReminders)
	}
	b.WriteString(otherRoomsNote)
	return b.String()
}

func deleteReminder(ctx context.Context, roomID, sender string, reminderID *int64) (string, error) {
	if reminderID == nil {
		return "", errors.New("reminder_id is required for the delete action")
	}

	rem, err := remind.Delete(ctx, db.ReminderScope{UserID: sender, RoomID: roomID}, *reminderID, remind.ViaChat)
	if err != nil && !errors.Is(err, remind.ErrNotFound) && !errors.Is(err, remind.ErrAlreadyDelivered) {
		return "", fmt.Errorf("failed to delete reminder: %w", err)
	}

	now := time.Now()
	if err == nil {
		// Posted where the conversation is, in the thread if the turn is in one. The reminder is
		// in this room, so the command it gives sets it again here.
		target, _ := ctx.Value("reply_target").(matrix.Target)
		matrix.SendFormattedNoticeTo(ctx, roomID, target, remind.DeletionNotice(rem, now))
	}
	return deletionResult(sender, *reminderID, rem, err, now), nil
}

// deletionResult tells the model how a deletion went. err is nil, remind.ErrNotFound or
// remind.ErrAlreadyDelivered.
func deletionResult(author string, id int64, rem db.Reminder, err error, now time.Time) string {
	switch {
	case errors.Is(err, remind.ErrNotFound):
		return fmt.Sprintf("%s has no reminder %d set in this room, so nothing was deleted. "+
			"Reminders set in other rooms can only be deleted there or on the web dashboard.", author, id)
	case errors.Is(err, remind.ErrAlreadyDelivered):
		return fmt.Sprintf("Reminder %d already went off, so there was nothing to delete.", id)
	default:
		return fmt.Sprintf("Deleted reminder %d, which was due %s\n"+
			"A notice with the command that sets it again was posted in the room, so there's no need to repeat the command.",
			id, remind.Describe(rem, now))
	}
}
