package remind

import (
	"context"
	"errors"
	"fmt"
	"html"
	"strings"
	"time"

	"github.com/Scrin/siikabot/config"
	"github.com/Scrin/siikabot/db"
	"github.com/Scrin/siikabot/matrix"
	"github.com/Scrin/siikabot/metrics"
	"github.com/Scrin/siikabot/tracing"
	"github.com/rs/zerolog/log"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
)

var dateTimeFormats = []string{
	"2.1.2006-15:04", "15:04-2.1.2006",
	"2.1.2006-15:04:05", "15:04:05-2.1.2006",
	"2006-01-02-15:04", "15:04-2006-01-02",
	"2006-01-02-15:04:05", "15:04:05-2006-01-02"}
var dateTimeFormatsTZ = []string{
	time.ANSIC, time.UnixDate,
	time.RFC822, time.RFC822Z,
	time.RFC1123, time.RFC1123Z,
	time.RFC3339, time.RFC3339Nano}
var timeFormats = []string{"15:04", "15:04:05"}
var dateFormats = []string{"2.1.2006", "2006-1-2"}

// Where a reminder was deleted, for the metrics and the logs
const (
	ViaDashboard = "dashboard"
	ViaChat      = "chat"
)

var (
	// ErrNotFound means there was no reminder to delete with that ID, in the scope given
	ErrNotFound = errors.New("no such reminder")
	// ErrAlreadyDelivered means the reminder went off before it could be deleted
	ErrAlreadyDelivered = errors.New("the reminder has already gone off")
)

// TimeError says why the time of a reminder couldn't be read, as either a duration or a date and
// time
type TimeError struct {
	Param       string
	DurationErr error
	TimeErr     error
}

func (e *TimeError) Error() string {
	return fmt.Sprintf("invalid date/time or duration: %s (duration error: %s, date/time error: %s)", e.Param, e.DurationErr, e.TimeErr)
}

// Init initializes the reminder system
func Init(ctx context.Context) {
	reminders, err := db.GetReminders(ctx)
	if err != nil {
		log.Error().Ctx(ctx).Err(err).Msg("Failed to get reminders during init")
		return
	}
	metrics.SetRemindersActive(len(reminders))
	for _, r := range reminders {
		StartReminder(ctx, r)
	}
}

// Create saves a new reminder and starts its timer, returning its ID
func Create(ctx context.Context, rem db.Reminder) (int64, error) {
	id, err := db.AddReminder(ctx, rem)
	if err != nil {
		return 0, err
	}
	rem.ID = id
	StartReminder(ctx, rem)
	metrics.RecordReminderCreated()
	return id, nil
}

// StartReminder starts a timer for the reminder. A reminder that is already due, such as one
// restored at startup after its time, goes off at once.
func StartReminder(ctx context.Context, rem db.Reminder) {
	log.Debug().
		Ctx(ctx).
		Int64("reminder_id", rem.ID).
		Str("user_id", rem.UserID).
		Str("room_id", rem.RoomID).
		Time("remind_time", rem.RemindTime).
		Msg("Starting reminder")

	deliverCtx, scheduledBy := deliveryContext(ctx)

	f := func() {
		// Deleted in the meantime
		if !scheduled.claim(rem.ID) {
			return
		}

		opts := []trace.SpanStartOption{
			trace.WithSpanKind(trace.SpanKindInternal),
			trace.WithAttributes(
				attribute.String("matrix.room_id", rem.RoomID),
				attribute.String("matrix.user_id", rem.UserID),
				attribute.Int64("siikabot.remind.id", rem.ID),
			),
		}
		if scheduledBy.SpanContext.IsValid() {
			opts = append(opts, trace.WithLinks(scheduledBy))
		}

		tracing.Run(deliverCtx, tracer, "remind.deliver", func(ctx context.Context) {
			matrix.SendFormattedMessage(ctx, rem.RoomID, "<a href=\"https://matrix.to/#/"+rem.UserID+"\">"+matrix.GetDisplayName(ctx, rem.UserID)+"</a> "+rem.Message)
			if err := db.RemoveReminder(ctx, rem.ID); err != nil {
				log.Error().Ctx(ctx).Err(err).Int64("id", rem.ID).Msg("Failed to remove triggered reminder")
			}
			metrics.RecordReminderTriggered()
			log.Debug().
				Ctx(ctx).
				Str("user_id", rem.UserID).
				Str("room_id", rem.RoomID).
				Msg("Reminder triggered")
		}, opts...)
	}
	scheduled.add(rem.ID, time.Until(rem.RemindTime), f)
}

