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
	SenderName        *string    `db:"sender_name"`
	SentAt            *time.Time `db:"sent_at"`
	Mentions          []Mention  `db:"mentions"`
	UnseenBefore      *int       `db:"unseen_before"`
	ReplyToEventID    *string    `db:"reply_to_event_id"`
	ReplyToSender     *string    `db:"reply_to_sender"`
	ReplyToSenderName *string    `db:"reply_to_sender_name"`
	ReplyToSentAt     *time.Time `db:"reply_to_sent_at"`
	ReplyToKind       *string    `db:"reply_to_kind"`
	ReplyToBody       *string    `db:"reply_to_body"`

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
	// ReplyTo is the message this one explicitly refers to, nil if none
	ReplyTo *QuotedMessage
}

// Kinds of message a user turn can quote
const (
	QuoteText    = "text"
	QuoteImage   = "image"
	QuoteDeleted = "deleted"
)

// QuotedMessage is the message a user turn explicitly refers to, as it was when the turn happened
type QuotedMessage struct {
	EventID    string
	Sender     string
	SenderName string
	SentAt     time.Time
	// Kind is QuoteText or QuoteImage, and becomes QuoteDeleted once the original is redacted
	Kind string
	// Body is the quoted text, for QuoteText only
	Body string
}

// UserTurn reads a user row back as the message it records.
//
// The chat_history_user_row_complete and chat_history_reply_complete constraints guarantee that
// every field dereferenced here is set on a user row, so there is nothing to check.
func (m ChatMessage) UserTurn() UserTurn {
	turn := UserTurn{
		Turn:         Turn{RoomID: m.RoomID, EventID: m.TurnEventID, ThreadRootID: stringOrEmpty(m.ThreadRootID)},
		UserID:       m.UserID,
		SenderName:   *m.SenderName,
		SentAt:       *m.SentAt,
		Message:      m.Message,
		Mentions:     m.Mentions,
		UnseenBefore: *m.UnseenBefore,
	}
	if m.ReplyToEventID != nil {
		turn.ReplyTo = &QuotedMessage{
			EventID:    *m.ReplyToEventID,
			Sender:     *m.ReplyToSender,
			SenderName: *m.ReplyToSenderName,
			SentAt:     *m.ReplyToSentAt,
			Kind:       *m.ReplyToKind,
			Body:       stringOrEmpty(m.ReplyToBody),
		}
	}
	return turn
}

// SaveUserTurn stores the message that started a turn
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
	}
	if quote := turn.ReplyTo; quote != nil {
		row.ReplyToEventID = &quote.EventID
		row.ReplyToSender = &quote.Sender
		row.ReplyToSenderName = &quote.SenderName
		row.ReplyToSentAt = &quote.SentAt
		row.ReplyToKind = &quote.Kind
		if quote.Kind == QuoteText {
			row.ReplyToBody = &quote.Body
		}
	}

	return insertChatMessage(ctx, pool, row)
}

// SaveAnswer stores the bot's answer that ended a turn, once it has been delivered as answerEventID
func SaveAnswer(ctx context.Context, turn Turn, userID, message, answerEventID string) error {
	return insertChatMessage(ctx, pool, ChatMessage{
		RoomID:        turn.RoomID,
		UserID:        userID,
		Message:       message,
		Role:          "assistant",
		MessageType:   "text",
		TurnEventID:   turn.EventID,
		ThreadRootID:  nullIfEmpty(turn.ThreadRootID),
		AnswerEventID: &answerEventID,
	})
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
		if err := insertChatMessage(ctx, tx, row); err != nil {
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
}

// insertChatMessage writes a single chat history row using the given executor
func insertChatMessage(ctx context.Context, executor chatMessageExecutor, row ChatMessage) error {
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
	if row.ReplyToBody != nil && !utf8.ValidString(*row.ReplyToBody) {
		body := strings.ToValidUTF8(*row.ReplyToBody, "")
		row.ReplyToBody = &body
	}

	// A nil list is SQL NULL, which is what rows other than user rows store
	var mentions any
	if row.Mentions != nil {
		mentions = row.Mentions
	}

	_, err := executor.Exec(ctx,
		`INSERT INTO chat_history (room_id, user_id, message, role, message_type, tool_call_id, tool_name,
			expiry, turn_event_id, thread_root_id, sender_name, sent_at, mentions, unseen_before,
			reply_to_event_id, reply_to_sender, reply_to_sender_name, reply_to_sent_at, reply_to_kind,
			reply_to_body, answer_event_id)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17, $18, $19, $20, $21)`,
		row.RoomID, row.UserID, row.Message, row.Role, row.MessageType, row.ToolCallID, row.ToolName,
		row.Expiry, row.TurnEventID, row.ThreadRootID, row.SenderName, row.SentAt, mentions, row.UnseenBefore,
		row.ReplyToEventID, row.ReplyToSender, row.ReplyToSenderName, row.ReplyToSentAt, row.ReplyToKind,
		row.ReplyToBody, row.AnswerEventID)
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
		return err
	}
	return nil
}

