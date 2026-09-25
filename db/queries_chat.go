package db

import (
	"context"
	"errors"
	"strings"
	"time"
	"unicode/utf8"

	pgx "github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/rs/zerolog/log"
)

// ChatMessage represents a message in the chat history
type ChatMessage struct {
	ID          int64      `db:"id"`
	RoomID      string     `db:"room_id"`
	UserID      string     `db:"user_id"`
	Message     string     `db:"message"`
	Role        string     `db:"role"`
	Timestamp   time.Time  `db:"timestamp"`
	MessageType string     `db:"message_type"`
	ToolCallID  *string    `db:"tool_call_id"`
	ToolName    *string    `db:"tool_name"`
	Expiry      *time.Time `db:"expiry"`

	// The turn the row belongs to, on every row
	TurnEventID  string  `db:"turn_event_id"`
	ThreadRootID *string `db:"thread_root_id"`

	// User rows only; read them through UserTurn
	SenderName   *string    `db:"sender_name"`
	SentAt       *time.Time `db:"sent_at"`
	Mentions     []Mention  `db:"mentions"`
	UnseenBefore *int       `db:"unseen_before"`
	HasImage     *bool      `db:"has_image"`
	// References are the messages a user row refers to, read from chat_references along with it
	References []Reference `db:"-"`

	// Answer rows only
	AnswerEventID *string `db:"answer_event_id"`
}

// Mention is a user a message mentioned, with the name they had at the time
type Mention struct {
	UserID string `json:"user_id"`
	Name   string `json:"name"`
}

// Turn identifies the chat turn a history row belongs to
type Turn struct {
	RoomID string
	// EventID is the message that triggered the turn
	EventID string
	// ThreadRootID is the thread the turn happened in, empty for the main timeline
	ThreadRootID string
}

// Timeline is where chat turns happen, each with a context window of its own: a room's main
// timeline, or a thread in it
type Timeline struct {
	RoomID string
	// ThreadRootID is the root of the thread, empty for the main timeline
	ThreadRootID string
}

// Timeline is the timeline the turn happened in
func (t Turn) Timeline() Timeline {
	return Timeline{RoomID: t.RoomID, ThreadRootID: t.ThreadRootID}
}

// UserTurn is a message that addressed the bot, with everything recorded about it
type UserTurn struct {
	Turn
	UserID     string
	SenderName string
	SentAt     time.Time
	Message    string
	Mentions   []Mention
	// UnseenBefore is how many room messages the bot did not see between the previous message
	// addressed to it and this one
	UnseenBefore int
	// HasImage says the message was an image, with Message as its caption
	HasImage bool
	// ReplyTo is the message this one replies to, or the root of the thread it starts; nil if none
	ReplyTo *QuotedMessage
	// Links are the messages in the room this one links to, in the order it links to them
	Links []QuotedMessage
}

// Kinds of message a user turn can quote
const (
	QuoteText    = "text"
	QuoteImage   = "image"
	QuoteDeleted = "deleted"
)

// How a user turn refers to a message
const (
	// RelationReply is the message the turn replies to, or the root of the thread it starts
	RelationReply = "reply"
	// RelationLink is a message in the room that the turn links to
	RelationLink = "link"
)

// Reference is a message a user turn refers to, and how it refers to it
type Reference struct {
	Relation string
	QuotedMessage
}

// QuotedMessage is a message a user turn explicitly refers to, as it was when the turn happened
type QuotedMessage struct {
	EventID    string
	Sender     string
	SenderName string
	SentAt     time.Time
	// Kind is QuoteText or QuoteImage, and becomes QuoteDeleted once the original is redacted
	Kind string
	// Body is the quoted text, or the caption of an image, if it has one
	Body string
}

// UserTurn reads a user row back as the message it records.
//
// The chat_history_user_row_complete constraint guarantees that every field dereferenced here is
// set on a user row, and the chat_references constraints that each reference is complete, so there
// is nothing to check.
func (m ChatMessage) UserTurn() UserTurn {
	turn := UserTurn{
		Turn:         Turn{RoomID: m.RoomID, EventID: m.TurnEventID, ThreadRootID: stringOrEmpty(m.ThreadRootID)},
		UserID:       m.UserID,
		SenderName:   *m.SenderName,
		SentAt:       *m.SentAt,
		Message:      m.Message,
		Mentions:     m.Mentions,
		UnseenBefore: *m.UnseenBefore,
		HasImage:     *m.HasImage,
	}
	for _, reference := range m.References {
		switch reference.Relation {
		case RelationReply:
			quote := reference.QuotedMessage
			turn.ReplyTo = &quote
		case RelationLink:
			turn.Links = append(turn.Links, reference.QuotedMessage)
		}
	}
	return turn
}

