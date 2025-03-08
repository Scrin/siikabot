BEGIN;

-- Table for storing chat history
CREATE TABLE chat_history (
    id SERIAL PRIMARY KEY,
    room_id TEXT NOT NULL,
    user_id TEXT NOT NULL,
    message TEXT NOT NULL,
    role TEXT NOT NULL,
    timestamp TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- Create index for faster lookups by room
CREATE INDEX chat_history_room_idx ON chat_history (room_id);

COMMIT; 
