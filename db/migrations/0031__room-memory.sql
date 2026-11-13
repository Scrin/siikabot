BEGIN;

-- The bot's own memories of group rooms. It forms them by thinking back over the conversations it
-- had in a room once the room has gone quiet, and they are shown to the model in that room's turns.
-- Nothing in a room changes them directly: a later pass revises them, and the admin can delete them
-- on the dashboard.
CREATE TABLE room_memory (
    id BIGSERIAL PRIMARY KEY,
    room_id TEXT NOT NULL,
    memory TEXT NOT NULL CHECK (char_length(memory) BETWEEN 1 AND 300),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX room_memory_room_idx ON room_memory (room_id, updated_at);

-- How far the bot has thought back over each room's chat history. reflected_until is the id of the
-- last chat_history row a pass covered: ids are never reused, as the history is only ever emptied
-- without RESTART IDENTITY. reflected_at is when a pass last completed, and failed_attempts how many
-- have failed since then, with last_attempt_at when the latest of any kind was made.
--
-- A room without a row has never been thought back over, so its whole history is new to the bot.
CREATE TABLE room_memory_progress (
    room_id TEXT PRIMARY KEY,
    reflected_until BIGINT NOT NULL,
    reflected_at TIMESTAMPTZ,
    failed_attempts INTEGER NOT NULL DEFAULT 0 CHECK (failed_attempts >= 0),
    last_attempt_at TIMESTAMPTZ NOT NULL
);

-- chat_usage records the bot's passes over a room as well as its turns. Every existing row is a turn.
ALTER TABLE chat_usage ADD COLUMN kind TEXT NOT NULL DEFAULT 'turn' CHECK (kind IN ('turn', 'reflection'));
ALTER TABLE chat_usage ALTER COLUMN kind DROP DEFAULT;

-- How long a room has to be quiet after a chat turn before the bot thinks back over it. NULL turns
-- the passes off. The existing rows get 30 minutes, which turns the feature on with the latest one.
ALTER TABLE chat_config ADD COLUMN room_memory_delay INTERVAL DEFAULT '30 minutes'
    CHECK (room_memory_delay > INTERVAL '0');
ALTER TABLE chat_config ALTER COLUMN room_memory_delay DROP DEFAULT;

COMMIT;
