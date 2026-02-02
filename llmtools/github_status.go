package llmtools

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/Scrin/siikabot/openrouter"
	"github.com/rs/zerolog/log"
)

// GitHubStatusToolDefinition returns the tool definition for the GitHub status tool
var GitHubStatusToolDefinition = openrouter.ToolDefinition{
	Type: "function",
	Function: openrouter.FunctionSchema{
		Name:        "github_status",
		Description: "Query GitHub's current operational status and recent incidents. Use this to check if GitHub is experiencing any issues or outages.",
		Parameters: json.RawMessage(`{
			"type": "object",
			"properties": {
				"include_past_incidents": {
					"type": "boolean",
					"description": "Whether to include resolved past incidents (default: false, only shows current/unresolved issues)"
				}
			},
			"required": []
		}`),
	},
	Handler:          handleGitHubStatusToolCall,
	ValidityDuration: 5 * time.Minute,
}

// GitHub Status API response types
type gitHubStatusSummary struct {
	Page       gitHubStatusPage        `json:"page"`
	Status     gitHubStatusIndicator   `json:"status"`
	Components []gitHubStatusComponent `json:"components"`
	Incidents  []gitHubStatusIncident  `json:"incidents"`
}

type gitHubStatusPage struct {
	Name      string `json:"name"`
	URL       string `json:"url"`
	UpdatedAt string `json:"updated_at"`
}

type gitHubStatusIndicator struct {
	Indicator   string `json:"indicator"`
	Description string `json:"description"`
}

type gitHubStatusComponent struct {
	Name   string `json:"name"`
	Status string `json:"status"`
}

type gitHubStatusIncident struct {
	Name            string                       `json:"name"`
	Status          string                       `json:"status"`
	Impact          string                       `json:"impact"`
	CreatedAt       string                       `json:"created_at"`
	UpdatedAt       string                       `json:"updated_at"`
	Shortlink       string                       `json:"shortlink"`
	IncidentUpdates []gitHubStatusIncidentUpdate `json:"incident_updates"`
}

type gitHubStatusIncidentUpdate struct {
	Status    string `json:"status"`
	Body      string `json:"body"`
	CreatedAt string `json:"created_at"`
}

// handleGitHubStatusToolCall handles GitHub status tool calls
func handleGitHubStatusToolCall(ctx context.Context, arguments string) (string, error) {
	var args struct {
		IncludePastIncidents bool `json:"include_past_incidents"`
	}

	log.Debug().Ctx(ctx).Str("arguments", arguments).Msg("Received github_status tool call")

	if arguments != "" && arguments != "{}" {
		if err := json.Unmarshal([]byte(arguments), &args); err != nil {
			log.Error().Ctx(ctx).Err(err).Str("arguments", arguments).Msg("Failed to parse github_status tool arguments")
			return "", fmt.Errorf("failed to parse arguments: %w", err)
		}
	}

	// Fetch current status summary
	summary, err := fetchGitHubStatusSummary(ctx)
	if err != nil {
		return "", err
	}

	// Fetch past incidents if requested
	var pastIncidents []gitHubStatusIncident
	if args.IncludePastIncidents {
		pastIncidents, err = fetchGitHubStatusIncidents(ctx)
		if err != nil {
			log.Warn().Ctx(ctx).Err(err).Msg("Failed to fetch past incidents, continuing with summary only")
		}
	}

	return formatGitHubStatus(summary, pastIncidents, args.IncludePastIncidents), nil
}

