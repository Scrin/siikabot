package matrix

import (
	"time"
)

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
			id = <-sendMessage(roomID, simpleMessage{msgType, stripFormatting(text), "org.matrix.custom.html", text, nil}, true)
		} else {
			id = <-sendMessage(roomID, simpleMessage{msgType, text, "", "", nil}, true)
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
