package db

import (
	"context"
	"database/sql"
	"encoding/json"

	"github.com/rs/zerolog/log"
)

func migrateSQLiteToPostgres() (err error) {
	if err = migrateGrafanaUsers(); err != nil {
		return
	}
	if err = migrateRuuviEndpoints(); err != nil {
		return
	}
	if err = migrateReminders(); err != nil {
		return
	}
	if err = migrateGrafana(); err != nil {
		return
	}
	return nil
}

func migrateGrafanaUsers() error {
	ctx := context.Background()

	// Check if there are any users in Postgres already
	var count int
	err := pool.QueryRow(ctx, "SELECT COUNT(*) FROM user_authorizations").Scan(&count)
	if err != nil {
		log.Error().Err(err).Msg("Failed to check existing users in Postgres")
		return err
	}

	// If there are already users in Postgres, skip migration
	if count > 0 {
		log.Info().Int("existing_users", count).Msg("Skipping grafana users migration, users already exist in Postgres")
		return nil
	}

	// Get grafana users from SQLite
	stmt, err := db.Prepare("select v from kv where k = ?")
	if err != nil {
		log.Error().Err(err).Msg("Failed to prepare SQLite select statement for grafana users")
		return err
	}
	defer stmt.Close()

	var usersJson string
	err = stmt.QueryRow("grafana_users").Scan(&usersJson)
	if err != nil && err != sql.ErrNoRows {
		log.Error().Err(err).Msg("Failed to query grafana users from SQLite")
		return err
	}

	// If no users found, nothing to migrate
	if err == sql.ErrNoRows || usersJson == "" {
		return nil
	}

	// Parse the JSON array of users
	var users []string
	if err := json.Unmarshal([]byte(usersJson), &users); err != nil {
		log.Error().Err(err).Str("users_json", usersJson).Msg("Failed to unmarshal grafana users")
		return err
	}

	// Begin a transaction for the Postgres migration
	tx, err := pool.Begin(ctx)
	if err != nil {
		log.Error().Err(err).Msg("Failed to begin Postgres transaction")
		return err
	}
	defer tx.Rollback(ctx)

	// Insert each user
	for _, user := range users {
		if _, err := tx.Exec(ctx, "INSERT INTO user_authorizations (user_id, grafana) VALUES ($1, true) ON CONFLICT DO NOTHING", user); err != nil {
			log.Error().Err(err).Str("user_id", user).Msg("Failed to insert user into Postgres")
			return err
		}
	}

	// Commit the transaction
	if err := tx.Commit(ctx); err != nil {
		log.Error().Err(err).Msg("Failed to commit Postgres transaction")
		return err
	}

	return nil
}

func migrateRuuviEndpoints() error {
	ctx := context.Background()

	// Check if there are any endpoints in Postgres already
	var count int
	err := pool.QueryRow(ctx, "SELECT COUNT(*) FROM ruuvi_endpoints").Scan(&count)
	if err != nil {
		log.Error().Err(err).Msg("Failed to check existing ruuvi endpoints in Postgres")
		return err
	}

	// If there are already endpoints in Postgres, skip migration
	if count > 0 {
		log.Info().Int("existing_endpoints", count).Msg("Skipping ruuvi endpoints migration, endpoints already exist in Postgres")
		return nil
	}

	// Get ruuvi endpoints from SQLite
	stmt, err := db.Prepare("select v from kv where k = ?")
	if err != nil {
		log.Error().Err(err).Msg("Failed to prepare SQLite select statement for ruuvi endpoints")
		return err
	}
	defer stmt.Close()

	var endpointsJson string
	err = stmt.QueryRow("ruuvi_endpoints").Scan(&endpointsJson)
	if err != nil && err != sql.ErrNoRows {
		log.Error().Err(err).Msg("Failed to query ruuvi endpoints from SQLite")
		return err
	}

	// If no endpoints found, nothing to migrate
	if err == sql.ErrNoRows || endpointsJson == "" {
		return nil
	}

	// Parse the JSON array of endpoints
	var endpoints []struct {
		Name    string `json:"name"`
		BaseURL string `json:"base_url"`
		TagName string `json:"tag_name"`
	}
	if err := json.Unmarshal([]byte(endpointsJson), &endpoints); err != nil {
		log.Error().Err(err).Str("endpoints_json", endpointsJson).Msg("Failed to unmarshal ruuvi endpoints")
		return err
	}

	// Begin a transaction for the Postgres migration
	tx, err := pool.Begin(ctx)
	if err != nil {
		log.Error().Err(err).Msg("Failed to begin Postgres transaction")
		return err
	}
	defer tx.Rollback(ctx)

	// Insert each endpoint
	for _, endpoint := range endpoints {
		if _, err := tx.Exec(ctx, "INSERT INTO ruuvi_endpoints (name, base_url, tag_name) VALUES ($1, $2, $3) ON CONFLICT DO NOTHING",
			endpoint.Name, endpoint.BaseURL, endpoint.TagName); err != nil {
			log.Error().Err(err).Str("name", endpoint.Name).Msg("Failed to insert ruuvi endpoint into Postgres")
			return err
		}
	}

	// Commit the transaction
	if err := tx.Commit(ctx); err != nil {
		log.Error().Err(err).Msg("Failed to commit Postgres transaction")
		return err
	}

	return nil
}

