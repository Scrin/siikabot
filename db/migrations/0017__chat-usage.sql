BEGIN;

-- Per-turn usage accounting.
--
-- This lives in the database rather than in Prometheus because the metrics endpoint is served
-- without authentication, so it must never carry room or user identifiers. The same data is
-- perfectly safe here, where it is only reachable through the authenticated admin API, and it is
-- queryable historically in a way metric labels would not be.
CREATE TABLE chat_usage (
    id BIGSERIAL PRIMARY KEY,
    timestamp TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    room_id TEXT NOT NULL,
    user_id TEXT NOT NULL,
    model TEXT NOT NULL,
    prompt_tokens INTEGER NOT NULL,
    completion_tokens INTEGER NOT NULL,
    cached_prompt_tokens INTEGER NOT NULL,
    tool_iterations INTEGER NOT NULL,
    has_image BOOLEAN NOT NULL,
    duration_ms INTEGER NOT NULL,
    outcome TEXT NOT NULL
);

CREATE INDEX chat_usage_timestamp_idx ON chat_usage (timestamp);
CREATE INDEX chat_usage_room_idx ON chat_usage (room_id, timestamp);

COMMIT;
