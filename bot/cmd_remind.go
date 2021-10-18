package bot

import (
	"encoding/json"
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

var dateTimeFormats = []string{
	"2.1.2006-15:04", "15:04-2.1.2006",
	"2.1.2006_15:04", "15:04_2.1.2006",
	"2006-01-02_15:04", "15:04_2006-01-02"}
var dateTimeFormatsTZ = []string{
	time.ANSIC, time.UnixDate,
	time.RFC822, time.RFC822Z,
	time.RFC1123, time.RFC1123Z,
	time.RFC3339, time.RFC3339Nano}
var timeFormats = []string{"15:04"}
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
	t := time.Now()
	params := strings.SplitN(msg, " ", 3)
	if len(params) < 3 {
		client.SendMessage(roomID, "Usage: !remind <time, date, datetime or duration> <message>")
		return
	}
	duration, durationErr := time.ParseDuration(params[1])
	if durationErr == nil {
		durationSeconds := int64(duration / time.Second)
		if durationSeconds <= 0 {
			client.SendMessage(roomID, "Duration must be at least 1s")
			return
		}
		reminder := reminder{t.Unix() + durationSeconds, sender, roomID, params[2]}
		startReminder(reminder)
		saveReminders(append(getReminders(), reminder))
		client.SendMessage(roomID, "Reminding in "+duration.String()+": "+params[2])
	} else {
		var reminderTime time.Time
		loc, err := time.LoadLocation("Europe/Helsinki")
		for _, f := range dateTimeFormatsTZ {
			reminderTime, err = time.Parse(f, params[1])
			if err == nil {
				break
			}
		}
		if err != nil {
			for _, f := range dateTimeFormats {
				reminderTime, err = time.Parse(f, params[1])
				if err == nil {
					reminderTime = time.Date(reminderTime.Year(), reminderTime.Month(), reminderTime.Day(), reminderTime.Hour(), reminderTime.Minute(), reminderTime.Second(), reminderTime.Nanosecond(), loc)
					break
				}
			}
		}
		if err != nil {
			for _, f := range timeFormats {
				reminderTime, err = time.Parse(f, params[1])
				if err == nil {
					reminderTime = time.Date(t.Year(), t.Month(), t.Day(), reminderTime.Hour(), reminderTime.Minute(), reminderTime.Second(), t.Nanosecond(), loc)
					break
				}
			}
		}
		if err != nil {
			for _, f := range dateFormats {
				reminderTime, err = time.Parse(f, params[1])
				if err == nil {
					reminderTime = time.Date(reminderTime.Year(), reminderTime.Month(), reminderTime.Day(), t.Hour(), t.Minute(), t.Second(), t.Nanosecond(), loc)
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
			client.SendFormattedMessage(roomID, "Invalid date/time or duration: "+params[1]+"<br>duration error: "+durationErr.Error()+formats)
			return
		}
		if reminderTime.Unix() <= t.Unix() {
			client.SendMessage(roomID, "Reminder date/time must be in future")
			return
		}
		reminder := reminder{reminderTime.Unix(), sender, roomID, params[2]}
		startReminder(reminder)
		saveReminders(append(getReminders(), reminder))
		client.SendMessage(roomID, "Reminding at "+reminderTime.String()+": "+params[2])
	}
}
