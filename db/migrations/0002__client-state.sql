BEGIN;

-- Tables for Matrix client state storage

-- Store filter IDs for users
CREATE TABLE user_filter_ids (
    user_id TEXT PRIMARY KEY,
    filter_id TEXT NOT NULL
);

-- Store next batch tokens for users
CREATE TABLE user_batch_tokens (
    user_id TEXT PRIMARY KEY,
    next_batch_token TEXT NOT NULL
);

-- Store room information including encryption events
CREATE TABLE rooms (
    room_id TEXT PRIMARY KEY,
    encryption_event JSONB
);

-- Store room membership information
CREATE TABLE room_members (
    room_id TEXT NOT NULL,
    user_id TEXT NOT NULL,
    PRIMARY KEY (room_id, user_id)
);

COMMIT;
