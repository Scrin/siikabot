package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/url"
	"slices"
	"strings"

	"github.com/Scrin/siikabot/commands/grafana"
	"github.com/Scrin/siikabot/db"
	"github.com/rs/zerolog/log"
)

// GrafanaDatasourceResponse represents a single datasource in the API response
type GrafanaDatasourceResponse struct {
	Name string `json:"name"`
	URL  string `json:"url"`
}

// GrafanaTemplateResponse represents a single template in the API response
type GrafanaTemplateResponse struct {
	Name        string                      `json:"name"`
	Template    string                      `json:"template"`
	Datasources []GrafanaDatasourceResponse `json:"datasources"`
}

// GrafanaTemplatesResponse is the response for the templates list endpoint
type GrafanaTemplatesResponse struct {
	Templates []GrafanaTemplateResponse `json:"templates"`
}

// CreateTemplateRequest is the request body for creating a template
type CreateTemplateRequest struct {
	Name     string `json:"name"`
	Template string `json:"template"`
}

// UpdateTemplateRequest is the request body for updating a template
type UpdateTemplateRequest struct {
	Template string `json:"template"`
}

// SetDatasourceRequest is the request body for setting a datasource
type SetDatasourceRequest struct {
	URL string `json:"url"`
}

// GrafanaRenderResponse is the response for the render endpoint
type GrafanaRenderResponse struct {
	Rendered string `json:"rendered"`
}

// GrafanaTemplatesHandler handles /api/grafana/templates
// GET: List all templates with their datasources
// POST: Create a new template
func GrafanaTemplatesHandler(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	userID, ok := GetUserIDFromContext(ctx)
	if !ok {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusUnauthorized)
		json.NewEncoder(w).Encode(ErrorResponse{Error: "Not authenticated"})
		return
	}

	if !db.IsGrafanaAuthorized(ctx, userID) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusForbidden)
		json.NewEncoder(w).Encode(ErrorResponse{Error: "Grafana access not authorized"})
		return
	}

	switch r.Method {
	case http.MethodGet:
		handleListTemplates(ctx, w)
	case http.MethodPost:
		handleCreateTemplate(ctx, w, r)
	default:
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

// GrafanaTemplateRouteHandler handles /api/grafana/templates/{name} and /api/grafana/templates/{name}/datasources/{sourceName}
func GrafanaTemplateRouteHandler(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	userID, ok := GetUserIDFromContext(ctx)
	if !ok {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusUnauthorized)
		json.NewEncoder(w).Encode(ErrorResponse{Error: "Not authenticated"})
		return
	}

	if !db.IsGrafanaAuthorized(ctx, userID) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusForbidden)
		json.NewEncoder(w).Encode(ErrorResponse{Error: "Grafana access not authorized"})
		return
	}

	// Parse the path: /api/grafana/templates/{name}[/datasources/{sourceName}]
	path := strings.TrimPrefix(r.URL.Path, "/api/grafana/templates/")
	if path == "" {
		http.Error(w, "Template name required", http.StatusBadRequest)
		return
	}

	parts := strings.Split(path, "/")
	templateName, err := url.PathUnescape(parts[0])
	if err != nil {
		http.Error(w, "Invalid template name", http.StatusBadRequest)
		return
	}

	if len(parts) == 1 {
		// /api/grafana/templates/{name}
		handleTemplateOperations(ctx, w, r, templateName)
	} else if len(parts) == 2 && parts[1] == "render" {
		// /api/grafana/templates/{name}/render
		handleRenderTemplate(ctx, w, r, templateName)
	} else if len(parts) == 3 && parts[1] == "datasources" {
		// /api/grafana/templates/{name}/datasources/{sourceName}
		sourceName, err := url.PathUnescape(parts[2])
		if err != nil {
			http.Error(w, "Invalid datasource name", http.StatusBadRequest)
			return
		}
		handleDatasourceOperations(ctx, w, r, templateName, sourceName)
	} else {
		http.Error(w, "Not found", http.StatusNotFound)
	}
}

