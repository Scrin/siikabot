package matrix

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/Scrin/siikabot/config"
	"github.com/Scrin/siikabot/constants"
	"github.com/Scrin/siikabot/logging"
	"github.com/Scrin/siikabot/metrics"
	"github.com/rs/zerolog/log"
	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
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
	ctx            context.Context
	RoomID         string
	EventType      string
	Content        simpleMessage
	RetryOnFailure bool
	done           chan<- string
	// target, when set, places the message and may cancel it, both just before it is sent
	target *Target
}

// content returns the message to send, placed at its target if it has one
func (e outboundEvent) content() simpleMessage {
	msg := e.Content
	if e.target != nil {
		msg.RelatesTo = e.target.relatesTo(e.RoomID)
	}
	return msg
}

type simpleMessage struct {
	MsgType       string           `json:"msgtype"`
	Body          string           `json:"body"`
	Format        string           `json:"format,omitempty"`
	FormattedBody string           `json:"formatted_body,omitempty"`
	RelatesTo     *event.RelatesTo `json:"m.relates_to,omitempty"`
	DebugData     map[string]any   `json:"fi.2kgwf.debug,omitempty"`
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

// displayNameTTL bounds how stale a cached display name may be. Display names change rarely and
// nothing here breaks if one is an hour out of date.
const displayNameTTL = time.Hour

type displayNameEntry struct {
	name      string
	fetchedAt time.Time
}

var displayNameCache = struct {
	sync.Mutex
	entries map[string]displayNameEntry
}{entries: make(map[string]displayNameEntry)}

// GetDisplayName returns a user's display name, cached for displayNameTTL.
//
// The cache is not an optimisation so much as a correction. This is called from mention detection on
// every single incoming message — twice more when a mention matches — and each call used to be a
// live homeserver round trip on the critical path before the bot had even decided what the message
// was. Bot display names in particular are effectively constant.
// fresh reports whether a cached entry can still be served without asking the homeserver
func (e displayNameEntry) fresh() bool {
	return time.Since(e.fetchedAt) < displayNameTTL
}

// lookupDisplayName returns the cached entry for a user, if there is one
func lookupDisplayName(mxid string) (displayNameEntry, bool) {
	displayNameCache.Lock()
	defer displayNameCache.Unlock()

	entry, ok := displayNameCache.entries[mxid]
	return entry, ok
}

func GetDisplayName(ctx context.Context, mxid string) string {
	entry, ok := lookupDisplayName(mxid)
	if ok && entry.fresh() {
		return entry.name
	}

	dn, err := client.GetDisplayName(ctx, id.UserID(mxid))
	if err != nil {
		log.Error().Ctx(ctx).Err(err).Str("user_id", mxid).Msg("Failed to get display name")
		// A failure is deliberately not cached, so a homeserver blip does not pin the fallback for
		// an hour. Any previously cached name is still better than the raw id.
		if ok {
			return entry.name
		}
	}
	if dn == nil {
		return mxid
	}

	displayNameCache.Lock()
	displayNameCache.entries[mxid] = displayNameEntry{name: dn.DisplayName, fetchedAt: time.Now()}
	displayNameCache.Unlock()

	return dn.DisplayName
}

func GetRoomName(ctx context.Context, roomID string) string {
	var nameContent struct {
		Name string `json:"name"`
	}
	err := client.StateEvent(ctx, id.RoomID(roomID), event.StateRoomName, "", &nameContent)
	if err != nil {
		log.Debug().Ctx(ctx).Err(err).Str("room_id", roomID).Msg("Failed to get room name")
		return ""
	}
	return nameContent.Name
}

func processOutboundEvents(ctx context.Context) {
	for evt := range outboundEvents {
		sendOutboundEvent(ctx, evt)
	}
}

// recordSendOutcome records how a send ended, on the metric and on the span alike.
//
// The failure paths all recorded the metric and left the span untouched, so a reply that was never
// delivered produced a span with an unset status — rendered exactly like a success. That made
// matrix.send the most misleading span in the system, sitting as it does at the end of the path a
// user complains about.
func recordSendOutcome(span trace.Span, status constants.MatrixSendStatus) {
	metrics.RecordMatrixMessageSent(status)
	if status != constants.MatrixSendSuccess {
		span.SetStatus(codes.Error, string(status))
	}
}

// sendOutboundEvent delivers a single queued event to the homeserver.
//
// Split out from the loop so the span can be ended with a defer: the body has several early exits,
// and inside a loop those were labelled continues that no defer would cover.
func sendOutboundEvent(ctx context.Context, evt outboundEvent) {
	// Continue the caller's trace rather than starting a fresh one. Sends are queued and delivered
	// on this goroutine, so without carrying the context across the channel a reply would be traced
	// separately from the work that produced it.
	//
	// Cancellation is deliberately dropped: the turn that queued a message is often finished by the
	// time it goes out, and a cancelled context must not stop a reply being delivered.
	ctx, span := tracer.Start(context.WithoutCancel(evt.ctx), "matrix.send",
		trace.WithSpanKind(trace.SpanKindProducer),
		trace.WithAttributes(
			attribute.String("matrix.room_id", evt.RoomID),
			attribute.String("matrix.event_type", evt.EventType),
		))
	defer span.End()

	// Decided now rather than when the event was queued, since the queue can hold it for a while
	if evt.target.cancelled() {
		log.Debug().Ctx(ctx).Str("room_id", evt.RoomID).Str("event_type", evt.EventType).Msg("Dropping an outbound event that is no longer wanted")
		span.SetAttributes(attribute.Bool("siikabot.matrix.dropped", true))
		if evt.done != nil {
			evt.done <- ""
		}
		return
	}

	startTime := time.Now()
	metrics.SetMatrixOutboundQueueDepth(len(outboundEvents))

	roomId := id.RoomID(evt.RoomID)
	evtType := event.NewEventType(evt.EventType)

	// Placed now too, for the same reason: whether it goes out as a reply depends on what has been
	// posted since the message it answers, which can change while it waits
	message := evt.content()
	var evtContent any = message
	if evt.target != nil {
		asReply := message.RelatesTo.GetNonFallbackReplyTo() != ""
		span.SetAttributes(attribute.Bool("siikabot.matrix.as_reply", asReply))
		log.Debug().Ctx(ctx).
			Str("room_id", evt.RoomID).
			Str("thread_root_id", evt.target.ThreadRootID).
			Str("in_reply_to", evt.target.InReplyTo).
			Bool("as_reply", asReply).
			Msg("Placing an outbound message")
	}

encryptionLoop:
	for {
		isEncrypted, err := stateStore.IsEncrypted(ctx, roomId)

		if err != nil {
			log.Error().Ctx(ctx).Err(err).Str("room_id", evt.RoomID).Msg("Failed to check if room is encrypted")
			if !evt.RetryOnFailure {
				recordSendOutcome(span, constants.MatrixSendFailedEncryption)
				return
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
						recordSendOutcome(span, constants.MatrixSendFailedEncryption)
						return
					}
					time.Sleep(100 * time.Millisecond)
					continue encryptionLoop
				}
				err = olmMachine.ShareGroupSession(ctx, roomId, members)
				if err != nil {
					log.Error().Ctx(ctx).Err(err).Str("room_id", evt.RoomID).Msg("Failed to share group session")
					if !evt.RetryOnFailure {
						recordSendOutcome(span, constants.MatrixSendFailedEncryption)
						return
					}
					time.Sleep(100 * time.Millisecond)
					continue encryptionLoop
				}
				encrypted, err = olmMachine.EncryptMegolmEvent(ctx, roomId, evtType, evtContent)
			}

			if err != nil {
				log.Error().Ctx(ctx).Err(err).Str("room_id", evt.RoomID).Msg("Failed to encrypt message")
				if !evt.RetryOnFailure {
					recordSendOutcome(span, constants.MatrixSendFailedEncryption)
					return
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
			// Recorded before anyone learns the message is out, so that the next message placed
			// sees it even before its copy comes back through sync
			recordLatest(evt.target.timeline(evt.RoomID), resp.EventID.String())
			if evt.done != nil {
				evt.done <- string(resp.EventID)
			}
			recordSendOutcome(span, constants.MatrixSendSuccess)
			metrics.RecordMatrixMessageLatency(time.Since(startTime).Seconds())
			break // Success, break the retry loop
		}
		var httpErr httpError
		httpError, isHttpError := err.(mautrix.HTTPError)
		if !isHttpError {
			log.Error().Ctx(ctx).Err(err).Msg("Failed to parse error response of unexpected type")
			recordSendOutcome(span, constants.MatrixSendFailedSend)
			evt.done <- ""
			break
		}
		if jsonErr := json.Unmarshal([]byte(httpError.ResponseBody), &httpErr); jsonErr != nil {
			log.Error().Ctx(ctx).Err(jsonErr).Msg("Failed to parse error response")
		}

		switch e := httpErr.Errcode; e {
		case "M_LIMIT_EXCEEDED":
			metrics.RecordMatrixRateLimitRetry()
			time.Sleep(time.Duration(httpErr.RetryAfterMs) * time.Millisecond)
		case "M_FORBIDDEN":
			log.Error().
				Ctx(ctx).
				Err(err).
				Str("room_id", evt.RoomID).
				Str("error_code", e).
				Msg("Failed to send message due to permissions")
			recordSendOutcome(span, constants.MatrixSendFailedForbidden)
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
			recordSendOutcome(span, constants.MatrixSendFailedSend)
			evt.done <- ""
			break
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

	// Instrument the homeserver calls. GetDisplayName and FetchMessage sit on the chat path, so
	// without this a turn's trace has unexplained gaps where it was waiting on Matrix.
	//
	// The sync loop is deliberately excluded: it is a single long-poll that runs for the process's
	// lifetime, so it would produce one span per sync cycle carrying no useful information.
	client.Client.Transport = otelhttp.NewTransport(client.Client.Transport,
		otelhttp.WithFilter(func(r *http.Request) bool {
			return !strings.Contains(r.URL.Path, "/sync")
		}))
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

	mautrixLogger := log.Logger.Hook(logging.FieldHook{
		Fields: map[string]string{
			"lib": "mautrix",
		},
	})

	olmMachine = crypto.NewOlmMachine(client, &mautrixLogger, cryptoStore, stateStore)
	err = olmMachine.Load(ctx)
	if err != nil {
		return err
	}

	client.Syncer.(mautrix.ExtensibleSyncer).OnSync(olmMachine.ProcessSyncResponse)

	syncer := client.Syncer.(*mautrix.DefaultSyncer)

	syncer.OnEventType(event.StateMember, func(ctx context.Context, evt *event.Event) {
		olmMachine.HandleMemberEvent(ctx, evt)
		stateStore.SetMembership(ctx, evt)
		invalidateRoom(evt.RoomID.String())
	})
	syncer.OnEventType(event.StateRoomName, func(ctx context.Context, evt *event.Event) {
		invalidateRoom(evt.RoomID.String())
	})
	syncer.OnEventType(event.StateHistoryVisibility, func(ctx context.Context, evt *event.Event) {
		invalidateHistoryVisibility(evt.RoomID.String())
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
	// Listeners for every event run before the ones for a type, in the order they were added, so a
	// message is recorded in its timeline before anything handles it, decrypted or not
	syncer.OnEvent(func(ctx context.Context, evt *event.Event) {
		trackTimeline(evt)
	})
	syncer.OnEvent(func(ctx context.Context, evt *event.Event) {
		handleEvent(ctx, evt, false)
	})

	outboundEvents = make(chan outboundEvent, 256)
	go processOutboundEvents(ctx)
	return nil
}

// InitialSync gets the initial sync from the server for catching up with important missed event such as invites.
//
// Returns an error rather than exiting: a process that calls os.Exit here skips the tracing flush,
// so the spans explaining the failure are discarded along with everything else still batched.
func InitialSync(ctx context.Context) (*mautrix.RespSync, error) {
	resp, err := client.SyncRequest(ctx, 0, "", "", false, "online")
	if err != nil {
		log.Error().Ctx(ctx).Err(err).Msg("Failed to perform initial sync")
		return nil, fmt.Errorf("failed to perform initial sync: %w", err)
	}
	return resp, nil
}

// Sync begins synchronizing events from the server, returning when the context ends or on a severe
// error. Taking a context is what lets a shutdown signal unwind the process in an orderly way
// instead of killing it where it stands.
func Sync(ctx context.Context) error {
	return client.SyncWithContext(ctx)
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
