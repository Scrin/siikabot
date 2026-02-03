package api

import (
	"net/http"
	"slices"

	"github.com/Scrin/siikabot/commands/grafana"
	"github.com/Scrin/siikabot/db"
	"github.com/gin-gonic/gin"
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

// GrafanaAuthMiddleware checks if the user has Grafana authorization
func GrafanaAuthMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		ctx := c.Request.Context()

		userID, ok := GetUserIDFromContext(c)
		if !ok {
			c.AbortWithStatusJSON(http.StatusUnauthorized, ErrorResponse{Error: "Not authenticated"})
			return
		}

		if !db.IsGrafanaAuthorized(ctx, userID) {
			c.AbortWithStatusJSON(http.StatusForbidden, ErrorResponse{Error: "Grafana access not authorized"})
			return
		}

		c.Next()
	}
}

// ListGrafanaTemplatesHandler handles GET /api/grafana/templates
func ListGrafanaTemplatesHandler(c *gin.Context) {
	ctx := c.Request.Context()

	configs, err := db.GetGrafanaConfigs(ctx)
	if err != nil {
		log.Error().Ctx(ctx).Err(err).Msg("Failed to fetch grafana configs")
		c.JSON(http.StatusInternalServerError, ErrorResponse{Error: "Failed to fetch templates"})
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

	c.JSON(http.StatusOK, response)
}

// CreateGrafanaTemplateHandler handles POST /api/grafana/templates
func CreateGrafanaTemplateHandler(c *gin.Context) {
	ctx := c.Request.Context()

	var req CreateTemplateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{Error: "Invalid request body"})
		return
	}

	if req.Name == "" {
		c.JSON(http.StatusBadRequest, ErrorResponse{Error: "Template name is required"})
		return
	}

	if err := db.AddGrafanaTemplate(ctx, req.Name, req.Template); err != nil {
		log.Error().Ctx(ctx).Err(err).Str("template_name", req.Name).Msg("Failed to create grafana template")
		c.JSON(http.StatusInternalServerError, ErrorResponse{Error: "Failed to create template"})
		return
	}

	c.JSON(http.StatusCreated, gin.H{"status": "created"})
}

// UpdateGrafanaTemplateHandler handles PUT /api/grafana/templates/:name
func UpdateGrafanaTemplateHandler(c *gin.Context) {
	ctx := c.Request.Context()
	templateName := c.Param("name")

	var req UpdateTemplateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{Error: "Invalid request body"})
		return
	}

	if err := db.AddGrafanaTemplate(ctx, templateName, req.Template); err != nil {
		log.Error().Ctx(ctx).Err(err).Str("template_name", templateName).Msg("Failed to update grafana template")
		c.JSON(http.StatusInternalServerError, ErrorResponse{Error: "Failed to update template"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"status": "updated"})
}

// DeleteGrafanaTemplateHandler handles DELETE /api/grafana/templates/:name
func DeleteGrafanaTemplateHandler(c *gin.Context) {
	ctx := c.Request.Context()
	templateName := c.Param("name")

	if err := db.RemoveGrafanaTemplate(ctx, templateName); err != nil {
		log.Error().Ctx(ctx).Err(err).Str("template_name", templateName).Msg("Failed to delete grafana template")
		c.JSON(http.StatusInternalServerError, ErrorResponse{Error: "Failed to delete template"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"status": "deleted"})
}

// RenderGrafanaTemplateHandler handles GET /api/grafana/templates/:name/render
func RenderGrafanaTemplateHandler(c *gin.Context) {
	ctx := c.Request.Context()
	templateName := c.Param("name")

	configs, err := db.GetGrafanaConfigs(ctx)
	if err != nil {
		log.Error().Ctx(ctx).Err(err).Msg("Failed to fetch grafana configs")
		c.JSON(http.StatusInternalServerError, ErrorResponse{Error: "Failed to fetch templates"})
		return
	}

	config, ok := configs[templateName]
	if !ok {
		c.JSON(http.StatusNotFound, ErrorResponse{Error: "Template not found"})
		return
	}

	rendered := grafana.FormatTemplate(config)
	c.JSON(http.StatusOK, GrafanaRenderResponse{Rendered: rendered})
}

// SetGrafanaDatasourceHandler handles PUT /api/grafana/templates/:name/datasources/:sourceName
func SetGrafanaDatasourceHandler(c *gin.Context) {
	ctx := c.Request.Context()
	templateName := c.Param("name")
	sourceName := c.Param("sourceName")

	var req SetDatasourceRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{Error: "Invalid request body"})
		return
	}

	if req.URL == "" {
		c.JSON(http.StatusBadRequest, ErrorResponse{Error: "URL is required"})
		return
	}

	if err := db.SetGrafanaDatasource(ctx, templateName, sourceName, req.URL); err != nil {
		log.Error().Ctx(ctx).Err(err).
			Str("template_name", templateName).
			Str("source_name", sourceName).
			Msg("Failed to set grafana datasource")
		c.JSON(http.StatusInternalServerError, ErrorResponse{Error: "Failed to set datasource"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"status": "updated"})
}

// DeleteGrafanaDatasourceHandler handles DELETE /api/grafana/templates/:name/datasources/:sourceName
func DeleteGrafanaDatasourceHandler(c *gin.Context) {
	ctx := c.Request.Context()
	templateName := c.Param("name")
	sourceName := c.Param("sourceName")

	// Use "-" as URL to delete, as per the existing db function convention
	if err := db.SetGrafanaDatasource(ctx, templateName, sourceName, "-"); err != nil {
		log.Error().Ctx(ctx).Err(err).
			Str("template_name", templateName).
			Str("source_name", sourceName).
			Msg("Failed to delete grafana datasource")
		c.JSON(http.StatusInternalServerError, ErrorResponse{Error: "Failed to delete datasource"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"status": "deleted"})
}
