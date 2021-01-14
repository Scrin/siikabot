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
		client.SendFormattedMessage(rem.RoomID, "<a href=\"https://matrix.to/#/"+rem.User+"\">"+rem.User+"</a> "+rem.Message)
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
		client.SendMessage(roomID, "Usage: !remind <rfc3339 or duration> <message>")
		return
	}
	duration, durationErr := time.ParseDuration(params[1])
	if durationErr == nil {
		durationSeconds := int64(duration / time.Second)
		if durationSeconds <= 0 {
			client.SendMessage(roomID, "Duration must be at least 1s")
			return
		}
		reminder := reminder{time.Now().Unix() + durationSeconds, sender, roomID, params[2]}
		startReminder(reminder)
		saveReminders(append(getReminders(), reminder))
		client.SendMessage(roomID, "Reminding in "+duration.String()+": "+params[2])
	} else {
		reminderTime, err := time.Parse(time.RFC3339, params[1])
		if err != nil {
			client.SendFormattedMessage(roomID, "Invalid date/time or duration: "+params[1]+"<br>duration: "+durationErr.Error()+"<br>RFC3339 date/time: "+err.Error())
			return
		}
		if reminderTime.Unix() <= time.Now().Unix() {
			client.SendMessage(roomID, "Reminder date/time must be in future")
			return
		}
		reminder := reminder{reminderTime.Unix(), sender, roomID, params[2]}
		startReminder(reminder)
		saveReminders(append(getReminders(), reminder))
		client.SendMessage(roomID, "Reminding at "+reminderTime.String()+": "+params[2])
	}
}
