package api

import (
	"net/http"
	"time"

	"github.com/Scrin/siikabot/commands/chat"
	"github.com/Scrin/siikabot/db"
	"github.com/gin-gonic/gin"
	"github.com/rs/zerolog/log"
)

// chatConfigHistoryLength is how many of the latest configurations the response lists
const chatConfigHistoryLength = 20

// ChatSettingEntry is one chat setting as it stands in the configuration in effect
type ChatSettingEntry struct {
	Key   string `json:"key"`
	Group string `json:"group"`
	// Value is without its unit, which Unit gives
	Value   string `json:"value"`
	Unit    string `json:"unit"`
	Summary string `json:"summary"`
	// Command changes the setting in a room, with an example value
	Command string `json:"command"`
}

// ChatSettingChange is a change of one setting from one configuration to the next
type ChatSettingChange struct {
	Key  string `json:"key"`
	From string `json:"from"`
	To   string `json:"to"`
	Unit string `json:"unit"`
}

// ChatConfigVersion is one configuration on record, with what it changed from the one before it
type ChatConfigVersion struct {
	ID        int64               `json:"id"`
	CreatedAt time.Time           `json:"created_at"`
	Changes   []ChatSettingChange `json:"changes"`
	// First says this was the first configuration, so there was nothing for it to change
	First bool `json:"first"`
}

// ChatConfigResponse is the chat configuration in effect, and the latest configurations on record
type ChatConfigResponse struct {
	ID        int64              `json:"id"`
	CreatedAt time.Time          `json:"created_at"`
	Settings  []ChatSettingEntry `json:"settings"`
	// History lists the latest configurations, newest first, the one in effect included
	History []ChatConfigVersion `json:"history"`
}

// ChatConfigHandler returns the chat configuration in effect and the latest changes to it (admin
// only)
// GET /api/admin/chat-config
//
// Read-only: the configuration is changed with !chat set in a room, which answers there, so a change
// is never made where nobody sees it.
func ChatConfigHandler(c *gin.Context) {
	ctx := c.Request.Context()

	history, err := db.GetChatConfigHistory(ctx)
	if err != nil {
		log.Error().Ctx(ctx).Err(err).Msg("Failed to fetch chat config")
		c.JSON(http.StatusInternalServerError, ErrorResponse{Error: "Failed to fetch chat configuration"})
		return
	}

	c.JSON(http.StatusOK, chatConfigResponse(history))
}

// chatConfigResponse builds the response from every configuration on record, newest first, of which
// there is always at least one
func chatConfigResponse(history []db.ChatConfig) ChatConfigResponse {
	current := history[0]

	infos := chat.Settings(current)
	settings := make([]ChatSettingEntry, 0, len(infos))
	for _, info := range infos {
		settings = append(settings, ChatSettingEntry{
			Key:     info.Key,
			Group:   info.Group,
			Value:   info.Value,
			Unit:    info.Unit,
			Summary: info.Summary,
			Command: info.Command,
		})
	}

	versions := make([]ChatConfigVersion, 0, min(len(history), chatConfigHistoryLength))
	for i, cfg := range history[:min(len(history), chatConfigHistoryLength)] {
		version := ChatConfigVersion{ID: cfg.ID, CreatedAt: cfg.CreatedAt, Changes: []ChatSettingChange{}}
		if i+1 == len(history) {
			version.First = true
		} else {
			for _, change := range chat.Changes(history[i+1], cfg) {
				version.Changes = append(version.Changes, ChatSettingChange{
					Key:  change.Key,
					From: change.From,
					To:   change.To,
					Unit: change.Unit,
				})
			}
		}
		versions = append(versions, version)
	}

	return ChatConfigResponse{
		ID:        current.ID,
		CreatedAt: current.CreatedAt,
		Settings:  settings,
		History:   versions,
	}
}
