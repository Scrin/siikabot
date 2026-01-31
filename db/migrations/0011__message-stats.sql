BEGIN;

CREATE TABLE message_stats (
    room_id TEXT NOT NULL,
    user_id TEXT NOT NULL,
    first_seen TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    last_seen TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    message_count INTEGER NOT NULL DEFAULT 0,
    word_count INTEGER NOT NULL DEFAULT 0,
    character_count INTEGER NOT NULL DEFAULT 0,
    PRIMARY KEY (room_id, user_id)
);

COMMIT;
