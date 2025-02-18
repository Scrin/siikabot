package matrix

import (
	"encoding/json"
	"html"
	"log"
	"strings"
	"time"

	strip "github.com/grokify/html-strip-tags-go"
	"github.com/matrix-org/gomatrix"
)

var (
	userID         string
	client         *gomatrix.Client
	outboundEvents chan outboundEvent
)

type outboundEvent struct {
	RoomID         string
	EventType      string
	Content        interface{}
	RetryOnFailure bool
	done           chan<- string
}

type simpleMessage struct {
	MsgType       string `json:"msgtype"`
	Body          string `json:"body"`
	Format        string `json:"format,omitempty"`
	FormattedBody string `json:"formatted_body,omitempty"`
}

type messageEdit struct {
	MsgType       string `json:"msgtype"`
	Body          string `json:"body"`
	Format        string `json:"format,omitempty"`
	FormattedBody string `json:"formatted_body,omitempty"`
	NewContent    struct {
		MsgType       string `json:"msgtype"`
		Body          string `json:"body"`
		Format        string `json:"format,omitempty"`
		FormattedBody string `json:"formatted_body,omitempty"`
	} `json:"m.new_content"`
	RelatesTo struct {
		RelType string `json:"rel_type"`
		EventID string `json:"event_id"`
	} `json:"m.relates_to"`
}

type httpError struct {
	Errcode      string `json:"errcode"`
	Err          string `json:"error"`
	RetryAfterMs int    `json:"retry_after_ms"`
}

func sendMessage(roomID string, message interface{}, retryOnFailure bool) <-chan string {
	done := make(chan string, 1)
	outboundEvents <- outboundEvent{roomID, "m.room.message", message, retryOnFailure, done}
	return done
}

// InitialSync gets the initial sync from the server for catching up with important missed event such as invites
func InitialSync() *gomatrix.RespSync {
	resp, err := client.SyncRequest(0, "", "", false, "")
	if err != nil {
		log.Fatal(err)
	}
	return resp
}

// Sync begins synchronizing the events from the server and returns only in case of a severe error
func Sync() error {
	return client.Sync()
}

func OnEvent(eventType string, callback gomatrix.OnEventListener) {
	client.Syncer.(*gomatrix.DefaultSyncer).OnEventType(eventType, callback)
}

func JoinRoom(roomID string) {
	_, err := client.JoinRoom(roomID, "", nil)
	if err != nil {
		log.Println("Failed to join room "+roomID+": ", err)
	}
}

func GetDisplayName(mxid string) string {
	foo, err := client.GetDisplayName(mxid)
	if err != nil {
		log.Println(err)
	}
	return foo.DisplayName
}

// SendMessage queues a message to be sent and returns immediatedly.
//
// The returned channel will provide the event ID of the message after the message has been sent
func SendMessage(roomID string, message string) <-chan string {
	return sendMessage(roomID, simpleMessage{"m.text", message, "", ""}, true)
}

// SendFormattedMessage queues a html-formatted message to be sent and returns immediatedly.
//
// The returned channel will provide the event ID of the message after the message has been sent
func SendFormattedMessage(roomID string, message string) <-chan string {
	return sendMessage(roomID, simpleMessage{"m.text", stripFormatting(message), "org.matrix.custom.html", message}, true)
}

// SendNotice queues a notice to be sent and returns immediatedly.
//
// The returned channel will provide the event ID of the notice after the notice has been sent
func SendNotice(roomID string, notice string) <-chan string {
	return sendMessage(roomID, simpleMessage{"m.notice", notice, "", ""}, true)
}

// SendFormattedNotice queues a html-formatted notice to be sent and returns immediatedly.
//
// The returned channel will provide the event ID of the notice after the notice has been sent
func SendFormattedNotice(roomID string, notice string) <-chan string {
	return sendMessage(roomID, simpleMessage{"m.notice", stripFormatting(notice), "org.matrix.custom.html", notice}, true)
}

func stripFormatting(s string) string {
	// paragraph and header tags are on their own lines
	s = strings.Replace(s, "<p>", "\n", -1)
	s = strings.Replace(s, "<h1>", "\n", -1)
	s = strings.Replace(s, "<h2>", "\n", -1)
	s = strings.Replace(s, "<h3>", "\n", -1)
	s = strings.Replace(s, "<h4>", "\n", -1)
	s = strings.Replace(s, "<h5>", "\n", -1)
	s = strings.Replace(s, "<h6>", "\n", -1)
	s = strings.Replace(s, "</p>", "\n", -1)
	s = strings.Replace(s, "</h1>", "\n", -1)
	s = strings.Replace(s, "</h2>", "\n", -1)
	s = strings.Replace(s, "</h3>", "\n", -1)
	s = strings.Replace(s, "</h4>", "\n", -1)
	s = strings.Replace(s, "</h5>", "\n", -1)
	s = strings.Replace(s, "</h6>", "\n", -1)
	// beginning of every list element means beginning of a new line, break the line at the end of the list
	s = strings.Replace(s, "<li>", "\n - ", -1)
	s = strings.Replace(s, "</ul>", "\n", -1)
	// table cells have a space between them and row end ends the line
	s = strings.Replace(s, "</td>", " ", -1)
	s = strings.Replace(s, "</tr>", "\n", -1)
	// duh
	s = strings.Replace(s, "<br>", "\n", -1)
	s = strings.Replace(s, "<br/>", "\n", -1)
	s = strings.Replace(s, "<br />", "\n", -1)
	return strip.StripTags(html.UnescapeString(s))
}