// references lists what a user turn refers to in the order it is stored and shown: the message it
// replies to first, then the ones it links to
func (t UserTurn) references() []Reference {
	var references []Reference
	if t.ReplyTo != nil {
		references = append(references, Reference{Relation: RelationReply, QuotedMessage: *t.ReplyTo})
	}
	for _, link := range t.Links {
		references = append(references, Reference{Relation: RelationLink, QuotedMessage: link})
	}
	return references
}

// SaveUserTurn stores the message that started a turn, with the messages it refers to
func SaveUserTurn(ctx context.Context, turn UserTurn) error {
	// Never nil: an empty list is stored as one, and the column is required on user rows
	mentions := turn.Mentions
	if mentions == nil {
		mentions = []Mention{}
	}

	row := ChatMessage{
		RoomID:       turn.RoomID,
		UserID:       turn.UserID,
		Message:      turn.Message,
		Role:         "user",
		MessageType:  "text",
		TurnEventID:  turn.EventID,
		ThreadRootID: nullIfEmpty(turn.ThreadRootID),
		SenderName:   &turn.SenderName,
		SentAt:       &turn.SentAt,
		Mentions:     mentions,
		UnseenBefore: &turn.UnseenBefore,
		HasImage:     &turn.HasImage,
	}

	tx, err := pool.Begin(ctx)
	if err != nil {
		log.Error().Ctx(ctx).Err(err).Str("room_id", turn.RoomID).Str("turn_event_id", turn.EventID).
			Msg("Failed to begin transaction for a user turn")
		return err
	}
	defer tx.Rollback(ctx)

	historyID, err := insertChatMessage(ctx, tx, row)
	if err != nil {
		return err
	}
	for position, reference := range turn.references() {
		if err := insertReference(ctx, tx, historyID, position, reference); err != nil {
			return err
		}
	}

	if err := tx.Commit(ctx); err != nil {
		log.Error().Ctx(ctx).Err(err).Str("room_id", turn.RoomID).Str("turn_event_id", turn.EventID).
			Msg("Failed to commit a user turn")
		return err
	}
	return nil
}

// insertReference stores one of the messages a user row refers to
func insertReference(ctx context.Context, executor chatMessageExecutor, historyID int64, position int, reference Reference) error {
	// Text always has a body, an image only when it has a caption
	var body *string
	if reference.Kind == QuoteText || (reference.Kind == QuoteImage && reference.Body != "") {
		// Ensure the text is valid UTF-8, replacing invalid sequences
		cleaned := strings.ToValidUTF8(reference.Body, "")
		body = &cleaned
	}

	_, err := executor.Exec(ctx,
		`INSERT INTO chat_references (history_id, position, relation, event_id, sender, sender_name, sent_at, kind, body)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)`,
		historyID, position, reference.Relation, reference.EventID, reference.Sender, reference.SenderName,
		reference.SentAt, reference.Kind, body)
	if err != nil {
		log.Error().Ctx(ctx).Err(err).
			Int64("history_id", historyID).
			Str("relation", reference.Relation).
			Str("event_id", reference.EventID).
			Msg("Failed to save a message a turn refers to")
	}
	return err
}

// SaveAnswer stores the bot's answer that ended a turn, once it has been delivered as answerEventID
func SaveAnswer(ctx context.Context, turn Turn, userID, message, answerEventID string) error {
	_, err := insertChatMessage(ctx, pool, ChatMessage{
		RoomID:        turn.RoomID,
		UserID:        userID,
		Message:       message,
		Role:          "assistant",
		MessageType:   "text",
		TurnEventID:   turn.EventID,
		ThreadRootID:  nullIfEmpty(turn.ThreadRootID),
		AnswerEventID: &answerEventID,
	})
	return err
}

// ToolCallRecord holds a tool call together with the response it produced, so the pair can be
// persisted atomically
type ToolCallRecord struct {
	ToolCallID       string
	ToolName         string
	Arguments        string
	Response         string
	ValidityDuration time.Duration
}

