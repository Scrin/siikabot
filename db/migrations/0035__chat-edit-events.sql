BEGIN;

-- The edit whose text a stored question or quote holds, so that a redaction of the edit can bring
-- the message back to the version that stands. NULL for a message as it was sent. The rows stored
-- so far can't say which edit they hold, so the chat history starts over.
ALTER TABLE chat_history ADD COLUMN edit_event_id TEXT;
ALTER TABLE chat_references ADD COLUMN edit_event_id TEXT;

TRUNCATE chat_history, chat_references, chat_images, chat_context_anchors;

COMMIT;