// SendStreamingMessage creates a pair of channels that can be used to send and update (by editing) a message in place.
//
// The initial message will be sent when messageUpdate receives the first message. The message will be
// updated until done is closed, at which point messageUpdate will be drained and the last version be updated.
func SendStreamingMessage(roomID string) (messageUpdate chan<- string, done chan<- struct{}) {
	return sendStreaming(roomID, false, "m.text")
}

// SendStreamingFormattedMessage creates a pair of channels that can be used to send and update (by editing) a formatted message in place.
//
// The initial message will be sent when messageUpdate receives the first message. The message will be
// updated until done is closed, at which point messageUpdate will be drained and the last version be updated.
func SendStreamingFormattedMessage(roomID string) (messageUpdate chan<- string, done chan<- struct{}) {
	return sendStreaming(roomID, true, "m.text")
}

// SendStreamingNotice creates a pair of channels that can be used to send and update (by editing) a notice in place.
//
// The initial notice will be sent when noticeUpdate receives the first notice. The notice will be
// updated until done is closed, at which point noticeUpdate will be drained and the last version be updated.
func SendStreamingNotice(roomID string) (noticeUpdate chan<- string, done chan<- struct{}) {
	return sendStreaming(roomID, false, "m.notice")
}

// SendStreamingFormattedNotice creates a pair of channels that can be used to send and update (by editing) a formatted notice in place.
//
// The initial notice will be sent when noticeUpdate receives the first notice. The notice will be
// updated until done is closed, at which point noticeUpdate will be drained and the last version be updated.
func SendStreamingFormattedNotice(roomID string) (noticeUpdate chan<- string, done chan<- struct{}) {
	return sendStreaming(roomID, true, "m.notice")
}

func sendStreaming(roomID string, formatted bool, msgType string) (messageUpdate chan<- string, done chan<- struct{}) {
	input := make(chan string, 256)
	doneChan := make(chan struct{})
	go func() {
		text := <-input
		var id string
		if formatted {
			id = <-sendMessage(roomID, simpleMessage{msgType, stripFormatting(text), "org.matrix.custom.html", text}, true)
		} else {
			id = <-sendMessage(roomID, simpleMessage{msgType, text, "", ""}, true)
		}
		msgEdit := messageEdit{}
		if formatted {
			msgEdit.Body = stripFormatting(text)
			msgEdit.FormattedBody = text
			msgEdit.Format = "org.matrix.custom.html"
			msgEdit.NewContent.Body = stripFormatting(text)
			msgEdit.NewContent.FormattedBody = text
			msgEdit.NewContent.Format = "org.matrix.custom.html"
		} else {
			msgEdit.Body = text
			msgEdit.NewContent.Body = text
		}
		msgEdit.MsgType = msgType
		msgEdit.NewContent.MsgType = msgType
		msgEdit.RelatesTo.RelType = "m.replace"
		msgEdit.RelatesTo.EventID = id
		messageDone := false
		for !messageDone {
			select { // Wait for more input or done signal
			case m := <-input:
				if formatted {
					msgEdit.Body = stripFormatting(m)
					msgEdit.FormattedBody = m
					msgEdit.NewContent.Body = stripFormatting(m)
					msgEdit.NewContent.FormattedBody = m
				} else {
					msgEdit.Body = m
					msgEdit.NewContent.Body = m
				}
			case <-doneChan:
				messageDone = true
			}
			for messages := true; messages; { // drain the input in case done was signaled
				select {
				case m := <-input:
					if formatted {
						msgEdit.Body = stripFormatting(m)
						msgEdit.FormattedBody = m
						msgEdit.NewContent.Body = stripFormatting(m)
						msgEdit.NewContent.FormattedBody = m
					} else {
						msgEdit.Body = m
						msgEdit.NewContent.Body = m
					}
				default:
					messages = false
				}
			}
			res := <-sendMessage(roomID, msgEdit, messageDone)
			if res == "" { // no event id, send failed, wait for a bit before retrying
				time.Sleep(100 * time.Millisecond)
			}
		}

	}()
	return input, doneChan
}

func processOutboundEvents() {
	for event := range outboundEvents {
	retry:
		for {
			resp, err := client.SendMessageEvent(event.RoomID, event.EventType, event.Content)
			if err == nil {
				if event.done != nil {
					event.done <- resp.EventID
				}
				break // Success, break the retry loop
			}
			var httpErr httpError
			httpError, isHttpError := err.(gomatrix.HTTPError)
			if !isHttpError {
				log.Print("Failed to parse error response of unexpected type!", err)
				event.done <- ""
				break
			}
			if jsonErr := json.Unmarshal(httpError.Contents, &httpErr); jsonErr != nil {
				log.Print("Failed to parse error response!", jsonErr)
			}

			switch e := httpErr.Errcode; e {
			case "M_LIMIT_EXCEEDED":
				time.Sleep(time.Duration(httpErr.RetryAfterMs) * time.Millisecond)
			case "M_FORBIDDEN":
				log.Print("Failed to send message to room "+event.RoomID+" err: ", err)
				log.Print(string(err.(gomatrix.HTTPError).Contents))
				event.done <- ""
				break retry
			default:
				log.Print("Failed to send message to room "+event.RoomID+" err: ", err)
				log.Print(string(err.(gomatrix.HTTPError).Contents))
			}
			if !event.RetryOnFailure {
				event.done <- ""
				break
			}
		}
	}
}

// GetUserID returns the ID of the currently logged in user
func GetUserID() string {
	return userID
}

// Init initializes the Matrix client with the given parameters
func Init(homeserverURL, uid, accessToken string) error {
	var err error
	client, err = gomatrix.NewClient(homeserverURL, uid, accessToken)
	if err != nil {
		return err
	}
	userID = uid
	outboundEvents = make(chan outboundEvent, 256)
	go processOutboundEvents()
	return nil
}
