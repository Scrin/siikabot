BEGIN;

-- Chat history rows now record who wrote each message and what it referred to, so the model can
-- tell the people in a room apart.
--
-- Existing rows have none of this, and supporting them would mean a fallback in the code for every
-- field they lack. The context is short-lived anyway, so every room starts over, exactly as if
-- !chat reset had been run everywhere. The anchors point into the deleted rows, so they go too.
TRUNCATE chat_history, chat_context_anchors;

-- NOT NULL without a default only works because the table is now empty. Everything else is
-- nullable because it applies to only some rows, and the constraints below say which.
ALTER TABLE chat_history
    -- Every row a turn writes
    ADD COLUMN turn_event_id TEXT NOT NULL,     -- the event that triggered the turn
    ADD COLUMN thread_root_id TEXT,             -- the thread the turn happened in; NULL for the main timeline
    -- User rows: the message that triggered the turn
    ADD COLUMN sender_name TEXT,                -- display name at send time
    ADD COLUMN sent_at TIMESTAMPTZ,             -- origin_server_ts, for display only; rows are ordered by timestamp
    ADD COLUMN mentions JSONB,                  -- [{"user_id": ..., "name": ...}], possibly empty
    ADD COLUMN unseen_before INTEGER,           -- room messages the bot did not see since the previous one
    -- User rows that refer to another message: the message they reply to, or the thread they start
    ADD COLUMN reply_to_event_id TEXT,
    ADD COLUMN reply_to_sender TEXT,
    ADD COLUMN reply_to_sender_name TEXT,
    ADD COLUMN reply_to_sent_at TIMESTAMPTZ,
    ADD COLUMN reply_to_kind TEXT CHECK (reply_to_kind IN ('text', 'image', 'deleted')),
    ADD COLUMN reply_to_body TEXT,              -- set for kind 'text' only
    -- Answer rows: the event the answer was delivered as, so a redacted answer can be found
    ADD COLUMN answer_event_id TEXT,
    -- The invariants the code relies on instead of checking for itself. A CHECK passes when its
    -- expression is NULL, so each one tests IS NOT NULL explicitly rather than trusting comparisons.
    ADD CONSTRAINT chat_history_message_type CHECK (
        message_type IN ('text', 'tool_call', 'tool_response')),
    ADD CONSTRAINT chat_history_tool_row_complete CHECK (message_type = 'text' OR (
        tool_call_id IS NOT NULL AND tool_name IS NOT NULL)),
    -- An answer that couldn't be delivered isn't stored, so every stored answer has its event
    ADD CONSTRAINT chat_history_answer_complete CHECK (
        role <> 'assistant' OR message_type <> 'text' OR answer_event_id IS NOT NULL),
    ADD CONSTRAINT chat_history_user_row_complete CHECK (role <> 'user' OR (
        sender_name IS NOT NULL AND sent_at IS NOT NULL AND
        mentions IS NOT NULL AND unseen_before IS NOT NULL)),
    ADD CONSTRAINT chat_history_reply_complete CHECK (reply_to_event_id IS NULL OR (
        reply_to_sender IS NOT NULL AND reply_to_sender_name IS NOT NULL AND
        reply_to_sent_at IS NOT NULL AND reply_to_kind IS NOT NULL AND
        (reply_to_kind = 'text') = (reply_to_body IS NOT NULL)));

-- Redactions look rows up by the event they came from
CREATE INDEX chat_history_turn_event_idx ON chat_history (room_id, turn_event_id);
CREATE INDEX chat_history_reply_to_idx ON chat_history (room_id, reply_to_event_id);
CREATE INDEX chat_history_answer_event_idx ON chat_history (room_id, answer_event_id);

-- How many messages were posted in each room, not addressed to the bot, since the last message
-- that was. Only the count is kept, never the messages.
CREATE TABLE chat_unseen_messages (
    room_id TEXT PRIMARY KEY,
    count INTEGER NOT NULL
);

COMMIT;
