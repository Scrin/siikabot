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

type Client struct {
	UserID         string
	client         *gomatrix.Client
	outboundEvents chan outboundEvent
}

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
	MsgType    string `json:"msgtype"`
	Body       string `json:"body"`
	NewContent struct {
		MsgType string `json:"msgtype"`
		Body    string `json:"body"`
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

func (c Client) sendMessage(roomID string, message interface{}, retryOnFailure bool) <-chan string {
	done := make(chan string, 1)
	c.outboundEvents <- outboundEvent{roomID, "m.room.message", message, retryOnFailure, done}
	return done
}

// InitialSync gets the initial sync from the server for catching up with important missed event such as invites
func (c Client) InitialSync() *gomatrix.RespSync {
	resp, err := c.client.SyncRequest(0, "", "", false, "")
	if err != nil {
		log.Fatal(err)
	}
	return resp
}

// Sync begins synchronizing the events from the server and returns only in case of a severe error
func (c Client) Sync() error {
	return c.client.Sync()
}

func (c Client) OnEvent(eventType string, callback gomatrix.OnEventListener) {
	c.client.Syncer.(*gomatrix.DefaultSyncer).OnEventType(eventType, callback)
}

func (c Client) JoinRoom(roomID string) {
	_, err := c.client.JoinRoom(roomID, "", nil)
	if err != nil {
		log.Print("Failed to join room "+roomID+": ", err)
	}
}

// SendMessage queues a message to be sent and returns immediatedly.
//
// The returned channel will provide the event ID of the message after the message has been sent
func (c Client) SendMessage(roomID string, message string) <-chan string {
	return c.sendMessage(roomID, simpleMessage{"m.text", message, "", ""}, true)
}

// SendFormattedMessage queues a html-formatted message to be sent and returns immediatedly.
//
// The returned channel will provide the event ID of the message after the message has been sent
func (c Client) SendFormattedMessage(roomID string, message string) <-chan string {
	return c.sendMessage(roomID, simpleMessage{"m.text", stripFormatting(message), "org.matrix.custom.html", message}, true)
}

// SendNotice queues a notice to be sent and returns immediatedly.
//
// The returned channel will provide the event ID of the notice after the notice has been sent
func (c Client) SendNotice(roomID string, notice string) <-chan string {
	return c.sendMessage(roomID, simpleMessage{"m.notice", notice, "", ""}, true)
}

// SendFormattedNotice queues a html-formatted notice to be sent and returns immediatedly.
//
// The returned channel will provide the event ID of the notice after the notice has been sent
func (c Client) SendFormattedNotice(roomID string, notice string) <-chan string {
	return c.sendMessage(roomID, simpleMessage{"m.notice", stripFormatting(notice), "org.matrix.custom.html", notice}, true)
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
func (c Client) SendStreamingMessage(roomID string) (messageUpdate chan<- string, done chan<- struct{}) {
	input := make(chan string, 256)
	doneChan := make(chan struct{})
	go func() {
		text := <-input
		id := <-c.SendMessage(roomID, text)
		msgEdit := messageEdit{}
		msgEdit.Body = text
		msgEdit.NewContent.Body = text
		msgEdit.MsgType = "m.text"
		msgEdit.NewContent.MsgType = "m.text"
		msgEdit.RelatesTo.RelType = "m.replace"
		msgEdit.RelatesTo.EventID = id
		messageDone := false
		for !messageDone {
			select { // Wait for more input or done signal
			case m := <-input:
				msgEdit.Body = m
				msgEdit.NewContent.Body = m
			case <-doneChan:
				messageDone = true
			}
			for messages := true; messages; { // drain the input in case done was signaled
				select {
				case m := <-input:
					msgEdit.Body = m
					msgEdit.NewContent.Body = m
				default:
					messages = false
				}
			}
			res := <-c.sendMessage(roomID, msgEdit, messageDone)
			if res == "" { // no event id, send failed, wait for a bit before retrying
				time.Sleep(100 * time.Millisecond)
			}
		}

	}()
	return input, doneChan
}

func processOutboundEvents(client Client) {
	for event := range client.outboundEvents {
	retry:
		for {
			resp, err := client.client.SendMessageEvent(event.RoomID, event.EventType, event.Content)
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

// NewClient creates a new Matrix client and performs basic initialization on it
func NewClient(homeserverURL, userID, accessToken string) Client {
	client, err := gomatrix.NewClient(homeserverURL, userID, accessToken)
	if err != nil {
		log.Fatal(err)
	}
	c := Client{
		userID,
		client,
		make(chan outboundEvent, 256),
	}
	go processOutboundEvents(c)
	return c
}