// Delete deletes a reminder within scope and stops its timer, and returns the deleted reminder.
// It returns ErrNotFound if there's no such reminder in scope, and ErrAlreadyDelivered if it went
// off first.
func Delete(ctx context.Context, scope db.ReminderScope, id int64, via string) (db.Reminder, error) {
	rem, err := db.DeleteReminder(ctx, scope, id)
	if err != nil {
		return db.Reminder{}, err
	}
	if rem == nil {
		return db.Reminder{}, ErrNotFound
	}

	// A reminder whose timer isn't waiting has been claimed for sending, unless it isn't due yet,
	// which means it was never scheduled. Deleting its row did no harm either way: the delivery's own
	// delete finds nothing.
	if !scheduled.cancel(id) && !rem.RemindTime.After(time.Now()) {
		log.Info().
			Ctx(ctx).
			Int64("reminder_id", id).
			Str("user_id", rem.UserID).
			Str("room_id", rem.RoomID).
			Str("via", via).
			Msg("Reminder went off before it could be deleted")
		return *rem, ErrAlreadyDelivered
	}

	metrics.RecordReminderDeleted(via)
	log.Info().
		Ctx(ctx).
		Int64("reminder_id", id).
		Str("user_id", rem.UserID).
		Str("room_id", rem.RoomID).
		Time("remind_time", rem.RemindTime).
		Str("via", via).
		Msg("Reminder deleted")
	return *rem, nil
}

// Location is the timezone reminders are shown in
func Location() *time.Location {
	loc, err := time.LoadLocation(config.Timezone)
	if err != nil {
		return time.UTC
	}
	return loc
}

// Describe describes a reminder as the bot's messages about it do: "at 09:00:00 on 20.11.2026
// (in 6d23h0m0s): <message>", in the bot's timezone, with the message as HTML
func Describe(rem db.Reminder, now time.Time) string {
	return "at " + rem.RemindTime.In(Location()).Format("15:04:05 on 2.1.2006") +
		" (in " + rem.RemindTime.Sub(now).Truncate(time.Second).String() + "): " + rem.Message
}

// UndoCommand is the !remind command that sets the reminder again as it was: for the same time,
// to the second, with the same message. The time carries its offset, so that it stays exact during
// the hour that repeats when the clocks go back.
func UndoCommand(rem db.Reminder) string {
	return "!remind " + rem.RemindTime.In(Location()).Truncate(time.Second).Format(time.RFC3339) + " " + rem.Message
}

// DeletionNotice is the notice posted when the chat deletes a reminder: what was deleted, and the
// command that sets it again
func DeletionNotice(rem db.Reminder, now time.Time) string {
	return "[AI tool call] Deleted the reminder " + Describe(rem, now) +
		"<br>To undo this, send this message: <code>" + html.EscapeString(UndoCommand(rem)) + "</code>"
}

// Handle handles the remind command
func Handle(ctx context.Context, roomID, sender, msg, msgType, formattedBody string) {
	params := strings.SplitN(msg, " ", 3)
	if len(params) < 3 {
		matrix.SendMessage(ctx, roomID, "Usage: !remind <time, date, datetime or duration> <message>")
		return
	}

	t := time.Now()
	reminderTime, err := ParseTime(t, params[1])
	if timeErr, ok := errors.AsType[*TimeError](err); ok {
		matrix.SendFormattedMessage(ctx, roomID, "Invalid date/time or duration: "+params[1]+"<br>duration error: "+timeErr.DurationErr.Error()+"<br> date/time error: "+timeErr.TimeErr.Error())
		return
	}

	rem := db.Reminder{
		RemindTime: reminderTime,
		UserID:     sender,
		RoomID:     roomID,
		Message:    messageText(msg, msgType, formattedBody),
	}

	id, err := Create(ctx, rem)
	if err != nil {
		matrix.SendMessage(ctx, roomID, "Failed to save reminder: "+err.Error())
		return
	}
	rem.ID = id

	duration := reminderTime.Sub(t).Truncate(time.Second)

	log.Info().
		Ctx(ctx).
		Int64("reminder_id", id).
		Str("room_id", roomID).
		Str("sender", sender).
		Time("remind_time", reminderTime).
		Str("duration", duration.String()).
		Msg("Reminder set")

	matrix.SendFormattedMessage(ctx, roomID, "Reminding "+Describe(rem, t))
}

