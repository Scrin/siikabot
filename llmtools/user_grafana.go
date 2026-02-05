package llmtools

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/Scrin/siikabot/db"
	"github.com/Scrin/siikabot/openrouter"
	"github.com/rs/zerolog/log"
)

const maxDatasourcesPerUser = 20
const maxDatasourceNameLength = 50
const maxDatasourceDescriptionLength = 200

// UserGrafanaToolDefinition defines the user Grafana datasource tool
var UserGrafanaToolDefinition = openrouter.ToolDefinition{
	Type: "function",
	Function: openrouter.FunctionSchema{
		Name:        "user_grafana",
		Description: "Manage and query user-defined Grafana datasources. Users can add custom metrics they want the AI to be able to query (like temperature sensors, system stats, etc). Only available to users with Grafana authorization.",
		Parameters: json.RawMessage(`{
			"type": "object",
			"properties": {
				"action": {
					"type": "string",
					"enum": ["add", "delete", "clear_all", "list", "query"],
					"description": "The action to perform: 'add' to create/update a datasource, 'delete' to remove by ID, 'clear_all' to remove all, 'list' to show all datasources, 'query' to fetch current value"
				},
				"name": {
					"type": "string",
					"description": "Name of the datasource (required for 'add' and 'query'). Max 50 characters."
				},
				"description": {
					"type": "string",
					"description": "Human-readable description of what this datasource measures (required for 'add'). Max 200 characters."
				},
				"url": {
					"type": "string",
					"description": "Full Grafana query URL that returns JSON (required for 'add')"
				},
				"datasource_id": {
					"type": "integer",
					"description": "The ID of the datasource to delete (required for 'delete' action)"
				}
			},
			"required": ["action"]
		}`),
	},
	Handler:          handleUserGrafanaToolCall,
	ValidityDuration: 5 * time.Minute,
}

// grafanaResponse matches the Grafana API response structure
type grafanaResponse struct {
	Results []struct {
		Series []struct {
			Values [][]any `json:"values"`
		} `json:"series"`
	} `json:"results"`
}

func handleUserGrafanaToolCall(ctx context.Context, arguments string) (string, error) {
	var args struct {
		Action       string `json:"action"`
		Name         string `json:"name"`
		Description  string `json:"description"`
		URL          string `json:"url"`
		DatasourceID *int64 `json:"datasource_id"`
	}

	log.Debug().Ctx(ctx).Str("arguments", arguments).Msg("Received user_grafana tool call")

	if err := json.Unmarshal([]byte(arguments), &args); err != nil {
		log.Error().Ctx(ctx).Err(err).Str("arguments", arguments).Msg("Failed to parse user_grafana tool arguments")
		return "", fmt.Errorf("failed to parse arguments: %w", err)
	}

	// Get sender from context
	sender, ok := ctx.Value("sender").(string)
	if !ok || sender == "" {
		return "", errors.New("sender not found in context")
	}

	// Check Grafana authorization for all actions
	if !db.IsGrafanaAuthorized(ctx, sender) {
		return "", errors.New("user not authorized for Grafana access")
	}

	switch args.Action {
	case "add":
		return handleAddUserGrafanaDatasource(ctx, sender, args.Name, args.Description, args.URL)
	case "delete":
		return handleDeleteUserGrafanaDatasource(ctx, sender, args.DatasourceID)
	case "clear_all":
		return handleClearAllUserGrafanaDatasources(ctx, sender)
	case "list":
		return handleListUserGrafanaDatasources(ctx, sender)
	case "query":
		return handleQueryUserGrafanaDatasource(ctx, sender, args.Name)
	default:
		return "", fmt.Errorf("unknown action: %s", args.Action)
	}
}

func handleAddUserGrafanaDatasource(ctx context.Context, userID, name, description, url string) (string, error) {
	if name == "" {
		return "", errors.New("name is required for add action")
	}
	if description == "" {
		return "", errors.New("description is required for add action")
	}
	if url == "" {
		return "", errors.New("url is required for add action")
	}
	if len(name) > maxDatasourceNameLength {
		return "", fmt.Errorf("name exceeds maximum length of %d characters", maxDatasourceNameLength)
	}
	if len(description) > maxDatasourceDescriptionLength {
		return "", fmt.Errorf("description exceeds maximum length of %d characters", maxDatasourceDescriptionLength)
	}

	// Check datasource count limit
	existing, err := db.GetUserGrafanaDatasources(ctx, userID)
	if err != nil {
		return "", fmt.Errorf("failed to check existing datasources: %w", err)
	}

	// Check if updating existing or adding new
	isUpdate := false
	for _, ds := range existing {
		if ds.Name == name {
			isUpdate = true
			break
		}
	}

	if !isUpdate && len(existing) >= maxDatasourcesPerUser {
		return "", fmt.Errorf("maximum of %d datasources per user reached", maxDatasourcesPerUser)
	}

	err = db.SaveUserGrafanaDatasource(ctx, userID, name, description, url)
	if err != nil {
		log.Error().Ctx(ctx).Err(err).
			Str("user_id", userID).
			Str("name", name).
			Msg("Failed to save user grafana datasource")
		return "", fmt.Errorf("failed to save datasource: %w", err)
	}

	log.Info().Ctx(ctx).
		Str("user_id", userID).
		Str("name", name).
		Str("description", description).
		Bool("is_update", isUpdate).
		Msg("User grafana datasource saved")

	if isUpdate {
		return fmt.Sprintf("Datasource '%s' updated successfully.", name), nil
	}
	return fmt.Sprintf("Datasource '%s' added successfully.", name), nil
}

