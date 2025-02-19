package remind

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/Scrin/siikabot/db"
	"github.com/Scrin/siikabot/matrix"
	"github.com/rs/zerolog/log"
)

const timezone = "Europe/Helsinki"

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

// Init initializes the reminder system
func Init() {
	ctx := context.Background()
	reminders, err := db.GetReminders(ctx)
	if err != nil {
		log.Error().Err(err).Msg("Failed to get reminders during init")
		return
	}
	for _, r := range reminders {
		startReminder(r)
	}
}

func startReminder(rem db.Reminder) {
	log.Debug().
		Str("user_id", rem.UserID).
		Str("room_id", rem.RoomID).
		Time("remind_time", rem.RemindTime).
		Msg("Starting reminder")

	f := func() {
		matrix.SendFormattedMessage(rem.RoomID, "<a href=\"https://matrix.to/#/"+rem.UserID+"\">"+matrix.GetDisplayName(rem.UserID)+"</a> "+rem.Message)
		ctx := context.Background()
		if err := db.RemoveReminder(ctx, rem.ID); err != nil {
			log.Error().Err(err).Int64("id", rem.ID).Msg("Failed to remove triggered reminder")
		}
		log.Debug().
			Str("user_id", rem.UserID).
			Str("room_id", rem.RoomID).
			Msg("Reminder triggered")
	}
	duration := time.Until(rem.RemindTime)
	if duration <= 0 {
		f()
	} else {
		time.AfterFunc(duration, f)
	}
}

// Handle handles the remind command
func Handle(roomID, sender, msg, msgType, formattedBody string) {
	params := strings.SplitN(msg, " ", 3)
	if len(params) < 3 {
		matrix.SendMessage(roomID, "Usage: !remind <time, date, datetime or duration> <message>")
		return
	}

	t := time.Now()
	reminderTime, durationErr := remindDuration(t, params[1])
	var timeErr error
	if durationErr != nil {
		reminderTime, timeErr = remindTime(t, params[1])
	}
	if timeErr != nil {
		matrix.SendFormattedMessage(roomID, "Invalid date/time or duration: "+params[1]+"<br>duration error: "+durationErr.Error()+"<br> date/time error: "+timeErr.Error())
		return
	}

	formattedParams := strings.SplitN(formattedBody, " ", 3)
	var reminderText string
	if msgType == "org.matrix.custom.html" && len(formattedParams) >= 3 {
		reminderText = formattedParams[2]
	} else {
		reminderText = strings.Replace(params[2], "\n", "<br>", -1)
	}

	rem := db.Reminder{
		RemindTime: reminderTime,
		UserID:     sender,
		RoomID:     roomID,
		Message:    reminderText,
	}

	ctx := context.Background()
	id, err := db.AddReminder(ctx, rem)
	if err != nil {
		matrix.SendMessage(roomID, "Failed to save reminder: "+err.Error())
		return
	}
	rem.ID = id

	startReminder(rem)
	duration := reminderTime.Sub(t).Truncate(time.Second)
	loc, _ := time.LoadLocation(timezone)

	log.Info().
		Str("room_id", roomID).
		Str("sender", sender).
		Time("remind_time", reminderTime).
		Str("duration", duration.String()).
		Msg("Reminder set")

	matrix.SendFormattedMessage(roomID, "Reminding at "+reminderTime.In(loc).Format("15:04:05 on 2.1.2006")+" (in "+duration.String()+"): "+reminderText)
}

func remindDuration(now time.Time, param string) (time.Time, error) {
	duration, durationErr := time.ParseDuration(param)
	if durationErr != nil {
		return time.Unix(0, 0), durationErr
	}

	if int64(duration/time.Second) < 1 {
		return time.Unix(0, 0), errors.New("duration must be at least 1s")
	}

	return now.Add(duration), nil
}

func remindTime(now time.Time, param string) (time.Time, error) {
	param = strings.Replace(param, "_", "-", -1)
	var reminderTime time.Time
	loc, err := time.LoadLocation(timezone)
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
