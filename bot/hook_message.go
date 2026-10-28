package bot

import (
	"context"
	"errors"
	"io"
	"net/http"
	"time"

	"github.com/Scrin/siikabot/api"
	"github.com/Scrin/siikabot/config"
	"github.com/Scrin/siikabot/constants"
	"github.com/Scrin/siikabot/db"
	"github.com/Scrin/siikabot/matrix"
	"github.com/Scrin/siikabot/metrics"
	"github.com/Scrin/siikabot/webhooks"
	"github.com/gin-gonic/gin"
	"github.com/rs/zerolog/log"
	"maunium.net/go/mautrix/event"
)

// The webhooks the admin sets up on the web dashboard post the message a request brings into their
// room, under their name. Anything that can make an HTTP request can send one, such as curl:
//
//	curl -H "Authorization: Bearer $TOKEN" --data-binary "hello there" https://<bot>/hooks/message/<id>

// messageHookTimeout is how long a request waits for its message to go out. The message is given up
// at the same time, so one that timed out never goes out later, and sending it again can't post it
// twice.
const messageHookTimeout = 30 * time.Second

// messageHookWebhookKey is where the middleware leaves the webhook for the handler
const messageHookWebhookKey = "webhook"

// MessageHookAuthMiddleware creates Gin middleware that checks the bearer token of a request to a
// webhook, looked up with lookUp, and leaves the webhook for the handler. An unknown webhook and a
// wrong token get the same answer, so the endpoint can't be used to find out which webhooks exist.
func MessageHookAuthMiddleware(lookUp func(ctx context.Context, id string) (db.Webhook, bool, error)) gin.HandlerFunc {
	return func(c *gin.Context) {
		ctx := c.Request.Context()
		webhookID := c.Param("id")

		webhook, found, err := lookUp(ctx, webhookID)
		if err != nil {
			// Already logged
			c.AbortWithStatusJSON(http.StatusInternalServerError, api.ErrorResponse{Error: "Failed to look up the webhook"})
			return
		}

		// The token is hashed whether the webhook was found or not
		token := api.ExtractBearerToken(c.Request)
		matches := webhooks.TokenMatches(webhook.TokenHash, token)
		if !found || !matches {
			log.Warn().Ctx(ctx).
				Str("webhook_id", webhookID).
				Bool("token_provided", token != "").
				Bool("webhook_found", found).
				Msg("Message webhook called without a valid token")
			c.Header("WWW-Authenticate", `Bearer realm="siikabot"`)
			c.AbortWithStatusJSON(http.StatusUnauthorized, api.ErrorResponse{Error: "Unknown webhook, or a missing or wrong token"})
			return
		}

		c.Set(messageHookWebhookKey, webhook)
		c.Next()
	}
}

// MessageHookHandler posts the message a request brings into its webhook's room, and answers with
// the message's event ID once it has gone out
func MessageHookHandler(c *gin.Context) {
	ctx := c.Request.Context()
	metrics.RecordWebhookHandled(constants.WebhookMessage)
	webhook := c.MustGet(messageHookWebhookKey).(db.Webhook)

	message, status, err := readHookMessage(c)
	if err != nil {
		log.Warn().Ctx(ctx).Err(err).
			Str("webhook_id", webhook.ID).
			Int("status", status).
			Msg("Rejected a message webhook request")
		c.JSON(status, api.ErrorFrom(err))
		return
	}

	member, joined, err := db.GetRoomMember(ctx, webhook.RoomID, config.UserID)
	if err != nil {
		// Already logged
		c.JSON(http.StatusInternalServerError, api.ErrorResponse{Error: "Failed to check the webhook's room"})
		return
	}
	if !joined || member.Membership != event.MembershipJoin {
		log.Warn().Ctx(ctx).
			Str("webhook_id", webhook.ID).
			Str("room_id", webhook.RoomID).
			Msg("Message webhook called for a room the bot isn't in")
		c.JSON(http.StatusConflict, api.ErrorResponse{Error: "The bot is no longer in the webhook's room"})
		return
	}

	msg := matrix.NewWebhookMessage(matrix.WebhookOrigin{ID: webhook.ID, Name: webhook.Name}, message)
	if msg.Size() > webhooks.MaxContentBytes {
		log.Warn().Ctx(ctx).
			Str("webhook_id", webhook.ID).
			Int("message_length", len(message)).
			Int("content_size", msg.Size()).
			Msg("Message webhook message too large once formatted")
		c.JSON(http.StatusRequestEntityTooLarge, api.ErrorResponse{Error: "The message is too large once formatted"})
		return
	}

	log.Debug().Ctx(ctx).
		Str("webhook_id", webhook.ID).
		Str("webhook_name", webhook.Name).
		Str("room_id", webhook.RoomID).
		Int("message_length", len(message)).
		Msg("Relaying a message webhook message")

	eventID := <-matrix.SendWebhookMessage(ctx, webhook.RoomID, msg, messageHookTimeout)
	if eventID == "" {
		metrics.RecordWebhookEventHandled(constants.WebhookMessage, constants.WebhookEventMessageFailed)
		log.Error().Ctx(ctx).
			Str("webhook_id", webhook.ID).
			Str("room_id", webhook.RoomID).
			Msg("Failed to relay a message webhook message")
		c.JSON(http.StatusBadGateway, api.ErrorResponse{Error: "The message couldn't be sent to Matrix. It won't be sent later, so it's safe to try again"})
		return
	}
	metrics.RecordWebhookEventHandled(constants.WebhookMessage, constants.WebhookEventMessageSent)

	// The message is out either way, so a failure here, already logged, only leaves the webhook's
	// last use out of date
	_ = db.TouchWebhook(ctx, webhook.ID)

	c.JSON(http.StatusOK, gin.H{"event_id": eventID})
}

// readHookMessage reads the message a request to a webhook brings, or returns the status and error
// to answer with
func readHookMessage(c *gin.Context) (string, int, error) {
	body, err := io.ReadAll(http.MaxBytesReader(c.Writer, c.Request.Body, webhooks.MaxBodyBytes))
	var tooLarge *http.MaxBytesError
	if errors.As(err, &tooLarge) {
		return "", http.StatusRequestEntityTooLarge, webhooks.ErrMessageTooLarge
	}
	if err != nil {
		return "", http.StatusBadRequest, errors.New("the request body couldn't be read")
	}

	message, err := webhooks.ParseMessage(c.GetHeader("Content-Type"), body)
	if err != nil {
		return "", http.StatusBadRequest, err
	}
	return message, http.StatusOK, nil
}
