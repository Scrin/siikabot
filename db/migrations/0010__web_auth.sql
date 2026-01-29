BEGIN;

ALTER TABLE user_authorizations
ADD COLUMN web_session_token TEXT;

COMMIT;
