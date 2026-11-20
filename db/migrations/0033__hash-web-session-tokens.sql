BEGIN;

-- Dashboard session tokens are kept only as their SHA-256 hashes, like webhook tokens, so the database holds
-- nothing that would log anyone in. A session also ends once it has gone unused for a while, which the code
-- checks against web_session_last_used_at.
--
-- The sessions people have now are kept: each token is replaced by its hash, which is what the code looks up
-- from now on, and counts as used now.

ALTER TABLE user_authorizations
    ADD COLUMN web_session_token_hash TEXT,
    ADD COLUMN web_session_last_used_at TIMESTAMPTZ;

UPDATE user_authorizations
SET web_session_token_hash = encode(sha256(convert_to(web_session_token, 'UTF8')), 'hex'),
    web_session_last_used_at = now()
WHERE web_session_token IS NOT NULL;

ALTER TABLE user_authorizations DROP COLUMN web_session_token;

CREATE UNIQUE INDEX user_authorizations_web_session_token_hash_idx
    ON user_authorizations (web_session_token_hash);

ALTER TABLE user_authorizations ADD CONSTRAINT user_authorizations_web_session_complete
    CHECK ((web_session_token_hash IS NULL) = (web_session_last_used_at IS NULL));

COMMIT;
