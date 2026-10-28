package api

import (
	"context"
	"errors"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/Scrin/siikabot/config"
	"github.com/Scrin/siikabot/db"
	"github.com/Scrin/siikabot/matrix"
	"github.com/Scrin/siikabot/webhooks"
	"github.com/gin-gonic/gin"
	"github.com/rs/zerolog/log"
)

// WebhookResponse is a webhook as the dashboard shows it. Its token is never part of it: only the
// token's hash is stored.
type WebhookResponse struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	RoomID   string `json:"room_id"`
	RoomName string `json:"room_name,omitempty"`
	// BotInRoom is false once the bot has left the webhook's room, and the webhook can't post there
	BotInRoom bool `json:"bot_in_room"`
	// URL is where the webhook's messages are sent
	URL       string `json:"url"`
	CreatedAt string `json:"created_at"`
	// LastUsedAt is when a message was last delivered, nil if none has been
	LastUsedAt *string `json:"last_used_at"`
}

// WebhooksResponse is the response for the webhooks endpoint
type WebhooksResponse struct {
	Webhooks []WebhookResponse `json:"webhooks"`
}

// SingleWebhookResponse is the response for creating or changing a webhook: the webhook as it now is
type SingleWebhookResponse struct {
	Webhook WebhookResponse `json:"webhook"`
}

// CreateWebhookRequest is a new webhook. The token is the admin's choice, typed in or generated on
// the dashboard.
type CreateWebhookRequest struct {
	Name   string `json:"name"`
	RoomID string `json:"room_id"`
	Token  string `json:"token"`
}

// UpdateWebhookRequest is changes to a webhook. A field left out stays as it is.
type UpdateWebhookRequest struct {
	Name   *string `json:"name"`
	RoomID *string `json:"room_id"`
	Token  *string `json:"token"`
}

var (
	errNoChanges      = errors.New("the request changes nothing")
	errRoomNotJoined  = errors.New("the bot isn't in that room")
	errWebhookMissing = errors.New("no such webhook")
)

// WebhooksHandler returns every webhook (admin only)
// GET /api/admin/webhooks
func WebhooksHandler(c *gin.Context) {
	ctx := c.Request.Context()

	all, err := db.ListWebhooks(ctx)
	if err != nil {
		// Already logged
		c.JSON(http.StatusInternalServerError, ErrorResponse{Error: "Failed to fetch webhooks"})
		return
	}
	joined, err := botRooms(ctx)
	if err != nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{Error: "Failed to fetch the bot's rooms"})
		return
	}

	c.JSON(http.StatusOK, WebhooksResponse{
		Webhooks: webhookResponses(all, joined, func(roomID string) string {
			return matrix.GetRoomName(ctx, roomID)
		}),
	})
}

// CreateWebhookHandler creates a webhook (admin only)
// POST /api/admin/webhooks
func CreateWebhookHandler(c *gin.Context) {
	ctx := c.Request.Context()

	var req CreateWebhookRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{Error: "Invalid request body"})
		return
	}
	joined, err := botRooms(ctx)
	if err != nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{Error: "Failed to fetch the bot's rooms"})
		return
	}
	webhook, err := newWebhook(req, joined)
	if err != nil {
		c.JSON(http.StatusBadRequest, ErrorFrom(err))
		return
	}
	webhook.ID, err = webhooks.NewID()
	if err != nil {
		log.Error().Ctx(ctx).Err(err).Msg("Failed to generate a webhook ID")
		c.JSON(http.StatusInternalServerError, ErrorResponse{Error: "Failed to create the webhook"})
		return
	}

	created, err := db.CreateWebhook(ctx, webhook)
	if errors.Is(err, db.ErrWebhookNameTaken) {
		c.JSON(http.StatusConflict, ErrorFrom(err))
		return
	}
	if err != nil {
		// Already logged
		c.JSON(http.StatusInternalServerError, ErrorResponse{Error: "Failed to create the webhook"})
		return
	}

	log.Info().Ctx(ctx).
		Str("webhook_id", created.ID).
		Str("webhook_name", created.Name).
		Str("room_id", created.RoomID).
		Msg("Created a webhook")
	c.JSON(http.StatusCreated, SingleWebhookResponse{Webhook: webhookResponse(created, joined, matrix.GetRoomName(ctx, created.RoomID))})
}

// UpdateWebhookHandler changes a webhook's name, room or token, whichever the request gives (admin
// only)
// PATCH /api/admin/webhooks/:id
func UpdateWebhookHandler(c *gin.Context) {
	ctx := c.Request.Context()
	webhookID := c.Param("id")

	var req UpdateWebhookRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{Error: "Invalid request body"})
		return
	}
	joined, err := botRooms(ctx)
	if err != nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{Error: "Failed to fetch the bot's rooms"})
		return
	}
	changes, err := webhookChanges(req, joined)
	if err != nil {
		c.JSON(http.StatusBadRequest, ErrorFrom(err))
		return
	}

	updated, err := db.UpdateWebhook(ctx, webhookID, changes)
	switch {
	case errors.Is(err, db.ErrWebhookNotFound):
		c.JSON(http.StatusNotFound, ErrorFrom(errWebhookMissing))
		return
	case errors.Is(err, db.ErrWebhookNameTaken):
		c.JSON(http.StatusConflict, ErrorFrom(err))
		return
	case err != nil:
		// Already logged
		c.JSON(http.StatusInternalServerError, ErrorResponse{Error: "Failed to change the webhook"})
		return
	}

	log.Info().Ctx(ctx).
		Str("webhook_id", updated.ID).
		Str("webhook_name", updated.Name).
		Str("room_id", updated.RoomID).
		Bool("name_changed", changes.Name != nil).
		Bool("room_changed", changes.RoomID != nil).
		Bool("token_changed", changes.TokenHash != nil).
		Msg("Changed a webhook")
	c.JSON(http.StatusOK, SingleWebhookResponse{Webhook: webhookResponse(updated, joined, matrix.GetRoomName(ctx, updated.RoomID))})
}

