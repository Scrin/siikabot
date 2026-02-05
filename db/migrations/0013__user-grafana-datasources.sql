BEGIN;

CREATE TABLE user_grafana_datasources (
    id SERIAL PRIMARY KEY,
    user_id TEXT NOT NULL,
    name TEXT NOT NULL,
    description TEXT NOT NULL,
    url TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (user_id, name)
);

CREATE INDEX user_grafana_datasources_user_idx ON user_grafana_datasources (user_id);

COMMIT;