// ToolCallRows returns the history rows for a batch of tool calls, in the order they are stored:
// all of the batch's calls, then all of their responses.
//
// That is the order the replay reads a batch back in. Storing each call followed by its response
// instead makes a batch of parallel calls indistinguishable from calls made one at a time.
func ToolCallRows(turn Turn, userID string, records []ToolCallRecord) []ChatMessage {
	now := time.Now()
	calls := make([]ChatMessage, 0, len(records))
	responses := make([]ChatMessage, 0, len(records))
	for _, record := range records {
		// The call and its response share an expiry so they always drop out of history together
		var expiry *time.Time
		if record.ValidityDuration > 0 {
			expiryTime := now.Add(record.ValidityDuration)
			expiry = &expiryTime
		}

		base := ChatMessage{
			RoomID:       turn.RoomID,
			UserID:       userID,
			ToolCallID:   &record.ToolCallID,
			ToolName:     &record.ToolName,
			Expiry:       expiry,
			TurnEventID:  turn.EventID,
			ThreadRootID: nullIfEmpty(turn.ThreadRootID),
		}

		call := base
		call.Message, call.Role, call.MessageType = record.Arguments, "assistant", "tool_call"
		calls = append(calls, call)

		response := base
		response.Message, response.Role, response.MessageType = record.Response, "tool", "tool_response"
		responses = append(responses, response)
	}
	return append(calls, responses...)
}

// SaveToolCallsWithResponses saves tool calls and their responses in a single transaction.
//
// The pairing must be atomic: a tool call persisted without its response leaves history that
// rebuilds into an assistant message carrying tool_calls with no tool reply, which the chat API
// rejects outright, breaking every later request in the room.
func SaveToolCallsWithResponses(ctx context.Context, turn Turn, userID string, records []ToolCallRecord) error {
	if len(records) == 0 {
		return nil
	}

	tx, err := pool.Begin(ctx)
	if err != nil {
		log.Error().Ctx(ctx).Err(err).
			Str("room_id", turn.RoomID).
			Int("record_count", len(records)).
			Msg("Failed to begin transaction for tool call history")
		return err
	}
	defer tx.Rollback(ctx)

	for _, row := range ToolCallRows(turn, userID, records) {
		if _, err := insertChatMessage(ctx, tx, row); err != nil {
			return err
		}
	}

	if err := tx.Commit(ctx); err != nil {
		log.Error().Ctx(ctx).Err(err).
			Str("room_id", turn.RoomID).
			Int("record_count", len(records)).
			Msg("Failed to commit tool call history")
		return err
	}

	return nil
}

// chatMessageExecutor is satisfied by both the connection pool and a transaction, so chat history
// rows can be written either standalone or as part of an atomic group
type chatMessageExecutor interface {
	Exec(ctx context.Context, sql string, arguments ...any) (pgconn.CommandTag, error)
	QueryRow(ctx context.Context, sql string, arguments ...any) pgx.Row
}

// insertChatMessage writes a single chat history row using the given executor, returning its id
func insertChatMessage(ctx context.Context, executor chatMessageExecutor, row ChatMessage) (int64, error) {
	// Ensure the text is valid UTF-8, replacing invalid sequences
	if !utf8.ValidString(row.Message) {
		log.Warn().Ctx(ctx).
			Str("room_id", row.RoomID).
			Str("user_id", row.UserID).
			Str("role", row.Role).
			Str("message_type", row.MessageType).
			Msg("Message contains invalid UTF-8, cleaning up")
		row.Message = strings.ToValidUTF8(row.Message, "")
	}

	// A nil list is SQL NULL, which is what rows other than user rows store
	var mentions any
	if row.Mentions != nil {
		mentions = row.Mentions
	}

	var id int64
	err := executor.QueryRow(ctx,
		`INSERT INTO chat_history (room_id, user_id, message, role, message_type, tool_call_id, tool_name,
			expiry, turn_event_id, thread_root_id, sender_name, sent_at, mentions, unseen_before, has_image,
			answer_event_id)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16)
		RETURNING id`,
		row.RoomID, row.UserID, row.Message, row.Role, row.MessageType, row.ToolCallID, row.ToolName,
		row.Expiry, row.TurnEventID, row.ThreadRootID, row.SenderName, row.SentAt, mentions, row.UnseenBefore,
		row.HasImage, row.AnswerEventID).Scan(&id)
	if err != nil {
		log.Error().Ctx(ctx).Err(err).
			Str("room_id", row.RoomID).
			Str("user_id", row.UserID).
			Str("role", row.Role).
			Str("message_type", row.MessageType).
			Str("turn_event_id", row.TurnEventID).
			Str("tool_call_id", stringOrEmpty(row.ToolCallID)).
			Str("tool_name", stringOrEmpty(row.ToolName)).
			Msg("Failed to save chat message")
		return 0, err
	}
	return id, nil
}