// DeleteWebhookHandler deletes a webhook (admin only)
// DELETE /api/admin/webhooks/:id
func DeleteWebhookHandler(c *gin.Context) {
	ctx := c.Request.Context()
	webhookID := c.Param("id")

	deleted, err := db.DeleteWebhook(ctx, webhookID)
	if err != nil {
		// Already logged
		c.JSON(http.StatusInternalServerError, ErrorResponse{Error: "Failed to delete the webhook"})
		return
	}
	if !deleted {
		c.JSON(http.StatusNotFound, ErrorFrom(errWebhookMissing))
		return
	}

	log.Info().Ctx(ctx).Str("webhook_id", webhookID).Msg("Deleted a webhook")
	c.Status(http.StatusNoContent)
}

// botRooms returns the rooms the bot has joined, the rooms a webhook can post into
func botRooms(ctx context.Context) (map[string]bool, error) {
	roomIDs, err := db.FindJoinedRooms(ctx, config.UserID)
	if err != nil {
		// Already logged
		return nil, err
	}
	joined := make(map[string]bool, len(roomIDs))
	for _, roomID := range roomIDs {
		joined[roomID] = true
	}
	return joined, nil
}

// newWebhook checks a new webhook and returns it as it is to be stored, still without its ID
func newWebhook(req CreateWebhookRequest, joined map[string]bool) (db.Webhook, error) {
	name, err := webhooks.ValidateName(req.Name)
	if err != nil {
		return db.Webhook{}, err
	}
	if !joined[req.RoomID] {
		return db.Webhook{}, errRoomNotJoined
	}
	if err := webhooks.ValidateToken(req.Token); err != nil {
		return db.Webhook{}, err
	}
	return db.Webhook{Name: name, RoomID: req.RoomID, TokenHash: webhooks.HashToken(req.Token)}, nil
}

// webhookChanges checks the changes a request makes to a webhook. Only the fields it gives are
// checked, so a webhook whose room the bot has left can still be renamed without being moved.
func webhookChanges(req UpdateWebhookRequest, joined map[string]bool) (db.WebhookChanges, error) {
	if req.Name == nil && req.RoomID == nil && req.Token == nil {
		return db.WebhookChanges{}, errNoChanges
	}

	var changes db.WebhookChanges
	if req.Name != nil {
		name, err := webhooks.ValidateName(*req.Name)
		if err != nil {
			return db.WebhookChanges{}, err
		}
		changes.Name = &name
	}
	if req.RoomID != nil {
		if !joined[*req.RoomID] {
			return db.WebhookChanges{}, errRoomNotJoined
		}
		changes.RoomID = req.RoomID
	}
	if req.Token != nil {
		if err := webhooks.ValidateToken(*req.Token); err != nil {
			return db.WebhookChanges{}, err
		}
		changes.TokenHash = webhooks.HashToken(*req.Token)
	}
	return changes, nil
}

// webhookResponses turns webhooks into their API form. Several webhooks may post into the same room,
// so each room's name is looked up once.
func webhookResponses(all []db.Webhook, joined map[string]bool, roomName func(roomID string) string) []WebhookResponse {
	names := make(map[string]string)
	responses := make([]WebhookResponse, len(all))
	for i, webhook := range all {
		name, ok := names[webhook.RoomID]
		if !ok {
			name = roomName(webhook.RoomID)
			names[webhook.RoomID] = name
		}
		responses[i] = webhookResponse(webhook, joined, name)
	}
	return responses
}

// webhookResponse turns a webhook into its API form
func webhookResponse(webhook db.Webhook, joined map[string]bool, roomName string) WebhookResponse {
	response := WebhookResponse{
		ID:        webhook.ID,
		Name:      webhook.Name,
		RoomID:    webhook.RoomID,
		RoomName:  roomName,
		BotInRoom: joined[webhook.RoomID],
		URL:       webhookURL(config.FrontendURL, webhook.ID),
		CreatedAt: webhook.CreatedAt.UTC().Format(time.RFC3339),
	}
	if webhook.LastUsedAt != nil {
		lastUsed := webhook.LastUsedAt.UTC().Format(time.RFC3339)
		response.LastUsedAt = &lastUsed
	}
	return response
}

// webhookURL is where a webhook's messages are sent. The dashboard and the hooks are served by the
// same server, so the URL is the dashboard's.
func webhookURL(frontendURL, webhookID string) string {
	return strings.TrimRight(frontendURL, "/") + "/hooks/message/" + url.PathEscape(webhookID)
}
