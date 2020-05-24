package matrix

import (
	"encoding/json"
	"log"
	"time"

	"github.com/matrix-org/gomatrix"
)

type OutboundEvent struct {
	RoomID         string
	EventType      string
	Content        interface{}
	RetryOnFailure bool
	done           chan<- string
}

type Client struct {
	UserID         string
	Client         *gomatrix.Client
	Syncer         *gomatrix.DefaultSyncer
	outboundEvents chan OutboundEvent
}

type simpleMessage struct {
	MsgType string `json:"msgtype"`
	Body    string `json:"body"`
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
	done := make(chan string)
	c.outboundEvents <- OutboundEvent{roomID, "m.room.message", message, retryOnFailure, done}
	return done
}

// SendMessage queues a message to be sent and returns immediatedly.
//
// The returned channel will provide the event ID of the message after the message has been sent
func (c Client) SendMessage(roomID string, message string) <-chan string {
	return c.sendMessage(roomID, simpleMessage{"m.text", message}, true)
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
		for {
			resp, err := client.Client.SendMessageEvent(event.RoomID, event.EventType, event.Content)
			if err == nil {
				if event.done != nil {
					event.done <- resp.EventID
				}
				break // Success, break the retry loop
			}
			var httpErr httpError
			if jsonErr := json.Unmarshal(err.(gomatrix.HTTPError).Contents, &httpErr); jsonErr != nil {
				log.Print("Failed to parse error response!", jsonErr)
			}

			switch e := httpErr.Errcode; e {
			case "M_LIMIT_EXCEEDED":
				time.Sleep(time.Duration(httpErr.RetryAfterMs) * time.Millisecond)
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
	syncer := client.Syncer.(*gomatrix.DefaultSyncer)
	c := Client{
		userID,
		client,
		syncer,
		make(chan OutboundEvent, 256),
	}
	go processOutboundEvents(c)
	return c
}
