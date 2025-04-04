package matrix

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/Scrin/siikabot/config"
	"github.com/rs/zerolog/log"
	"maunium.net/go/mautrix"
	"maunium.net/go/mautrix/crypto"
	"maunium.net/go/mautrix/event"
	"maunium.net/go/mautrix/id"
)

var (
	client      *mautrix.Client
	olmMachine  *crypto.OlmMachine
	stateStore  *StateStore
	syncStore   *SyncStore
	cryptoStore *CryptoStore

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
	MsgType       string         `json:"msgtype"`
	Body          string         `json:"body"`
	Format        string         `json:"format,omitempty"`
	FormattedBody string         `json:"formatted_body,omitempty"`
	DebugData     map[string]any `json:"fi.2kgwf.debug,omitempty"`
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

func JoinRoom(ctx context.Context, roomID string) {
	_, err := client.JoinRoom(ctx, roomID, &mautrix.ReqJoinRoom{})
	if err != nil {
		log.Error().Err(err).Str("room_id", roomID).Msg("Failed to join room")
	}
}

func GetDisplayName(ctx context.Context, mxid string) string {
	dn, err := client.GetDisplayName(ctx, id.UserID(mxid))
	if err != nil {
		log.Error().Err(err).Str("user_id", mxid).Msg("Failed to get display name")
	}
	if dn == nil {
		return mxid
	}
	return dn.DisplayName
}

func processOutboundEvents(ctx context.Context) {
outboundProcessingLoop:
	for evt := range outboundEvents {
		roomId := id.RoomID(evt.RoomID)
		evtType := event.NewEventType(evt.EventType)
		evtContent := evt.Content

	encryptionLoop:
		for {
			isEncrypted, err := stateStore.IsEncrypted(ctx, roomId)

			if err != nil {
				log.Error().Ctx(ctx).Err(err).Str("room_id", evt.RoomID).Msg("Failed to check if room is encrypted")
				if !evt.RetryOnFailure {
					continue outboundProcessingLoop
				}
				time.Sleep(100 * time.Millisecond)
				continue encryptionLoop
			}

			if isEncrypted {
				encrypted, err := olmMachine.EncryptMegolmEvent(ctx, roomId, evtType, evtContent)
				// These three errors mean we have to make a new Megolm session
				if err == crypto.SessionExpired || err == crypto.SessionNotShared || err == crypto.NoGroupSession {
					members, err := stateStore.GetRoomMembers(ctx, roomId)
					if err != nil {
						log.Error().Ctx(ctx).Err(err).Str("room_id", evt.RoomID).Msg("Failed to get room members")
						if !evt.RetryOnFailure {
							continue outboundProcessingLoop
						}
						time.Sleep(100 * time.Millisecond)
						continue encryptionLoop
					}
					err = olmMachine.ShareGroupSession(ctx, roomId, members)
					if err != nil {
						log.Error().Ctx(ctx).Err(err).Str("room_id", evt.RoomID).Msg("Failed to share group session")
						if !evt.RetryOnFailure {
							continue outboundProcessingLoop
						}
						time.Sleep(100 * time.Millisecond)
						continue encryptionLoop
					}
					encrypted, err = olmMachine.EncryptMegolmEvent(ctx, roomId, evtType, evtContent)
				}

				if err != nil {
					log.Error().Ctx(ctx).Err(err).Str("room_id", evt.RoomID).Msg("Failed to encrypt message")
					if !evt.RetryOnFailure {
						continue outboundProcessingLoop
					}
					time.Sleep(100 * time.Millisecond)
					continue encryptionLoop
				}
				evtType = event.EventEncrypted
				evtContent = encrypted
				break
			} else {
				break
			}
		}

	retry:
		for {
			resp, err := client.SendMessageEvent(ctx, roomId, evtType, evtContent)
			if err == nil {
				if evt.done != nil {
					evt.done <- string(resp.EventID)
				}
				break // Success, break the retry loop
			}
			var httpErr httpError
			httpError, isHttpError := err.(mautrix.HTTPError)
			if !isHttpError {
				log.Error().Ctx(ctx).Err(err).Msg("Failed to parse error response of unexpected type")
				evt.done <- ""
				break
			}
			if jsonErr := json.Unmarshal([]byte(httpError.ResponseBody), &httpErr); jsonErr != nil {
				log.Error().Ctx(ctx).Err(jsonErr).Msg("Failed to parse error response")
			}

			switch e := httpErr.Errcode; e {
			case "M_LIMIT_EXCEEDED":
				time.Sleep(time.Duration(httpErr.RetryAfterMs) * time.Millisecond)
			case "M_FORBIDDEN":
				log.Error().
					Ctx(ctx).
					Err(err).
					Str("room_id", evt.RoomID).
					Str("error_code", e).
					Msg("Failed to send message due to permissions")
				evt.done <- ""
				break retry
			default:
				log.Error().
					Ctx(ctx).
					Err(err).
					Str("room_id", evt.RoomID).
					Str("error_code", e).
					Msg("Failed to send message")
			}
			if !evt.RetryOnFailure {
				evt.done <- ""
				break
			}
		}
	}
}

func Init(ctx context.Context, handleEvent func(ctx context.Context, evt *event.Event, wasEncrypted bool)) error {
	var err error
	stateStore = NewStateStore()
	syncStore = NewSyncStore()
	cryptoStore = NewCryptoStore()

	client, err = mautrix.NewClient(config.HomeserverURL, "", "")
	if err != nil {
		return err
	}
	_, err = client.Login(ctx, &mautrix.ReqLogin{
		Type: mautrix.AuthTypePassword,
		Identifier: mautrix.UserIdentifier{
			Type: mautrix.IdentifierTypeUser,
			User: config.UserID,
		},
		Password:                 config.Password,
		InitialDeviceDisplayName: "Siikabot",
		DeviceID:                 "Siikabot",
		StoreCredentials:         true,
	})
	if err != nil {
		return err
	}
	client.Store = syncStore

	olmMachine = crypto.NewOlmMachine(client, &log.Logger, cryptoStore, stateStore)
	err = olmMachine.Load(ctx)
	if err != nil {
		return err
	}

	client.Syncer.(mautrix.ExtensibleSyncer).OnSync(olmMachine.ProcessSyncResponse)

	syncer := client.Syncer.(*mautrix.DefaultSyncer)

	syncer.OnEventType(event.StateMember, func(ctx context.Context, evt *event.Event) {
		olmMachine.HandleMemberEvent(ctx, evt)
		stateStore.SetMembership(ctx, evt)
	})
	syncer.OnEventType(event.StateEncryption, func(ctx context.Context, evt *event.Event) {
		stateStore.SetEncryptionEvent(ctx, evt)
	})
	syncer.OnEventType(event.EventEncrypted, func(ctx context.Context, evt *event.Event) {
		decryptedEvent, err := olmMachine.DecryptMegolmEvent(ctx, evt)
		if err != nil {
			log.Error().Err(err).Str("room_id", evt.RoomID.String()).Str("sender", evt.Sender.String()).Msg("Failed to decrypt message")
		} else {
			log.Debug().Str("room_id", evt.RoomID.String()).Str("sender", evt.Sender.String()).Msg("Received encrypted event")
			if decryptedEvent.Type == event.EventMessage {
				handleEvent(ctx, decryptedEvent, true)
			}
		}
	})
	syncer.OnEvent(func(ctx context.Context, evt *event.Event) {
		handleEvent(ctx, evt, false)
	})

	outboundEvents = make(chan outboundEvent, 256)
	go processOutboundEvents(ctx)
	return nil
}

// InitialSync gets the initial sync from the server for catching up with important missed event such as invites
func InitialSync(ctx context.Context) *mautrix.RespSync {
	resp, err := client.SyncRequest(ctx, 0, "", "", false, "online")
	if err != nil {
		log.Fatal().Err(err).Msg("Failed to perform initial sync")
	}
	return resp
}

// Sync begins synchronizing the events from the server and returns only in case of a severe error
func Sync() error {
	return client.Sync()
}

// SendTyping sends a typing indicator to a room.
// If typing is true, the bot will appear as typing for the specified duration.
// If typing is false, the bot will stop appearing as typing.
func SendTyping(ctx context.Context, roomID string, typing bool, timeout time.Duration) {
	_, err := client.UserTyping(ctx, id.RoomID(roomID), typing, timeout)
	if err != nil {
		log.Error().Ctx(ctx).Err(err).Str("room_id", roomID).Bool("typing", typing).Msg("Failed to send typing indicator")
	}
}

// MarkRead marks a message as read by the bot.
// This updates the read receipt for the bot in the room.
func MarkRead(ctx context.Context, roomID string, eventID string) {
	err := client.MarkRead(ctx, id.RoomID(roomID), id.EventID(eventID))
	if err != nil {
		log.Error().Ctx(ctx).Err(err).Str("room_id", roomID).Str("event_id", eventID).Msg("Failed to mark message as read")
	}
}

// GetEventContent retrieves the content of a message by its event ID.
// Returns the body of the message as a string.
func GetEventContent(ctx context.Context, roomID string, eventID string) (string, error) {
	// Get the event from the server
	evt, err := client.GetEvent(ctx, id.RoomID(roomID), id.EventID(eventID))
	if err != nil {
		log.Error().Ctx(ctx).Err(err).Str("room_id", roomID).Str("event_id", eventID).Msg("Failed to get event content")
		return "", err
	}

	// Check if the event is encrypted and decrypt it if necessary
	if evt.Type == event.EventEncrypted {
		log.Debug().Ctx(ctx).
			Str("room_id", roomID).
			Str("event_id", eventID).
			Msg("Attempting to decrypt event")

		err = evt.Content.ParseRaw(evt.Type)
		if err != nil {
			log.Error().Ctx(ctx).Err(err).
				Str("room_id", roomID).
				Str("event_id", eventID).
				Msg("Failed to parse encrypted event content")
		}

		// Try to decrypt the event using OlmMachine
		decryptedEvt, err := olmMachine.DecryptMegolmEvent(ctx, evt)
		if err != nil {
			// If we can't decrypt it, log the error and return a specific error
			log.Error().Ctx(ctx).Err(err).
				Str("room_id", roomID).
				Str("event_id", eventID).
				Msg("Failed to decrypt event")
			return "", fmt.Errorf("cannot decrypt encrypted event: %w", err)
		}

		// Use the decrypted event
		evt = decryptedEvt
	}

	// Check if the event is a message
	if evt.Type != event.EventMessage {
		return "", fmt.Errorf("event is not a message (type: %s)", evt.Type)
	}

	// Extract the message body
	if body, ok := evt.Content.Raw["body"].(string); ok {
		return body, nil
	}

	// Log the content for debugging
	log.Debug().Ctx(ctx).
		Str("room_id", roomID).
		Str("event_id", eventID).
		Interface("content", evt.Content.Raw).
		Msg("Message content does not contain body")

	return "", fmt.Errorf("message body not found")
}

// GetEventSender retrieves the sender of a message by its event ID.
func GetEventSender(ctx context.Context, roomID string, eventID string) (string, error) {
	// Get the event from the server
	evt, err := client.GetEvent(ctx, id.RoomID(roomID), id.EventID(eventID))
	if err != nil {
		log.Error().Ctx(ctx).Err(err).Str("room_id", roomID).Str("event_id", eventID).Msg("Failed to get event sender")
		return "", err
	}

	return evt.Sender.String(), nil
}

// GetRoomMembers returns a list of user IDs for all members in a room
func GetRoomMembers(ctx context.Context, roomID string) ([]string, error) {
	members, err := client.JoinedMembers(ctx, id.RoomID(roomID))
	if err != nil {
		return nil, err
	}

	var userIDs []string
	for userID := range members.Joined {
		userIDs = append(userIDs, string(userID))
	}
	return userIDs, nil
}
