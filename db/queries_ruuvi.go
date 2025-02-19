package db

import (
	"context"

	pgx "github.com/jackc/pgx/v5"
	"github.com/rs/zerolog/log"
)

type RuuviEndpoint struct {
	Name    string `db:"name"`
	BaseURL string `db:"base_url"`
	TagName string `db:"tag_name"`
}

func GetRuuviEndpoints(ctx context.Context) ([]RuuviEndpoint, error) {
	rows, err := pool.Query(ctx, "SELECT name, base_url, tag_name FROM ruuvi_endpoints")
	if err != nil {
		log.Error().Ctx(ctx).Err(err).Msg("Failed to query ruuvi endpoints")
		return nil, err
	}
	return pgx.CollectRows(rows, pgx.RowToStructByName[RuuviEndpoint])
}

func AddRuuviEndpoint(ctx context.Context, endpoint RuuviEndpoint) error {
	_, err := pool.Exec(ctx,
		"INSERT INTO ruuvi_endpoints (name, base_url, tag_name) VALUES ($1, $2, $3) ON CONFLICT (name) DO UPDATE SET base_url = $2, tag_name = $3",
		endpoint.Name, endpoint.BaseURL, endpoint.TagName)
	if err != nil {
		log.Error().Ctx(ctx).Err(err).
			Str("name", endpoint.Name).
			Str("base_url", endpoint.BaseURL).
			Str("tag_name", endpoint.TagName).
			Msg("Failed to insert/update ruuvi endpoint")
		return err
	}
	return nil
}

func RemoveRuuviEndpoint(ctx context.Context, name string) error {
	_, err := pool.Exec(ctx, "DELETE FROM ruuvi_endpoints WHERE name = $1", name)
	if err != nil {
		log.Error().Ctx(ctx).Err(err).Str("name", name).Msg("Failed to delete ruuvi endpoint")
		return err
	}
	return nil
}
