package db

import (
	"context"

	"github.com/rs/zerolog/log"
)

type GrafanaConfig struct {
	TemplateString string
	DataSources    map[string]string
}

func GetGrafanaConfigs(ctx context.Context) (map[string]GrafanaConfig, error) {
	// First get all templates
	rows, err := pool.Query(ctx, "SELECT name, template FROM grafana_templates ORDER BY name")
	if err != nil {
		log.Error().Ctx(ctx).Err(err).Msg("Failed to query grafana templates")
		return nil, err
	}
	defer rows.Close()

	configs := make(map[string]GrafanaConfig)
	for rows.Next() {
		var name, template string
		if err := rows.Scan(&name, &template); err != nil {
			log.Error().Ctx(ctx).Err(err).Msg("Failed to scan grafana template")
			return nil, err
		}
		configs[name] = GrafanaConfig{
			TemplateString: template,
			DataSources:    make(map[string]string),
		}
	}
	if err := rows.Err(); err != nil {
		log.Error().Ctx(ctx).Err(err).Msg("Error iterating over grafana templates")
		return nil, err
	}

	// Then get all datasources and add them to their respective templates
	rows, err = pool.Query(ctx, "SELECT template_name, name, url FROM grafana_datasources ORDER BY template_name, name")
	if err != nil {
		log.Error().Ctx(ctx).Err(err).Msg("Failed to query grafana datasources")
		return nil, err
	}
	defer rows.Close()

	for rows.Next() {
		var templateName, name, url string
		if err := rows.Scan(&templateName, &name, &url); err != nil {
			log.Error().Ctx(ctx).Err(err).Msg("Failed to scan grafana datasource")
			return nil, err
		}
		if config, ok := configs[templateName]; ok {
			config.DataSources[name] = url
			configs[templateName] = config
		}
	}
	if err := rows.Err(); err != nil {
		log.Error().Ctx(ctx).Err(err).Msg("Error iterating over grafana datasources")
		return nil, err
	}

	return configs, nil
}

func AddGrafanaTemplate(ctx context.Context, name, templateString string) error {
	_, err := pool.Exec(ctx,
		"INSERT INTO grafana_templates (name, template) VALUES ($1, $2) ON CONFLICT (name) DO UPDATE SET template = $2",
		name, templateString)
	if err != nil {
		log.Error().Ctx(ctx).Err(err).
			Str("name", name).
			Str("template", templateString).
			Msg("Failed to insert/update grafana template")
		return err
	}
	return nil
}

func SetGrafanaDatasource(ctx context.Context, templateName, sourceName, url string) error {
	if url == "-" {
		_, err := pool.Exec(ctx,
			"DELETE FROM grafana_datasources WHERE template_name = $1 AND name = $2",
			templateName, sourceName)
		if err != nil {
			log.Error().Ctx(ctx).Err(err).
				Str("template_name", templateName).
				Str("source_name", sourceName).
				Msg("Failed to delete grafana datasource")
			return err
		}
		return nil
	}

	_, err := pool.Exec(ctx,
		"INSERT INTO grafana_datasources (template_name, name, url) VALUES ($1, $2, $3) ON CONFLICT (template_name, name) DO UPDATE SET url = $3",
		templateName, sourceName, url)
	if err != nil {
		log.Error().Ctx(ctx).Err(err).
			Str("template_name", templateName).
			Str("source_name", sourceName).
			Str("url", url).
			Msg("Failed to insert/update grafana datasource")
		return err
	}
	return nil
}

func RemoveGrafanaTemplate(ctx context.Context, name string) error {
	// The datasources will be automatically deleted due to ON DELETE CASCADE
	_, err := pool.Exec(ctx, "DELETE FROM grafana_templates WHERE name = $1", name)
	if err != nil {
		log.Error().Ctx(ctx).Err(err).
			Str("name", name).
			Msg("Failed to delete grafana template")
		return err
	}
	return nil
}

func AuthorizeGrafanaUser(ctx context.Context, userID string) error {
	_, err := pool.Exec(ctx,
		"INSERT INTO user_authorizations (user_id, grafana) VALUES ($1, true) ON CONFLICT (user_id) DO UPDATE SET grafana = true",
		userID)
	if err != nil {
		log.Error().Ctx(ctx).Err(err).
			Str("user_id", userID).
			Msg("Failed to authorize user for grafana")
		return err
	}
	return nil
}

func IsGrafanaAuthorized(ctx context.Context, userID string) bool {
	var authorized bool
	err := pool.QueryRow(ctx,
		"SELECT grafana FROM user_authorizations WHERE user_id = $1",
		userID).Scan(&authorized)
	if err != nil {
		log.Error().Ctx(ctx).Err(err).
			Str("user_id", userID).
			Msg("Failed to check user grafana authorization")
		return false
	}
	return authorized
}