func handleListTemplates(ctx context.Context, w http.ResponseWriter) {
	configs, err := db.GetGrafanaConfigs(ctx)
	if err != nil {
		log.Error().Ctx(ctx).Err(err).Msg("Failed to fetch grafana configs")
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(ErrorResponse{Error: "Failed to fetch templates"})
		return
	}

	response := GrafanaTemplatesResponse{
		Templates: make([]GrafanaTemplateResponse, 0, len(configs)),
	}

	// Sort template names for consistent ordering
	templateNames := make([]string, 0, len(configs))
	for name := range configs {
		templateNames = append(templateNames, name)
	}
	slices.Sort(templateNames)

	for _, name := range templateNames {
		cfg := configs[name]
		datasources := make([]GrafanaDatasourceResponse, 0, len(cfg.DataSources))

		// Sort datasource names for consistent ordering
		dsNames := make([]string, 0, len(cfg.DataSources))
		for dsName := range cfg.DataSources {
			dsNames = append(dsNames, dsName)
		}
		slices.Sort(dsNames)

		for _, dsName := range dsNames {
			datasources = append(datasources, GrafanaDatasourceResponse{
				Name: dsName,
				URL:  cfg.DataSources[dsName],
			})
		}
		response.Templates = append(response.Templates, GrafanaTemplateResponse{
			Name:        name,
			Template:    cfg.TemplateString,
			Datasources: datasources,
		})
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(response); err != nil {
		log.Error().Ctx(ctx).Err(err).Msg("Failed to encode grafana templates response")
	}
}

func handleCreateTemplate(ctx context.Context, w http.ResponseWriter, r *http.Request) {
	var req CreateTemplateRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(ErrorResponse{Error: "Invalid request body"})
		return
	}

	if req.Name == "" {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(ErrorResponse{Error: "Template name is required"})
		return
	}

	if err := db.AddGrafanaTemplate(ctx, req.Name, req.Template); err != nil {
		log.Error().Ctx(ctx).Err(err).Str("template_name", req.Name).Msg("Failed to create grafana template")
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(ErrorResponse{Error: "Failed to create template"})
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(map[string]string{"status": "created"})
}

func handleTemplateOperations(ctx context.Context, w http.ResponseWriter, r *http.Request, templateName string) {
	switch r.Method {
	case http.MethodPut:
		var req UpdateTemplateRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusBadRequest)
			json.NewEncoder(w).Encode(ErrorResponse{Error: "Invalid request body"})
			return
		}

		if err := db.AddGrafanaTemplate(ctx, templateName, req.Template); err != nil {
			log.Error().Ctx(ctx).Err(err).Str("template_name", templateName).Msg("Failed to update grafana template")
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusInternalServerError)
			json.NewEncoder(w).Encode(ErrorResponse{Error: "Failed to update template"})
			return
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{"status": "updated"})

	case http.MethodDelete:
		if err := db.RemoveGrafanaTemplate(ctx, templateName); err != nil {
			log.Error().Ctx(ctx).Err(err).Str("template_name", templateName).Msg("Failed to delete grafana template")
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusInternalServerError)
			json.NewEncoder(w).Encode(ErrorResponse{Error: "Failed to delete template"})
			return
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{"status": "deleted"})

	default:
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

func handleDatasourceOperations(ctx context.Context, w http.ResponseWriter, r *http.Request, templateName, sourceName string) {
	switch r.Method {
	case http.MethodPut:
		var req SetDatasourceRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusBadRequest)
			json.NewEncoder(w).Encode(ErrorResponse{Error: "Invalid request body"})
			return
		}

		if req.URL == "" {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusBadRequest)
			json.NewEncoder(w).Encode(ErrorResponse{Error: "URL is required"})
			return
		}

		if err := db.SetGrafanaDatasource(ctx, templateName, sourceName, req.URL); err != nil {
			log.Error().Ctx(ctx).Err(err).
				Str("template_name", templateName).
				Str("source_name", sourceName).
				Msg("Failed to set grafana datasource")
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusInternalServerError)
			json.NewEncoder(w).Encode(ErrorResponse{Error: "Failed to set datasource"})
			return
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{"status": "updated"})

	case http.MethodDelete:
		// Use "-" as URL to delete, as per the existing db function convention
		if err := db.SetGrafanaDatasource(ctx, templateName, sourceName, "-"); err != nil {
			log.Error().Ctx(ctx).Err(err).
				Str("template_name", templateName).
				Str("source_name", sourceName).
				Msg("Failed to delete grafana datasource")
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusInternalServerError)
			json.NewEncoder(w).Encode(ErrorResponse{Error: "Failed to delete datasource"})
			return
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{"status": "deleted"})

	default:
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

func handleRenderTemplate(ctx context.Context, w http.ResponseWriter, r *http.Request, templateName string) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	configs, err := db.GetGrafanaConfigs(ctx)
	if err != nil {
		log.Error().Ctx(ctx).Err(err).Msg("Failed to fetch grafana configs")
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(ErrorResponse{Error: "Failed to fetch templates"})
		return
	}

	config, ok := configs[templateName]
	if !ok {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusNotFound)
		json.NewEncoder(w).Encode(ErrorResponse{Error: "Template not found"})
		return
	}

	rendered := grafana.FormatTemplate(config)

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(GrafanaRenderResponse{Rendered: rendered}); err != nil {
		log.Error().Ctx(ctx).Err(err).Str("template_name", templateName).Msg("Failed to encode render response")
	}
}