// GetChatHistory retrieves the recent chat history of a timeline, oldest first: its own turns, and
// for a thread started from a stored turn, that turn as well (seedTurnEventID, empty for none).
// maxMessages is a safety cap on how many rows to read, not the context window size — the window is
// decided by the caller from the timeline's anchor and token budget.
//
// Expired rows are returned rather than filtered out. Deleting them would silently reshape the
// conversation mid-thread, so the caller replaces expired tool results with a marker instead,
// keeping the structure intact. Check ChatMessage.Expiry to tell them apart.
//
// Ordering is by timestamp with the id as a tiebreaker. The tiebreaker is required, not cosmetic:
// rows written inside one transaction all take the transaction start time from NOW(), so ordering
// by timestamp alone would return a tool call and its response in an arbitrary order.
func GetChatHistory(ctx context.Context, timeline Timeline, seedTurnEventID string, maxMessages int) ([]ChatMessage, error) {
	// The seed belongs to the main timeline, where the thread was started from
	rows, err := pool.Query(ctx,
		`SELECT id, room_id, user_id, message, role, timestamp, message_type, tool_call_id, tool_name, expiry,
			turn_event_id, thread_root_id, sender_name, sent_at, mentions, unseen_before, has_image,
			answer_event_id
		FROM chat_history
		WHERE room_id = $1 AND (thread_root_id IS NOT DISTINCT FROM $2
			OR (thread_root_id IS NULL AND turn_event_id = $3))
		ORDER BY timestamp DESC, id DESC LIMIT $4`,
		timeline.RoomID, nullIfEmpty(timeline.ThreadRootID), seedTurnEventID, maxMessages)
	if err != nil {
		log.Error().Ctx(ctx).Err(err).
			Str("room_id", timeline.RoomID).
			Str("thread_root_id", timeline.ThreadRootID).
			Int("max_messages", maxMessages).
			Msg("Failed to get chat history")
		return nil, err
	}

	messages, err := pgx.CollectRows(rows, pgx.RowToStructByName[ChatMessage])
	if err != nil {
		log.Error().Ctx(ctx).Err(err).
			Str("room_id", timeline.RoomID).
			Str("thread_root_id", timeline.ThreadRootID).
			Msg("Failed to collect chat history rows")
		return nil, err
	}

	// Reverse the order to get chronological order (oldest first)
	for i, j := 0, len(messages)-1; i < j; i, j = i+1, j-1 {
		messages[i], messages[j] = messages[j], messages[i]
	}

	if err := loadReferences(ctx, timeline.RoomID, messages); err != nil {
		return nil, err
	}
	return messages, nil
}

// referenceRow is a row of chat_references
type referenceRow struct {
	HistoryID  int64     `db:"history_id"`
	Relation   string    `db:"relation"`
	EventID    string    `db:"event_id"`
	Sender     string    `db:"sender"`
	SenderName string    `db:"sender_name"`
	SentAt     time.Time `db:"sent_at"`
	Kind       string    `db:"kind"`
	Body       *string   `db:"body"`
}

// loadReferences reads the messages the user rows among messages refer to, in their order
func loadReferences(ctx context.Context, roomID string, messages []ChatMessage) error {
	byID := make(map[int64]int)
	var ids []int64
	for i, msg := range messages {
		if msg.Role == "user" {
			byID[msg.ID] = i
			ids = append(ids, msg.ID)
		}
	}
	if len(ids) == 0 {
		return nil
	}

	rows, err := pool.Query(ctx,
		`SELECT history_id, relation, event_id, sender, sender_name, sent_at, kind, body
		FROM chat_references WHERE history_id = ANY($1) ORDER BY history_id, position`,
		ids)
	if err != nil {
		log.Error().Ctx(ctx).Err(err).Str("room_id", roomID).Msg("Failed to get the messages chat turns refer to")
		return err
	}
	references, err := pgx.CollectRows(rows, pgx.RowToStructByName[referenceRow])
	if err != nil {
		log.Error().Ctx(ctx).Err(err).Str("room_id", roomID).Msg("Failed to collect the messages chat turns refer to")
		return err
	}

	for _, ref := range references {
		msg := &messages[byID[ref.HistoryID]]
		msg.References = append(msg.References, Reference{
			Relation: ref.Relation,
			QuotedMessage: QuotedMessage{
				EventID:    ref.EventID,
				Sender:     ref.Sender,
				SenderName: ref.SenderName,
				SentAt:     ref.SentAt,
				Kind:       ref.Kind,
				Body:       stringOrEmpty(ref.Body),
			},
		})
	}
	return nil
}