// fetchGitHubStatusSummary fetches the current GitHub status summary
func fetchGitHubStatusSummary(ctx context.Context) (*gitHubStatusSummary, error) {
	apiURL := "https://www.githubstatus.com/api/v2/summary.json"

	log.Debug().Ctx(ctx).Str("url", apiURL).Msg("Fetching GitHub status summary")

	req, err := http.NewRequestWithContext(ctx, "GET", apiURL, nil)
	if err != nil {
		log.Error().Ctx(ctx).Err(err).Str("url", apiURL).Msg("Failed to create GitHub status API request")
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("User-Agent", "Siikabot-GitHubStatus-Tool")

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		log.Error().Ctx(ctx).Err(err).Str("url", apiURL).Msg("Failed to fetch GitHub status")
		return nil, fmt.Errorf("failed to fetch GitHub status: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		log.Error().Ctx(ctx).Err(err).Str("url", apiURL).Msg("Failed to read GitHub status response")
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		log.Error().Ctx(ctx).Int("status_code", resp.StatusCode).Str("url", apiURL).Str("response", string(body)).Msg("GitHub status API returned non-OK status")
		return nil, fmt.Errorf("GitHub status API returned status code %d", resp.StatusCode)
	}

	var summary gitHubStatusSummary
	if err := json.Unmarshal(body, &summary); err != nil {
		log.Error().Ctx(ctx).Err(err).Str("url", apiURL).Str("response", string(body)).Msg("Failed to parse GitHub status response")
		return nil, fmt.Errorf("failed to parse GitHub status response: %w", err)
	}

	log.Debug().Ctx(ctx).Str("indicator", summary.Status.Indicator).Int("component_count", len(summary.Components)).Int("incident_count", len(summary.Incidents)).Msg("GitHub status fetched successfully")

	return &summary, nil
}

// fetchGitHubStatusIncidents fetches recent GitHub incidents
func fetchGitHubStatusIncidents(ctx context.Context) ([]gitHubStatusIncident, error) {
	apiURL := "https://www.githubstatus.com/api/v2/incidents.json"

	log.Debug().Ctx(ctx).Str("url", apiURL).Msg("Fetching GitHub incidents")

	req, err := http.NewRequestWithContext(ctx, "GET", apiURL, nil)
	if err != nil {
		log.Error().Ctx(ctx).Err(err).Str("url", apiURL).Msg("Failed to create GitHub incidents API request")
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("User-Agent", "Siikabot-GitHubStatus-Tool")

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		log.Error().Ctx(ctx).Err(err).Str("url", apiURL).Msg("Failed to fetch GitHub incidents")
		return nil, fmt.Errorf("failed to fetch GitHub incidents: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		log.Error().Ctx(ctx).Err(err).Str("url", apiURL).Msg("Failed to read GitHub incidents response")
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		log.Error().Ctx(ctx).Int("status_code", resp.StatusCode).Str("url", apiURL).Str("response", string(body)).Msg("GitHub incidents API returned non-OK status")
		return nil, fmt.Errorf("GitHub incidents API returned status code %d", resp.StatusCode)
	}

	var result struct {
		Incidents []gitHubStatusIncident `json:"incidents"`
	}
	if err := json.Unmarshal(body, &result); err != nil {
		log.Error().Ctx(ctx).Err(err).Str("url", apiURL).Str("response", string(body)).Msg("Failed to parse GitHub incidents response")
		return nil, fmt.Errorf("failed to parse GitHub incidents response: %w", err)
	}

	log.Debug().Ctx(ctx).Int("incident_count", len(result.Incidents)).Msg("GitHub incidents fetched successfully")

	return result.Incidents, nil
}

// formatGitHubStatus formats the GitHub status data into a readable string
func formatGitHubStatus(summary *gitHubStatusSummary, pastIncidents []gitHubStatusIncident, includePast bool) string {
	var sb strings.Builder

	// Overall status
	sb.WriteString("## GitHub Status\n\n")
	sb.WriteString(fmt.Sprintf("**Overall Status:** %s\n", summary.Status.Description))
	sb.WriteString(fmt.Sprintf("**Indicator:** %s\n", strings.ToUpper(summary.Status.Indicator)))

	// Component statuses - show all or just non-operational
	sb.WriteString("\n### Components\n")
	hasIssues := false
	for _, comp := range summary.Components {
		if comp.Status != "operational" {
			hasIssues = true
			sb.WriteString(fmt.Sprintf("- **%s:** %s\n", comp.Name, formatComponentStatus(comp.Status)))
		}
	}
	if !hasIssues {
		sb.WriteString("All components operational.\n")
	}

	// Active incidents
	if len(summary.Incidents) > 0 {
		sb.WriteString("\n### Active Incidents\n")
		for _, incident := range summary.Incidents {
			formatIncident(&sb, incident)
		}
	} else {
		sb.WriteString("\n### Active Incidents\n")
		sb.WriteString("No active incidents.\n")
	}

	// Past incidents if requested
	if includePast && len(pastIncidents) > 0 {
		sb.WriteString("\n### Recent Past Incidents\n")
		// Show up to 5 past resolved incidents
		count := 0
		for _, incident := range pastIncidents {
			if incident.Status == "resolved" || incident.Status == "postmortem" {
				formatIncident(&sb, incident)
				count++
				if count >= 5 {
					break
				}
			}
		}
		if count == 0 {
			sb.WriteString("No recent resolved incidents.\n")
		}
	}

	return sb.String()
}

// formatComponentStatus converts component status to readable format
func formatComponentStatus(status string) string {
	switch status {
	case "operational":
		return "Operational"
	case "degraded_performance":
		return "Degraded Performance"
	case "partial_outage":
		return "Partial Outage"
	case "major_outage":
		return "Major Outage"
	default:
		return status
	}
}

// formatIncident formats a single incident
func formatIncident(sb *strings.Builder, incident gitHubStatusIncident) {
	sb.WriteString(fmt.Sprintf("\n#### %s\n", incident.Name))
	sb.WriteString(fmt.Sprintf("**Status:** %s | **Impact:** %s\n", strings.Title(incident.Status), strings.Title(incident.Impact)))
	sb.WriteString(fmt.Sprintf("**Link:** %s\n", incident.Shortlink))

	// Show the most recent update
	if len(incident.IncidentUpdates) > 0 {
		latestUpdate := incident.IncidentUpdates[0]
		updateBody := latestUpdate.Body
		if len(updateBody) > 300 {
			updateBody = updateBody[:297] + "..."
		}
		sb.WriteString(fmt.Sprintf("**Latest Update (%s):** %s\n", latestUpdate.CreatedAt, updateBody))
	}
}
