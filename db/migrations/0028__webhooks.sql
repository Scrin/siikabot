BEGIN;

-- Webhooks that post messages into a room the bot is in, managed by the admin on the web dashboard.
-- Everything but the ID can be changed later.
CREATE TABLE webhooks (
    id TEXT PRIMARY KEY,                    -- random, and part of the webhook's URL
    name TEXT NOT NULL CHECK (name <> ''),  -- shown in brackets before every message
    room_id TEXT NOT NULL,                  -- where the messages go
    token_hash BYTEA NOT NULL,              -- SHA-256 of the bearer token, which can't be read back
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    last_used_at TIMESTAMPTZ                -- when a message was last delivered, NULL if never
);

-- A name always means the same webhook in a room, whatever its case
CREATE UNIQUE INDEX webhooks_name_idx ON webhooks (lower(name));

COMMIT;