// ThreadSeed is the stored turn a thread was started from, whose rows open the thread's context
type ThreadSeed struct {
	TurnEventID string
	// SentAt is when the turn's question was sent, the oldest message the seed shows
	SentAt time.Time
}

// GetThreadSeed finds the stored turn in the main timeline that a thread was started from: the turn
// whose question, or whose answer, is the thread's root. Returns false if there is none.
func GetThreadSeed(ctx context.Context, roomID, rootEventID string) (ThreadSeed, bool, error) {
	var seed ThreadSeed
	err := pool.QueryRow(ctx,
		`SELECT turn_event_id, sent_at FROM chat_history
		WHERE room_id = $1 AND thread_root_id IS NULL AND role = 'user' AND turn_event_id IN (
			SELECT turn_event_id FROM chat_history
			WHERE room_id = $1 AND thread_root_id IS NULL AND (turn_event_id = $2 OR answer_event_id = $2))
		LIMIT 1`,
		roomID, rootEventID).Scan(&seed.TurnEventID, &seed.SentAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return ThreadSeed{}, false, nil
	}
	if err != nil {
		log.Error().Ctx(ctx).Err(err).Str("room_id", roomID).Str("root_event_id", rootEventID).Msg("Failed to find the turn a thread was started from")
		return ThreadSeed{}, false, err
	}
	return seed, true, nil
}

// HasChatTurn reports whether a message started a chat turn that is still in the history
func HasChatTurn(ctx context.Context, roomID, eventID string) (bool, error) {
	var exists bool
	err := pool.QueryRow(ctx,
		"SELECT EXISTS (SELECT 1 FROM chat_history WHERE room_id = $1 AND turn_event_id = $2)",
		roomID, eventID).Scan(&exists)
	if err != nil {
		log.Error().Ctx(ctx).Err(err).Str("room_id", roomID).Str("event_id", eventID).Msg("Failed to check for a chat turn")
		return false, err
	}
	return exists, nil
}

// ForgetChatEvent removes a redacted event from the chat history. If the event triggered a turn,
// or was the bot's answer to one, the whole turn goes. If a turn referred to it, the reference is
// blanked and the turn stays. Returns how many rows were deleted and how many references were
// blanked.
func ForgetChatEvent(ctx context.Context, roomID, eventID string) (int64, int64, error) {
	tx, err := pool.Begin(ctx)
	if err != nil {
		log.Error().Ctx(ctx).Err(err).Str("room_id", roomID).Str("event_id", eventID).Msg("Failed to begin transaction for forgetting an event")
		return 0, 0, err
	}
	defer tx.Rollback(ctx)

	// The turn's references go with it
	deleted, err := tx.Exec(ctx,
		`DELETE FROM chat_history
		WHERE room_id = $1 AND turn_event_id IN (
			SELECT turn_event_id FROM chat_history
			WHERE room_id = $1 AND (turn_event_id = $2 OR answer_event_id = $2))`,
		roomID, eventID)
	if err != nil {
		log.Error().Ctx(ctx).Err(err).Str("room_id", roomID).Str("event_id", eventID).Msg("Failed to delete the turn of a redacted event")
		return 0, 0, err
	}

	blanked, err := tx.Exec(ctx,
		`UPDATE chat_references SET kind = 'deleted', body = NULL
		WHERE event_id = $2 AND kind <> 'deleted'
			AND history_id IN (SELECT id FROM chat_history WHERE room_id = $1)`,
		roomID, eventID)
	if err != nil {
		log.Error().Ctx(ctx).Err(err).Str("room_id", roomID).Str("event_id", eventID).Msg("Failed to blank references to a redacted event")
		return 0, 0, err
	}

	if err := tx.Commit(ctx); err != nil {
		log.Error().Ctx(ctx).Err(err).Str("room_id", roomID).Str("event_id", eventID).Msg("Failed to commit forgetting an event")
		return 0, 0, err
	}

	return deleted.RowsAffected(), blanked.RowsAffected(), nil
}

