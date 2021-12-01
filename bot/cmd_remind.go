package bot

import (
	"encoding/json"
	"errors"
	"log"
	"reflect"
	"strings"
	"time"
)

type reminder struct {
	RemindTime int64  `json:"remind_time"`
	User       string `json:"user"`
	RoomID     string `json:"room_id"`
	Message    string `json:"msg"`
}

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

func initReminder() {
	for _, r := range getReminders() {
		startReminder(r)
	}
}

func getReminders() []reminder {
	remindersJson := db.Get("reminders")
	var reminders []reminder
	if remindersJson != "" {
		json.Unmarshal([]byte(remindersJson), &reminders)
	}
	return reminders
}

func saveReminders(reminders []reminder) {
	res, err := json.Marshal(reminders)
	if err != nil {
		log.Print(err)
		return
	}
	db.Set("reminders", string(res))
}

func startReminder(rem reminder) {
	f := func() {
		client.SendFormattedMessage(rem.RoomID, "<a href=\"https://matrix.to/#/"+rem.User+"\">"+client.GetDisplayName(rem.User)+"</a> "+rem.Message)
		reminders := getReminders()
		var newReminders []reminder
		for _, r := range reminders {
			if !reflect.DeepEqual(rem, r) {
				newReminders = append(newReminders, r)
			}
		}
		saveReminders(newReminders)
	}
	duration := rem.RemindTime - time.Now().Unix()
	if duration <= 0 {
		f()
	} else {
		time.AfterFunc(time.Duration(duration)*time.Second, f)
	}
}

func remind(roomID, sender, msg string) {
	params := strings.SplitN(msg, " ", 3)
	if len(params) < 3 {
		client.SendMessage(roomID, "Usage: !remind <time, date, datetime or duration> <message>")
		return
	}

	t := time.Now()
	reminderTime, durationErr := remindDuration(t, params[1])
	var timeErr error
	if durationErr != nil {
		reminderTime, timeErr = remindTime(t, params[1])
	}
	if timeErr != nil {
		client.SendFormattedMessage(roomID, "Invalid date/time or duration: "+params[1]+"<br>duration error: "+durationErr.Error()+"<br> date/time error: "+timeErr.Error())
		return
	}

	rem := reminder{reminderTime.Unix(), sender, roomID, params[2]}
	startReminder(rem)
	saveReminders(append(getReminders(), rem))
	duration := reminderTime.Sub(t).Truncate(time.Second)
	loc, _ := time.LoadLocation(timezone)
	client.SendMessage(roomID, "Reminding at "+reminderTime.In(loc).Format("15:04:05 on 2.1.2006")+" (in "+duration.String()+"): "+params[2])
}

func remindDuration(now time.Time, param string) (time.Time, error) {

	duration, durationErr := time.ParseDuration(param)
	if durationErr != nil {
		return time.Unix(0, 0), durationErr
	}

	if int64(duration/time.Second) < 1 {
		return time.Unix(0, 0), errors.New("Duration must be at least 1s")
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
				reminderTime = time.Date(reminderTime.Year(), reminderTime.Month(), reminderTime.Day(), now.Hour(), now.Minute(), now.Second(), 0, loc)
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
		return time.Unix(0, 0), errors.New("Invalid date/time. Valid formats: " + formats)
	}
	if reminderTime.Unix() <= now.Unix() {
		return time.Unix(0, 0), errors.New("Reminder date/time must be in future")
	}
	return reminderTime, nil
}
