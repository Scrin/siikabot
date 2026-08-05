package aigateway

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/Scrin/siikabot/config"
	"github.com/Scrin/siikabot/metrics"
	"github.com/rs/zerolog/log"
)

// logPollInterval is how often gateway logs are polled for cost and token stats
const logPollInterval = 1 * time.Minute

// logPollPageSize is how many log entries to fetch per request
const logPollPageSize = 50

// logPollMaxPages bounds a single poll, so a burst can't spin forever
const logPollMaxPages = 10

// GatewayLog represents a single entry from the AI Gateway logs API.
// Note that ID is a content hash rather than a sortable identifier, so log ordering
// and the poll cursor are both based on CreatedAt.
type GatewayLog struct {
	ID         string    `json:"id"`
	CreatedAt  time.Time `json:"created_at"`
	Model      string    `json:"model"`
	Provider   string    `json:"provider"`
	Cost       *float64  `json:"cost"`
	TokensIn   int       `json:"tokens_in"`
	TokensOut  int       `json:"tokens_out"`
	Duration   int       `json:"duration"`
	Cached     bool      `json:"cached"`
	Success    bool      `json:"success"`
	StatusCode int       `json:"status_code"`
}

// logsResponse is the envelope the logs API returns
type logsResponse struct {
	Success bool         `json:"success"`
	Errors  []apiError   `json:"errors"`
	Result  []GatewayLog `json:"result"`
}

// StartLogPoller starts a background goroutine that polls the AI Gateway logs and records
// cost, token, latency and cache metrics from them.
//
// The gateway does not return a per-request log id on the REST API, so metrics cannot be
// attached to individual requests. That is not a problem: every metric here is an aggregate
// keyed by model and provider. Reading the gateway's own log records also keeps the numbers
// correct even when a provider's response body reports usage in a non-standard shape.
//
// Requires the API token to carry the AI Gateway Read permission in addition to inference
// access, otherwise every poll fails with HTTP 403.
func StartLogPoller(ctx context.Context) {
	go func() {
		ticker := time.NewTicker(logPollInterval)
		defer ticker.Stop()

		// Skip everything that already exists, so a restart doesn't replay history into the counters
		cursor := time.Now()
		seenAtCursor := make(map[string]bool)

		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				cursor, seenAtCursor = pollLogs(ctx, cursor, seenAtCursor)
			}
		}
	}()
}

// pollLogs fetches all logs newer than the cursor and records their metrics. It returns the
// updated cursor and the set of log ids seen at exactly that timestamp, which is needed to
// avoid recording an entry twice when several logs share a created_at value.
func pollLogs(ctx context.Context, cursor time.Time, seenAtCursor map[string]bool) (time.Time, map[string]bool) {
	fresh, err := collectFreshLogs(ctx, cursor, seenAtCursor)
	if err != nil {
		// Already logged in fetchLogs, and stats are best-effort: keep the old cursor so the
		// next tick retries the same window rather than losing entries
		return cursor, seenAtCursor
	}
	if len(fresh) == 0 {
		return cursor, seenAtCursor
	}

	// The API returns logs newest first. Record oldest first so that the "latest latency"
	// gauge ends up holding the newest entry rather than the oldest of the batch.
	for i := len(fresh) - 1; i >= 0; i-- {
		recordLog(ctx, fresh[i])
	}

	// fresh[0] is the newest entry, so it becomes the new cursor. Remember every id sharing
	// that exact timestamp, since created_at is not unique and log ids are not sortable.
	newCursor := fresh[0].CreatedAt
	newSeen := make(map[string]bool)
	for _, entry := range fresh {
		if entry.CreatedAt.Equal(newCursor) {
			newSeen[entry.ID] = true
		}
	}

	log.Debug().Ctx(ctx).
		Int("recorded_count", len(fresh)).
		Time("cursor", newCursor).
		Msg("Recorded gateway log stats")

	return newCursor, newSeen
}