func handleDeleteUserGrafanaDatasource(ctx context.Context, userID string, datasourceID *int64) (string, error) {
	if datasourceID == nil {
		return "", errors.New("datasource_id is required for delete action")
	}

	err := db.DeleteUserGrafanaDatasource(ctx, userID, *datasourceID)
	if err != nil {
		log.Error().Ctx(ctx).Err(err).
			Str("user_id", userID).
			Int64("datasource_id", *datasourceID).
			Msg("Failed to delete user grafana datasource")
		return "", fmt.Errorf("failed to delete datasource: %w", err)
	}

	log.Info().Ctx(ctx).
		Str("user_id", userID).
		Int64("datasource_id", *datasourceID).
		Msg("User grafana datasource deleted")

	return "Datasource deleted successfully.", nil
}

func handleClearAllUserGrafanaDatasources(ctx context.Context, userID string) (string, error) {
	count, err := db.DeleteAllUserGrafanaDatasources(ctx, userID)
	if err != nil {
		log.Error().Ctx(ctx).Err(err).
			Str("user_id", userID).
			Msg("Failed to clear all user grafana datasources")
		return "", fmt.Errorf("failed to clear datasources: %w", err)
	}

	log.Info().Ctx(ctx).
		Str("user_id", userID).
		Int64("deleted_count", count).
		Msg("All user grafana datasources cleared")

	return fmt.Sprintf("All datasources cleared (%d datasources deleted).", count), nil
}

func handleListUserGrafanaDatasources(ctx context.Context, userID string) (string, error) {
	datasources, err := db.GetUserGrafanaDatasources(ctx, userID)
	if err != nil {
		return "", fmt.Errorf("failed to list datasources: %w", err)
	}

	if len(datasources) == 0 {
		return "No datasources configured. Use the 'add' action to create one.", nil
	}

	result := fmt.Sprintf("Your Grafana datasources (%d):\n", len(datasources))
	for _, ds := range datasources {
		result += fmt.Sprintf("- [ID: %d] %s: %s\n", ds.ID, ds.Name, ds.Description)
	}
	return result, nil
}

func handleQueryUserGrafanaDatasource(ctx context.Context, userID, name string) (string, error) {
	if name == "" {
		return "", errors.New("name is required for query action")
	}

	datasource, err := db.GetUserGrafanaDatasourceByName(ctx, userID, name)
	if err != nil {
		return "", fmt.Errorf("failed to get datasource: %w", err)
	}
	if datasource == nil {
		return "", fmt.Errorf("datasource '%s' not found", name)
	}

	value := queryGrafanaURL(ctx, datasource.URL)

	log.Debug().Ctx(ctx).
		Str("user_id", userID).
		Str("name", name).
		Str("value", value).
		Msg("User grafana datasource queried")

	return fmt.Sprintf("%s (%s): %s", datasource.Name, datasource.Description, value), nil
}

// queryGrafanaURL fetches a value from a Grafana query URL
func queryGrafanaURL(ctx context.Context, queryURL string) string {
	client := &http.Client{Timeout: 10 * time.Second}
	req, err := http.NewRequestWithContext(ctx, "GET", queryURL, nil)
	if err != nil {
		log.Error().Ctx(ctx).Err(err).Str("url", queryURL).Msg("Failed to create grafana request")
		return "Error: " + err.Error()
	}

	resp, err := client.Do(req)
	if err != nil {
		log.Error().Ctx(ctx).Err(err).Str("url", queryURL).Msg("Failed to query grafana")
		return "Error: " + err.Error()
	}
	defer resp.Body.Close()

	var grafanaResp grafanaResponse
	if err = json.NewDecoder(resp.Body).Decode(&grafanaResp); err != nil {
		log.Error().Ctx(ctx).Err(err).Str("url", queryURL).Msg("Failed to decode grafana response")
		return "Error: " + err.Error()
	}

	if len(grafanaResp.Results) < 1 || len(grafanaResp.Results[0].Series) < 1 {
		return "N/A"
	}

	values := grafanaResp.Results[0].Series[0].Values
	if len(values) < 1 || len(values[0]) < 2 {
		return "N/A"
	}

	if floatValue, ok := values[0][1].(float64); ok {
		return strconv.FormatFloat(floatValue, 'f', 2, 64)
	}
	if strValue, ok := values[0][1].(string); ok {
		return strValue
	}
	return "N/A"
}
