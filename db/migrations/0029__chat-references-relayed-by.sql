BEGIN;

-- The webhook a quoted message was relayed from, NULL for a message that wasn't. NULL is right for
-- every existing row, since no webhook existed before this, so the chat history is kept.
ALTER TABLE chat_references ADD COLUMN relayed_by TEXT;

COMMIT;