// GetChatHistory retrieves recent chat history for a room, oldest first.
// maxMessages is a safety cap on how many rows to read, not the context window size — the window is
// decided by the caller from the room's anchor and token budget.
//
// Expired rows are returned rather than filtered out. Deleting them would silently reshape the
// conversation mid-thread, so the caller replaces expired tool results with a marker instead,
// keeping the structure intact. Check ChatMessage.Expiry to tell them apart.
//
// Ordering is by timestamp with the id as a tiebreaker. The tiebreaker is required, not cosmetic:
// rows written inside one transaction all take the transaction start time from NOW(), so ordering
// by timestamp alone would return a tool call and its response in an arbitrary order.
func GetChatHistory(ctx context.Context, roomID string, maxMessages int) ([]ChatMessage, error) {
	rows, err := pool.Query(ctx,
		`SELECT id, room_id, user_id, message, role, timestamp, message_type, tool_call_id, tool_name, expiry,
			turn_event_id, thread_root_id, sender_name, sent_at, mentions, unseen_before,
			reply_to_event_id, reply_to_sender, reply_to_sender_name, reply_to_sent_at, reply_to_kind,
			reply_to_body, answer_event_id
		FROM chat_history
		WHERE room_id = $1
		ORDER BY timestamp DESC, id DESC LIMIT $2`,
		roomID, maxMessages)
	if err != nil {
		log.Error().Ctx(ctx).Err(err).
			Str("room_id", roomID).
			Int("max_messages", maxMessages).
			Msg("Failed to get chat history")
		return nil, err
	}

	messages, err := pgx.CollectRows(rows, pgx.RowToStructByName[ChatMessage])
	if err != nil {
		log.Error().Ctx(ctx).Err(err).
			Str("room_id", roomID).
			Msg("Failed to collect chat history rows")
		return nil, err
	}

	// Reverse the order to get chronological order (oldest first)
	for i, j := 0, len(messages)-1; i < j; i, j = i+1, j-1 {
		messages[i], messages[j] = messages[j], messages[i]
	}

	return messages, nil
}

// ForgetChatEvent removes a redacted event from the chat history. If the event triggered a turn,
// or was the bot's answer to one, the whole turn goes. If a turn quoted it, the quote is blanked
// and the turn stays. Returns how many rows were deleted and how many quotes were blanked.
func ForgetChatEvent(ctx context.Context, roomID, eventID string) (int64, int64, error) {
	tx, err := pool.Begin(ctx)
	if err != nil {
		log.Error().Ctx(ctx).Err(err).Str("room_id", roomID).Str("event_id", eventID).Msg("Failed to begin transaction for forgetting an event")
		return 0, 0, err
	}
	defer tx.Rollback(ctx)

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
		`UPDATE chat_history SET reply_to_kind = 'deleted', reply_to_body = NULL
		WHERE room_id = $1 AND reply_to_event_id = $2 AND reply_to_kind <> 'deleted'`,
		roomID, eventID)
	if err != nil {
		log.Error().Ctx(ctx).Err(err).Str("room_id", roomID).Str("event_id", eventID).Msg("Failed to blank quotes of a redacted event")
		return 0, 0, err
	}

	if err := tx.Commit(ctx); err != nil {
		log.Error().Ctx(ctx).Err(err).Str("room_id", roomID).Str("event_id", eventID).Msg("Failed to commit forgetting an event")
		return 0, 0, err
	}

	return deleted.RowsAffected(), blanked.RowsAffected(), nil
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

// DeleteChatHistoryForRoom deletes all chat history for a specific room
func DeleteChatHistoryForRoom(ctx context.Context, roomID string) (int64, error) {
	tag, err := pool.Exec(ctx,
		"DELETE FROM chat_history WHERE room_id = $1",
		roomID)
	if err != nil {
		log.Error().Ctx(ctx).Err(err).
			Str("room_id", roomID).
			Msg("Failed to delete chat history for room")
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

// GetChatContextAnchor retrieves the id of the chat history row a room's context window starts at.
// Returns nil if no anchor is set, meaning the window starts at the oldest available history.
func GetChatContextAnchor(ctx context.Context, roomID string) (*int64, error) {
	var anchorID int64
	err := pool.QueryRow(ctx,
		"SELECT anchor_id FROM chat_context_anchors WHERE room_id = $1",
		roomID).Scan(&anchorID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		log.Error().Ctx(ctx).Err(err).
			Str("room_id", roomID).
			Msg("Failed to get chat context anchor")
		return nil, err
	}
	return &anchorID, nil
}

// SetChatContextAnchor sets the id of the chat history row a room's context window starts at
func SetChatContextAnchor(ctx context.Context, roomID string, anchorID int64) error {
	_, err := pool.Exec(ctx,
		"INSERT INTO chat_context_anchors (room_id, anchor_id) VALUES ($1, $2) "+
			"ON CONFLICT (room_id) DO UPDATE SET anchor_id = $2",
		roomID, anchorID)
	if err != nil {
		log.Error().Ctx(ctx).Err(err).
			Str("room_id", roomID).
			Int64("anchor_id", anchorID).
			Msg("Failed to set chat context anchor")
		return err
	}
	return nil
}

// ClearChatContextAnchor removes a room's context window anchor, so the window starts again at the
// oldest available history
func ClearChatContextAnchor(ctx context.Context, roomID string) error {
	_, err := pool.Exec(ctx,
		"DELETE FROM chat_context_anchors WHERE room_id = $1",
		roomID)
	if err != nil {
		log.Error().Ctx(ctx).Err(err).
			Str("room_id", roomID).
			Msg("Failed to clear chat context anchor")
		return err
	}
	return nil
}

// nullIfEmpty stores an empty string as NULL, for the columns where NULL is the meaningful value
// (thread_root_id is NULL for the main timeline)
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