// FollowChatEdit brings the chat history in line with an edit of a message: the text of a turn the
// message started becomes message, and the text, or caption, of any reference to it becomes quoted.
// Only edits by the message's own sender count, so nobody can rewrite what someone else said.
// Returns how many turns and references were changed.
func FollowChatEdit(ctx context.Context, roomID, eventID, sender, message, quoted string) (int64, int64, error) {
	tx, err := pool.Begin(ctx)
	if err != nil {
		log.Error().Ctx(ctx).Err(err).Str("room_id", roomID).Str("event_id", eventID).Msg("Failed to begin transaction for following an edit")
		return 0, 0, err
	}
	defer tx.Rollback(ctx)

	turns, err := tx.Exec(ctx,
		`UPDATE chat_history SET message = $4
		WHERE room_id = $1 AND turn_event_id = $2 AND user_id = $3 AND role = 'user' AND message_type = 'text'`,
		roomID, eventID, sender, strings.ToValidUTF8(message, ""))
	if err != nil {
		log.Error().Ctx(ctx).Err(err).Str("room_id", roomID).Str("event_id", eventID).Msg("Failed to follow an edit in a chat turn")
		return 0, 0, err
	}

	// An image's caption can be edited away, which text can't
	references, err := tx.Exec(ctx,
		`UPDATE chat_references SET body = CASE kind WHEN 'image' THEN NULLIF($4, '') ELSE $4 END
		WHERE event_id = $2 AND sender = $3 AND kind IN ('text', 'image')
			AND history_id IN (SELECT id FROM chat_history WHERE room_id = $1)`,
		roomID, eventID, sender, strings.ToValidUTF8(quoted, ""))
	if err != nil {
		log.Error().Ctx(ctx).Err(err).Str("room_id", roomID).Str("event_id", eventID).Msg("Failed to follow an edit in references")
		return 0, 0, err
	}

	if err := tx.Commit(ctx); err != nil {
		log.Error().Ctx(ctx).Err(err).Str("room_id", roomID).Str("event_id", eventID).Msg("Failed to commit following an edit")
		return 0, 0, err
	}
	return turns.RowsAffected(), references.RowsAffected(), nil
}

// CleanupOldChatHistory removes chat messages older than the specified duration
func CleanupOldChatHistory(ctx context.Context, olderThan time.Duration) (int64, error) {
	cutoffTime := time.Now().Add(-olderThan)

	tag, err := pool.Exec(ctx,
		"DELETE FROM chat_history WHERE timestamp < $1",
		cutoffTime)
	if err != nil {
		log.Error().Ctx(ctx).Err(err).
			Time("cutoff_time", cutoffTime).
			Msg("Failed to cleanup old chat history")
		return 0, err
	}

	return tag.RowsAffected(), nil
}

// ResetChatTimeline empties the context of a timeline, returning how many rows were deleted.
//
// Resetting the main timeline deletes its turns, including any that threads were started from:
// those threads keep their own turns. Resetting a thread deletes the thread's turns, and moves its
// anchor past everything stored so far, so the turn it was started from, which belongs to the main
// timeline, drops out of it as well.
func ResetChatTimeline(ctx context.Context, timeline Timeline) (int64, error) {
	tx, err := pool.Begin(ctx)
	if err != nil {
		log.Error().Ctx(ctx).Err(err).Str("room_id", timeline.RoomID).Str("thread_root_id", timeline.ThreadRootID).
			Msg("Failed to begin transaction for resetting a chat timeline")
		return 0, err
	}
	defer tx.Rollback(ctx)

	tag, err := tx.Exec(ctx,
		"DELETE FROM chat_history WHERE room_id = $1 AND thread_root_id IS NOT DISTINCT FROM $2",
		timeline.RoomID, nullIfEmpty(timeline.ThreadRootID))
	if err != nil {
		log.Error().Ctx(ctx).Err(err).Str("room_id", timeline.RoomID).Str("thread_root_id", timeline.ThreadRootID).
			Msg("Failed to delete the chat history of a timeline")
		return 0, err
	}

	if timeline.ThreadRootID == "" {
		_, err = tx.Exec(ctx,
			"DELETE FROM chat_context_anchors WHERE room_id = $1 AND thread_root_id = ''",
			timeline.RoomID)
	} else {
		_, err = tx.Exec(ctx,
			`INSERT INTO chat_context_anchors (room_id, thread_root_id, anchor_id)
			VALUES ($1, $2, (SELECT COALESCE(MAX(id), 0) + 1 FROM chat_history))
			ON CONFLICT (room_id, thread_root_id) DO UPDATE SET anchor_id = EXCLUDED.anchor_id`,
			timeline.RoomID, timeline.ThreadRootID)
	}
	if err != nil {
		log.Error().Ctx(ctx).Err(err).Str("room_id", timeline.RoomID).Str("thread_root_id", timeline.ThreadRootID).
			Msg("Failed to reset the anchor of a timeline")
		return 0, err
	}

	if err := tx.Commit(ctx); err != nil {
		log.Error().Ctx(ctx).Err(err).Str("room_id", timeline.RoomID).Str("thread_root_id", timeline.ThreadRootID).
			Msg("Failed to commit resetting a chat timeline")
		return 0, err
	}
	return tag.RowsAffected(), nil
}