// messageText is the message of a reminder set with !remind: the formatted body after the time when
// there is one, and otherwise the plain body after the time, with its line breaks as <br>
func messageText(msg, msgType, formattedBody string) string {
	formattedParams := strings.SplitN(formattedBody, " ", 3)
	if msgType == "org.matrix.custom.html" && len(formattedParams) >= 3 {
		return formattedParams[2]
	}
	return strings.Replace(strings.SplitN(msg, " ", 3)[2], "\n", "<br>", -1)
}

// ParseTime parses when a reminder goes off: a duration from now, or else a date, a time or both.
// It returns a *TimeError if the parameter is neither.
func ParseTime(now time.Time, param string) (time.Time, error) {
	reminderTime, durationErr := RemindDuration(now, param)
	if durationErr == nil {
		return reminderTime, nil
	}
	reminderTime, timeErr := RemindTime(now, param)
	if timeErr != nil {
		return time.Time{}, &TimeError{Param: param, DurationErr: durationErr, TimeErr: timeErr}
	}
	return reminderTime, nil
}

// RemindDuration parses a duration string and returns a time in the future
func RemindDuration(now time.Time, param string) (time.Time, error) {
	duration, durationErr := time.ParseDuration(param)
	if durationErr != nil {
		return time.Unix(0, 0), durationErr
	}

	if int64(duration/time.Second) < 1 {
		return time.Unix(0, 0), errors.New("duration must be at least 1s")
	}

	return now.Add(duration), nil
}

// RemindTime parses a time string and returns a time in the future
func RemindTime(now time.Time, param string) (time.Time, error) {
	param = strings.Replace(param, "_", "-", -1)
	var reminderTime time.Time
	loc, err := time.LoadLocation(config.Timezone)
	for _, f := range dateTimeFormatsTZ {
		reminderTime, err = time.Parse(f, param)
		if err == nil {
			break
		}
	}
	if err != nil {
		for _, f := range dateTimeFormats {
			reminderTime, err = time.Parse(f, param)
			if err == nil {
				reminderTime = time.Date(reminderTime.Year(), reminderTime.Month(), reminderTime.Day(), reminderTime.Hour(), reminderTime.Minute(), reminderTime.Second(), 0, loc)
				break
			}
		}
	}
	if err != nil {
		for _, f := range timeFormats {
			reminderTime, err = time.Parse(f, param)
			if err == nil {
				reminderTime = time.Date(now.Year(), now.Month(), now.Day(), reminderTime.Hour(), reminderTime.Minute(), reminderTime.Second(), 0, loc)
				if reminderTime.Unix() <= now.Unix() {
					reminderTime = reminderTime.Add(24 * time.Hour)
				}
				break
			}
		}
	}
	if err != nil {
		for _, f := range dateFormats {
			reminderTime, err = time.Parse(f, param)
			if err == nil {
				reminderTime = time.Date(reminderTime.Year(), reminderTime.Month(), reminderTime.Day(), 9, 0, 0, 0, loc)
				break
			}
		}
	}
	if err != nil {
		formats := "<br>valid date/time formats:<br>" +
			strings.Join(dateTimeFormats, "<br>") + "<br>" +
			strings.Join(dateTimeFormatsTZ, "<br>") + "<br>" +
			strings.Join(timeFormats, "<br>") + "<br>" +
			strings.Join(dateFormats, "<br>")
		return time.Unix(0, 0), errors.New("invalid date/time. Valid formats: " + formats)
	}
	if reminderTime.Unix() <= now.Unix() {
		return time.Unix(0, 0), errors.New("reminder date/time must be in future")
	}
	return reminderTime, nil
}