func migrateReminders() error {
	ctx := context.Background()

	// Check if there are any reminders in Postgres already
	var count int
	err := pool.QueryRow(ctx, "SELECT COUNT(*) FROM reminders").Scan(&count)
	if err != nil {
		log.Error().Err(err).Msg("Failed to check existing reminders in Postgres")
		return err
	}

	// If there are already reminders in Postgres, skip migration
	if count > 0 {
		log.Info().Int("existing_reminders", count).Msg("Skipping reminders migration, reminders already exist in Postgres")
		return nil
	}

	// Get reminders from SQLite
	stmt, err := db.Prepare("select v from kv where k = ?")
	if err != nil {
		log.Error().Err(err).Msg("Failed to prepare SQLite select statement for reminders")
		return err
	}
	defer stmt.Close()

	var remindersJson string
	err = stmt.QueryRow("reminders").Scan(&remindersJson)
	if err != nil && err != sql.ErrNoRows {
		log.Error().Err(err).Msg("Failed to query reminders from SQLite")
		return err
	}

	// If no reminders found, nothing to migrate
	if err == sql.ErrNoRows || remindersJson == "" {
		return nil
	}

	// Parse the JSON array of reminders
	var reminders []struct {
		RemindTime int64  `json:"remind_time"`
		User       string `json:"user"`
		RoomID     string `json:"room_id"`
		Message    string `json:"msg"`
	}
	if err := json.Unmarshal([]byte(remindersJson), &reminders); err != nil {
		log.Error().Err(err).Str("reminders_json", remindersJson).Msg("Failed to unmarshal reminders")
		return err
	}

	// Begin a transaction for the Postgres migration
	tx, err := pool.Begin(ctx)
	if err != nil {
		log.Error().Err(err).Msg("Failed to begin Postgres transaction")
		return err
	}
	defer tx.Rollback(ctx)

	// Insert each reminder
	for _, reminder := range reminders {
		// Convert Unix timestamp to timestamptz by using to_timestamp
		if _, err := tx.Exec(ctx,
			"INSERT INTO reminders (remind_time, user_id, room_id, message) VALUES (to_timestamp($1), $2, $3, $4)",
			reminder.RemindTime, reminder.User, reminder.RoomID, reminder.Message); err != nil {
			log.Error().Err(err).
				Int64("remind_time", reminder.RemindTime).
				Str("user_id", reminder.User).
				Msg("Failed to insert reminder into Postgres")
			return err
		}
	}

	// Commit the transaction
	if err := tx.Commit(ctx); err != nil {
		log.Error().Err(err).Msg("Failed to commit Postgres transaction")
		return err
	}

	return nil
}

func migrateGrafana() error {
	ctx := context.Background()

	// Check if there are any templates in Postgres already
	var count int
	err := pool.QueryRow(ctx, "SELECT COUNT(*) FROM grafana_templates").Scan(&count)
	if err != nil {
		log.Error().Err(err).Msg("Failed to check existing grafana templates in Postgres")
		return err
	}

	// If there are already templates in Postgres, skip migration
	if count > 0 {
		log.Info().Int("existing_templates", count).Msg("Skipping grafana migration, templates already exist in Postgres")
		return nil
	}

	// Get grafana configs from SQLite
	stmt, err := db.Prepare("select v from kv where k = ?")
	if err != nil {
		log.Error().Err(err).Msg("Failed to prepare SQLite select statement for grafana configs")
		return err
	}
	defer stmt.Close()

	var configsJson string
	err = stmt.QueryRow("grafana_configs").Scan(&configsJson)
	if err != nil && err != sql.ErrNoRows {
		log.Error().Err(err).Msg("Failed to query grafana configs from SQLite")
		return err
	}

	// If no configs found, nothing to migrate
	if err == sql.ErrNoRows || configsJson == "" {
		return nil
	}

	// Parse the JSON map of configs
	var configs map[string]struct {
		Template string            `json:"template"`
		Sources  map[string]string `json:"sources"`
	}
	if err := json.Unmarshal([]byte(configsJson), &configs); err != nil {
		log.Error().Err(err).Str("configs_json", configsJson).Msg("Failed to unmarshal grafana configs")
		return err
	}

	// Begin a transaction for the Postgres migration
	tx, err := pool.Begin(ctx)
	if err != nil {
		log.Error().Err(err).Msg("Failed to begin Postgres transaction")
		return err
	}
	defer tx.Rollback(ctx)

	// Insert each template and its datasources
	for name, config := range configs {
		// Insert template
		if _, err := tx.Exec(ctx, "INSERT INTO grafana_templates (name, template) VALUES ($1, $2) ON CONFLICT DO NOTHING",
			name, config.Template); err != nil {
			log.Error().Err(err).Str("name", name).Msg("Failed to insert grafana template into Postgres")
			return err
		}

		// Insert datasources for this template
		for sourceName, sourceURL := range config.Sources {
			if _, err := tx.Exec(ctx,
				"INSERT INTO grafana_datasources (name, template_name, url) VALUES ($1, $2, $3) ON CONFLICT DO NOTHING",
				sourceName, name, sourceURL); err != nil {
				log.Error().Err(err).
					Str("template_name", name).
					Str("source_name", sourceName).
					Msg("Failed to insert grafana datasource into Postgres")
				return err
			}
		}
	}

	// Commit the transaction
	if err := tx.Commit(ctx); err != nil {
		log.Error().Err(err).Msg("Failed to commit Postgres transaction")
		return err
	}

	return nil
}
