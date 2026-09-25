BEGIN;

-- Chat parameters are global rather than per room, and every change is kept.
--
-- Every chat parameter used to be a nullable per-room override in room_config, falling back to a
-- default in the code. The bot has one admin and one set of preferences, so the granularity bought
-- nothing. The per-room values are deliberately not carried over.
--
-- chat_config is append-only: a change inserts a complete new row rather than updating one, so the
-- row with the highest id is the configuration in effect and the older rows are its history. Each
-- row is a full snapshot, never a delta, so reading the configuration is always a single row.
--
-- This table is the only source of these values: the code has no fallbacks. The checks make sure
-- no row, including one written by hand, holds a configuration the bot cannot run with.
CREATE TABLE chat_config (
    id BIGSERIAL PRIMARY KEY,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    text_model TEXT NOT NULL CHECK (text_model <> ''),
    image_model TEXT NOT NULL CHECK (image_model <> ''),
    context_high_tokens INTEGER NOT NULL,
    context_low_tokens INTEGER NOT NULL CHECK (context_low_tokens > 0),
    max_tool_iterations INTEGER NOT NULL CHECK (max_tool_iterations > 0),
    max_tokens INTEGER NOT NULL CHECK (max_tokens > 0),
    image_detail TEXT NOT NULL CHECK (image_detail IN ('low', 'high', 'auto')),
    max_web_content_size INTEGER NOT NULL CHECK (max_web_content_size > 0),
    -- The window grows to the high mark and is then trimmed back to the low mark, so the marks have
    -- to straddle a usable range or the window would be trimmed on every turn
    CHECK (context_low_tokens < context_high_tokens)
);

-- The starting configuration: the defaults the code used until now, except the text model, which
-- moves from deepseek-v4-pro-0813 to the newer deepseek-v4.1-flash.
--
-- context: the window grows until it exceeds the high mark, then the anchor jumps forward far enough
-- to bring it under the low mark and stays put until the next overflow. Evicting one message per
-- turn instead would change the prompt prefix on every request and defeat provider-side caching.
-- max_tokens: generous enough that ordinary answers are unaffected, since the system prompt already
-- asks for concise replies, but a backstop so a runaway generation is not billed in full.
-- image_detail: 'low' costs a flat, small number of tokens per image, where the provider default
-- tiles the image and can cost thousands for the same picture. 'auto' leaves it to the provider.
INSERT INTO chat_config (
    text_model, image_model, context_high_tokens, context_low_tokens,
    max_tool_iterations, max_tokens, image_detail, max_web_content_size
) VALUES (
    'openrouter/deepseek/deepseek-v4.1-flash', 'openrouter/openai/gpt-5.6-luna', 16384, 8192,
    5, 2048, 'low', 10240
);

-- The context anchor is not configuration but per-room state: the history row each room's rolling
-- context window currently starts at. It is the only per-room value left, so it gets a table of its
-- own and room_config goes. Existing anchors are not copied: each room re-anchors on its next turn.
CREATE TABLE chat_context_anchors (
    room_id TEXT PRIMARY KEY,
    anchor_id BIGINT NOT NULL
);

DROP TABLE room_config;

COMMIT;
