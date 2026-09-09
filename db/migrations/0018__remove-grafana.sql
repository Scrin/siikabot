BEGIN;

-- Remove the Grafana/Ruuvi query features and the two gating mechanisms that
-- existed only to serve them: DB-backed user authorizations and per-room
-- command enablement.

DROP TABLE IF EXISTS user_grafana_datasources;
DROP TABLE IF EXISTS grafana_datasources;   -- FK -> grafana_templates, so drop first
DROP TABLE IF EXISTS grafana_templates;
DROP TABLE IF EXISTS ruuvi_endpoints;

-- user_authorizations itself is kept: web_session_token (0010) backs web auth.
ALTER TABLE user_authorizations DROP COLUMN IF EXISTS grafana;

-- room_config itself is kept: it still holds all the chat parameters.
ALTER TABLE room_config DROP COLUMN IF EXISTS enabled_commands;

COMMIT;
