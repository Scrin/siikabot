BEGIN;

CREATE TABLE user_authorizations (
    user_id TEXT PRIMARY KEY,
    grafana BOOLEAN NOT NULL DEFAULT FALSE
);

CREATE TABLE ruuvi_endpoints (
    name TEXT PRIMARY KEY,
    base_url TEXT NOT NULL,
    tag_name TEXT NOT NULL
);

CREATE TABLE reminders (
    id SERIAL PRIMARY KEY,
    remind_time TIMESTAMPTZ NOT NULL,
    user_id TEXT NOT NULL,
    room_id TEXT NOT NULL,
    message TEXT NOT NULL
);

CREATE TABLE grafana_templates (
    name TEXT PRIMARY KEY,
    template TEXT NOT NULL
);

CREATE TABLE grafana_datasources (
    name TEXT NOT NULL,
    template_name TEXT NOT NULL REFERENCES grafana_templates(name) ON DELETE CASCADE,
    url TEXT NOT NULL,
    PRIMARY KEY (name, template_name)
);

COMMIT;
