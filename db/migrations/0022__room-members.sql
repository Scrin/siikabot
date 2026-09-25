BEGIN;

-- The table recorded whoever sent a membership event instead of the member it was about, and never
-- removed anyone. Room keys were shared with the members it listed, people who had left included,
-- and dashboard access was checked against it. Its rows can't be repaired, so they go, and the bot
-- rebuilds the table from the homeserver when it starts.
DROP TABLE room_members;

-- The members of the rooms the bot is in: who has joined, and who is invited. Anyone who leaves, is
-- kicked or is banned is removed.
CREATE TABLE room_members (
    room_id TEXT NOT NULL,
    user_id TEXT NOT NULL,
    membership TEXT NOT NULL CHECK (membership IN ('join', 'invite')),
    display_name TEXT,              -- their display name in the room; NULL if they have none
    since TIMESTAMPTZ NOT NULL,     -- when their current membership began, as far as the bot knows
    PRIMARY KEY (room_id, user_id)
);

-- The dashboard and the crypto machine look rooms up by member
CREATE INDEX room_members_user_idx ON room_members (user_id);

COMMIT;
