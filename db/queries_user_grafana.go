package db

import (
	"context"
	"time"

	pgx "github.com/jackc/pgx/v5"
	"github.com/rs/zerolog/log"
)

// UserGrafanaDatasource represents a user-defined Grafana datasource
type UserGrafanaDatasource struct {
	ID          int64     `db:"id"`
	UserID      string    `db:"user_id"`
	Name        string    `db:"name"`
	Description string    `db:"description"`
	URL         string    `db:"url"`
	CreatedAt   time.Time `db:"created_at"`
}

// SaveUserGrafanaDatasource saves or updates a user's Grafana datasource
func SaveUserGrafanaDatasource(ctx context.Context, userID, name, description, url string) error {
	_, err := pool.Exec(ctx,
		`INSERT INTO user_grafana_datasources (user_id, name, description, url)
		 VALUES ($1, $2, $3, $4)
		 ON CONFLICT (user_id, name) DO UPDATE SET description = $3, url = $4`,
		userID, name, description, url)
	if err != nil {
		log.Error().Ctx(ctx).Err(err).
			Str("user_id", userID).
			Str("name", name).
			Msg("Failed to save user grafana datasource")
		return err
	}
	return nil
}

// GetUserGrafanaDatasources returns all Grafana datasources for a user
func GetUserGrafanaDatasources(ctx context.Context, userID string) ([]UserGrafanaDatasource, error) {
	rows, err := pool.Query(ctx,
		`SELECT id, user_id, name, description, url, created_at
		 FROM user_grafana_datasources
		 WHERE user_id = $1
		 ORDER BY name ASC`,
		userID)
	if err != nil {
		log.Error().Ctx(ctx).Err(err).Str("user_id", userID).Msg("Failed to query user grafana datasources")
		return nil, err
	}
	return pgx.CollectRows(rows, pgx.RowToStructByName[UserGrafanaDatasource])
}

// GetUserGrafanaDatasourceByName returns a specific datasource by name
func GetUserGrafanaDatasourceByName(ctx context.Context, userID, name string) (*UserGrafanaDatasource, error) {
	rows, err := pool.Query(ctx,
		`SELECT id, user_id, name, description, url, created_at
		 FROM user_grafana_datasources
		 WHERE user_id = $1 AND name = $2`,
		userID, name)
	if err != nil {
		log.Error().Ctx(ctx).Err(err).
			Str("user_id", userID).
			Str("name", name).
			Msg("Failed to query user grafana datasource by name")
		return nil, err
	}
	datasources, err := pgx.CollectRows(rows, pgx.RowToStructByName[UserGrafanaDatasource])
	if err != nil {
		return nil, err
	}
	if len(datasources) == 0 {
		return nil, nil
	}
	return &datasources[0], nil
}

// DeleteUserGrafanaDatasource deletes a specific datasource for a user by ID
func DeleteUserGrafanaDatasource(ctx context.Context, userID string, datasourceID int64) error {
	result, err := pool.Exec(ctx,
		"DELETE FROM user_grafana_datasources WHERE id = $1 AND user_id = $2",
		datasourceID, userID)
	if err != nil {
		log.Error().Ctx(ctx).Err(err).
			Str("user_id", userID).
			Int64("datasource_id", datasourceID).
			Msg("Failed to delete user grafana datasource")
		return err
	}
	if result.RowsAffected() == 0 {
		log.Warn().Ctx(ctx).
			Str("user_id", userID).
			Int64("datasource_id", datasourceID).
			Msg("No datasource found to delete")
	}
	return nil
}

// DeleteAllUserGrafanaDatasources deletes all datasources for a user
func DeleteAllUserGrafanaDatasources(ctx context.Context, userID string) (int64, error) {
	result, err := pool.Exec(ctx,
		"DELETE FROM user_grafana_datasources WHERE user_id = $1",
		userID)
	if err != nil {
		log.Error().Ctx(ctx).Err(err).
			Str("user_id", userID).
			Msg("Failed to delete all user grafana datasources")
		return 0, err
	}
	return result.RowsAffected(), nil
}
