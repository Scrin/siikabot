BEGIN;

-- Table for storing user memories
CREATE TABLE user_memory (
    id SERIAL PRIMARY KEY,
    user_id TEXT NOT NULL,
    memory TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- Create index for faster lookups by user
CREATE INDEX user_memory_user_idx ON user_memory (user_id);

COMMIT;
