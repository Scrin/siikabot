package api

import (
	"net/http"
	"strconv"
	"time"

	"github.com/Scrin/siikabot/db"
	"github.com/Scrin/siikabot/matrix"
	"github.com/gin-gonic/gin"
	"github.com/rs/zerolog/log"
)

// defaultUsageDays is the reporting period used when the request does not ask for one
const defaultUsageDays = 7

// maxUsageDays bounds the reporting period, since usage records are pruned beyond this anyway
const maxUsageDays = 90

// ChatUsageEntry is one room and model's usage over the reporting period
type ChatUsageEntry struct {
	RoomID             string  `json:"room_id"`
	RoomName           string  `json:"room_name"`
	Model              string  `json:"model"`
	Turns              int     `json:"turns"`
	PromptTokens       int     `json:"prompt_tokens"`
	CompletionTokens   int     `json:"completion_tokens"`
	CachedPromptTokens int     `json:"cached_prompt_tokens"`
	CacheHitRate       float64 `json:"cache_hit_rate"`
	ToolIterations     int     `json:"tool_iterations"`
	Failures           int     `json:"failures"`
	// Silent are the turns that ended without an answer because the message needed none
	Silent int `json:"silent"`
	// FailuresByOutcome breaks the failures down by how the turns failed, such as turn_timeout or
	// tool_limit, which says which of the chat settings a turn ran into
	FailuresByOutcome map[string]int `json:"failures_by_outcome"`
}

// ChatUsageResponse is the chat usage report
type ChatUsageResponse struct {
	Since   time.Time        `json:"since"`
	Days    int              `json:"days"`
	Entries []ChatUsageEntry `json:"entries"`
	Totals  ChatUsageEntry   `json:"totals"`
}

// ChatUsageHandler returns per-room chat usage (admin only)
// GET /api/admin/chat-usage?days=7
//
// This lives behind admin auth rather than on the metrics endpoint because it is keyed by room and
// user, and the metrics endpoint is served without authentication.
func ChatUsageHandler(c *gin.Context) {
	ctx := c.Request.Context()

	days := defaultUsageDays
	if raw := c.Query("days"); raw != "" {
		parsed, err := strconv.Atoi(raw)
		if err != nil || parsed <= 0 || parsed > maxUsageDays {
			c.JSON(http.StatusBadRequest, ErrorResponse{Error: "days must be between 1 and " + strconv.Itoa(maxUsageDays)})
			return
		}
		days = parsed
	}

	since := time.Now().AddDate(0, 0, -days)

	summaries, err := db.GetChatUsageSummary(ctx, since)
	if err != nil {
		log.Error().Ctx(ctx).Err(err).Int("days", days).Msg("Failed to fetch chat usage")
		c.JSON(http.StatusInternalServerError, ErrorResponse{Error: "Failed to fetch chat usage"})
		return
	}
	failures, err := db.GetChatUsageFailures(ctx, since)
	if err != nil {
		log.Error().Ctx(ctx).Err(err).Int("days", days).Msg("Failed to fetch chat usage failures")
		c.JSON(http.StatusInternalServerError, ErrorResponse{Error: "Failed to fetch chat usage"})
		return
	}

	entries, totals := usageEntries(summaries, failures, func(roomID string) string {
		return matrix.GetRoomName(ctx, roomID)
	})

	c.JSON(http.StatusOK, ChatUsageResponse{
		Since:   since,
		Days:    days,
		Entries: entries,
		Totals:  totals,
	})
}

// usageEntries turns the usage summaries into the report's entries, one per room and model, with how
// their turns failed, and adds them up into its totals
func usageEntries(summaries []db.ChatUsageSummary, failures []db.ChatUsageFailures, roomName func(roomID string) string) ([]ChatUsageEntry, ChatUsageEntry) {
	type roomModel struct{ roomID, model string }
	outcomes := make(map[roomModel]map[string]int)
	for _, failure := range failures {
		key := roomModel{failure.RoomID, failure.Model}
		if outcomes[key] == nil {
			outcomes[key] = make(map[string]int)
		}
		outcomes[key][failure.Outcome] += failure.Turns
	}

	entries := make([]ChatUsageEntry, 0, len(summaries))
	totals := ChatUsageEntry{Model: "all", FailuresByOutcome: make(map[string]int)}
	for _, summary := range summaries {
		byOutcome := outcomes[roomModel{summary.RoomID, summary.Model}]
		if byOutcome == nil {
			byOutcome = make(map[string]int)
		}
		for outcome, turns := range byOutcome {
			totals.FailuresByOutcome[outcome] += turns
		}

		entries = append(entries, ChatUsageEntry{
			RoomID:             summary.RoomID,
			RoomName:           roomName(summary.RoomID),
			Model:              summary.Model,
			Turns:              summary.Turns,
			PromptTokens:       summary.PromptTokens,
			CompletionTokens:   summary.CompletionTokens,
			CachedPromptTokens: summary.CachedPromptTokens,
			CacheHitRate:       cacheHitRate(summary.CachedPromptTokens, summary.PromptTokens),
			ToolIterations:     summary.ToolIterations,
			Failures:           summary.Failures,
			Silent:             summary.Silent,
			FailuresByOutcome:  byOutcome,
		})

		totals.Turns += summary.Turns
		totals.PromptTokens += summary.PromptTokens
		totals.CompletionTokens += summary.CompletionTokens
		totals.CachedPromptTokens += summary.CachedPromptTokens
		totals.ToolIterations += summary.ToolIterations
		totals.Failures += summary.Failures
		totals.Silent += summary.Silent
	}
	totals.CacheHitRate = cacheHitRate(totals.CachedPromptTokens, totals.PromptTokens)
	return entries, totals
}

// cacheHitRate returns the share of prompt tokens served from the provider's cache
func cacheHitRate(cachedTokens, promptTokens int) float64 {
	if promptTokens <= 0 {
		return 0
	}
	return float64(cachedTokens) / float64(promptTokens)
}
