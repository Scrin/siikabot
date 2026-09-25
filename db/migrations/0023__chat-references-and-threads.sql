BEGIN;

-- A turn can now refer to several messages, the one it replies to and the ones it links to, and the
-- message that started it can carry an image of its own. Each thread also gets a context window of
-- its own. What a turn stores changes, so the history starts over, as with 0020, and the anchors,
-- which point into it, go too.
TRUNCATE chat_history, chat_context_anchors;

ALTER TABLE chat_history
    -- The message a turn replies to moves to chat_references, with the messages it links to
    DROP CONSTRAINT chat_history_reply_complete,
    DROP COLUMN reply_to_event_id,
    DROP COLUMN reply_to_sender,
    DROP COLUMN reply_to_sender_name,
    DROP COLUMN reply_to_sent_at,
    DROP COLUMN reply_to_kind,
    DROP COLUMN reply_to_body,
    -- User rows: the message was an image, and its text is the image's caption
    ADD COLUMN has_image BOOLEAN,
    DROP CONSTRAINT chat_history_user_row_complete,
    ADD CONSTRAINT chat_history_user_row_complete CHECK (role <> 'user' OR (
        sender_name IS NOT NULL AND sent_at IS NOT NULL AND
        mentions IS NOT NULL AND unseen_before IS NOT NULL AND has_image IS NOT NULL));

-- The messages a user turn explicitly refers to, as they were when the turn happened: the message
-- it replies to, or the root of the thread it starts, and the messages in the room it links to.
-- They go with the turn, and a redacted one is kept as 'deleted', without its text.
CREATE TABLE chat_references (
    history_id BIGINT NOT NULL REFERENCES chat_history (id) ON DELETE CASCADE,
    position SMALLINT NOT NULL,         -- the order the turn shows them in
    relation TEXT NOT NULL CHECK (relation IN ('reply', 'link')),
    event_id TEXT NOT NULL,
    sender TEXT NOT NULL,
    sender_name TEXT NOT NULL,          -- display name when the turn happened
    sent_at TIMESTAMPTZ NOT NULL,
    kind TEXT NOT NULL CHECK (kind IN ('text', 'image', 'deleted')),
    body TEXT,                          -- the text, or an image's caption; never kept once deleted
    PRIMARY KEY (history_id, position),
    CONSTRAINT chat_references_body CHECK (CASE kind
        WHEN 'text' THEN body IS NOT NULL
        WHEN 'deleted' THEN body IS NULL
        ELSE TRUE END)
);

-- A turn replies to one message at most
CREATE UNIQUE INDEX chat_references_one_reply_idx ON chat_references (history_id) WHERE relation = 'reply';

-- Redactions and edits look references up by the message they refer to
CREATE INDEX chat_references_event_idx ON chat_references (event_id);

-- Each thread has a window of its own, read by thread
CREATE INDEX chat_history_thread_idx ON chat_history (room_id, thread_root_id);

-- And an anchor of its own. The main timeline is the empty string here rather than NULL as in
-- chat_history, since a primary key can't hold NULL.
DROP TABLE chat_context_anchors;
CREATE TABLE chat_context_anchors (
    room_id TEXT NOT NULL,
    thread_root_id TEXT NOT NULL,
    anchor_id BIGINT NOT NULL,
    PRIMARY KEY (room_id, thread_root_id)
);

COMMIT;