// collectFreshLogs pages through the gateway logs, newest first, gathering every entry newer
// than the cursor. Entries already recorded at the cursor timestamp are skipped.
func collectFreshLogs(ctx context.Context, cursor time.Time, seenAtCursor map[string]bool) ([]GatewayLog, error) {
	var fresh []GatewayLog

	for page := 1; page <= logPollMaxPages; page++ {
		logs, err := fetchLogs(ctx, page)
		if err != nil {
			return nil, err
		}

		for _, entry := range logs {
			if entry.CreatedAt.Before(cursor) {
				// Ordered newest first, so everything from here on is older still
				return fresh, nil
			}
			if entry.CreatedAt.Equal(cursor) && seenAtCursor[entry.ID] {
				continue
			}
			fresh = append(fresh, entry)
		}

		// A partial page means there is nothing older left to fetch
		if len(logs) < logPollPageSize {
			return fresh, nil
		}
	}

	log.Warn().Ctx(ctx).
		Int("max_pages", logPollMaxPages).
		Int("page_size", logPollPageSize).
		Msg("Gateway log poll hit the page limit, some logs were not processed")

	return fresh, nil
}

// fetchLogs fetches a single page of gateway logs, newest first
func fetchLogs(ctx context.Context, page int) ([]GatewayLog, error) {
	url := fmt.Sprintf("%s?page=%d&per_page=%d&order_by=created_at&order_by_direction=desc",
		endpoint(fmt.Sprintf("/ai-gateway/gateways/%s/logs", config.CloudflareAIGatewayID)), page, logPollPageSize)

	httpReq, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		log.Error().Ctx(ctx).Err(err).Int("page", page).Msg("Failed to create gateway logs request")
		return nil, fmt.Errorf("failed to create gateway logs request: %w", err)
	}
	setAuthHeaders(httpReq)

	resp, err := httpClient.Do(httpReq)
	if err != nil {
		log.Error().Ctx(ctx).Err(err).Int("page", page).Msg("Failed to fetch gateway logs")
		return nil, fmt.Errorf("failed to fetch gateway logs: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		log.Error().Ctx(ctx).Err(err).Int("page", page).Msg("Failed to read gateway logs response")
		return nil, fmt.Errorf("failed to read gateway logs response: %w", err)
	}

	var logsResp logsResponse
	if err := json.Unmarshal(body, &logsResp); err != nil {
		log.Error().Ctx(ctx).Err(err).
			Int("page", page).
			Int("status_code", resp.StatusCode).
			Str("response", string(body)).
			Msg("Failed to parse gateway logs response")
		return nil, fmt.Errorf("failed to parse gateway logs response: %w", err)
	}

	if resp.StatusCode >= 400 || !logsResp.Success || len(logsResp.Errors) > 0 {
		errorCode, errorMessage := firstError(logsResp.Errors)
		log.Error().Ctx(ctx).
			Int("page", page).
			Int("status_code", resp.StatusCode).
			Int("error_code", errorCode).
			Str("error_message", errorMessage).
			Msg("Gateway logs API returned error, check that the API token has the AI Gateway Read permission")
		return nil, fmt.Errorf("gateway logs API error: %s", errorMessage)
	}

	return logsResp.Result, nil
}

// recordLog records the metrics of a single gateway log entry
func recordLog(ctx context.Context, entry GatewayLog) {
	metrics.RecordAIGatewayCacheStatus(entry.Model, entry.Provider, entry.Cached)

	// Duration is reported in milliseconds
	if entry.Duration > 0 {
		metrics.RecordAIGatewayLatency(entry.Model, entry.Provider, float64(entry.Duration)/1000.0)
	}

	if !entry.Success {
		metrics.RecordAIGatewayFailure(entry.Model, entry.Provider, entry.StatusCode)
		return
	}

	if entry.Cost != nil {
		metrics.RecordAIGatewayCost(entry.Model, entry.Provider, *entry.Cost)
	}
	metrics.RecordAIGatewayTokens(entry.Model, entry.Provider, entry.TokensIn, entry.TokensOut)

	log.Trace().Ctx(ctx).
		Str("log_id", entry.ID).
		Str("model", entry.Model).
		Str("provider", entry.Provider).
		Int("tokens_in", entry.TokensIn).
		Int("tokens_out", entry.TokensOut).
		Int("duration_ms", entry.Duration).
		Bool("cached", entry.Cached).
		Msg("Recorded gateway log entry")
}
