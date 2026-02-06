BEGIN;

CREATE TABLE room_daily_stats (
    room_id TEXT NOT NULL,
    stat_date DATE NOT NULL,
    message_count INTEGER NOT NULL DEFAULT 0,
    word_count INTEGER NOT NULL DEFAULT 0,
    character_count INTEGER NOT NULL DEFAULT 0,
    PRIMARY KEY (room_id, stat_date)
);

COMMIT;
