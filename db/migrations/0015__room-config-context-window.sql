BEGIN;

-- Replace the message-count history limit with a token budget for the rolling context window.
--
-- The old limit counted rows, including tool_call and tool_response rows, so one question answered
-- with three tools consumed eight of the twenty slots. It also slid by one row on every turn, which
-- changed the prompt prefix constantly and made provider-side prompt caching impossible.
--
-- The replacement anchors the window and only moves it when the budget overflows: the context grows
-- until it exceeds chat_context_high_tokens, then the anchor jumps forward far enough to bring it
-- under chat_context_low_tokens and stays put until the next overflow.
ALTER TABLE room_config
    ADD COLUMN chat_context_high_tokens INTEGER,
    ADD COLUMN chat_context_low_tokens INTEGER,
    ADD COLUMN chat_context_anchor_id BIGINT,
    DROP COLUMN chat_max_history_messages;

COMMIT;