// CountUnseenMessage records one more room message that was not addressed to the bot
func CountUnseenMessage(ctx context.Context, roomID string) error {
	_, err := pool.Exec(ctx,
		`INSERT INTO chat_unseen_messages (room_id, count) VALUES ($1, 1)
		ON CONFLICT (room_id) DO UPDATE SET count = chat_unseen_messages.count + 1`,
		roomID)
	if err != nil {
		log.Error().Ctx(ctx).Err(err).Str("room_id", roomID).Msg("Failed to count an unseen message")
		return err
	}
	return nil
}

// TakeUnseenMessages returns how many room messages were not addressed to the bot since the last
// one that was, and starts the count over. A room with no count has had none.
func TakeUnseenMessages(ctx context.Context, roomID string) (int, error) {
	var count int
	err := pool.QueryRow(ctx,
		"DELETE FROM chat_unseen_messages WHERE room_id = $1 RETURNING count",
		roomID).Scan(&count)
	if errors.Is(err, pgx.ErrNoRows) {
		return 0, nil
	}
	if err != nil {
		log.Error().Ctx(ctx).Err(err).Str("room_id", roomID).Msg("Failed to take the unseen message count")
		return 0, err
	}
	return count, nil
}

// GetChatContextAnchor retrieves the id of the chat history row a timeline's context window starts
// at. Returns nil if no anchor is set, meaning the window starts at the oldest available history.
func GetChatContextAnchor(ctx context.Context, timeline Timeline) (*int64, error) {
	var anchorID int64
	err := pool.QueryRow(ctx,
		"SELECT anchor_id FROM chat_context_anchors WHERE room_id = $1 AND thread_root_id = $2",
		timeline.RoomID, timeline.ThreadRootID).Scan(&anchorID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		log.Error().Ctx(ctx).Err(err).
			Str("room_id", timeline.RoomID).
			Str("thread_root_id", timeline.ThreadRootID).
			Msg("Failed to get chat context anchor")
		return nil, err
	}
	return &anchorID, nil
}

// SetChatContextAnchor sets the id of the chat history row a timeline's context window starts at
func SetChatContextAnchor(ctx context.Context, timeline Timeline, anchorID int64) error {
	_, err := pool.Exec(ctx,
		`INSERT INTO chat_context_anchors (room_id, thread_root_id, anchor_id) VALUES ($1, $2, $3)
		ON CONFLICT (room_id, thread_root_id) DO UPDATE SET anchor_id = EXCLUDED.anchor_id`,
		timeline.RoomID, timeline.ThreadRootID, anchorID)
	if err != nil {
		log.Error().Ctx(ctx).Err(err).
			Str("room_id", timeline.RoomID).
			Str("thread_root_id", timeline.ThreadRootID).
			Int64("anchor_id", anchorID).
			Msg("Failed to set chat context anchor")
		return err
	}
	return nil
}

// nullIfEmpty stores an empty string as NULL, for the columns where NULL is the meaningful value
// (thread_root_id in chat_history is NULL for the main timeline)
func nullIfEmpty(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

// stringOrEmpty reads such a column back
func stringOrEmpty(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}
