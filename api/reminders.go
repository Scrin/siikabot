package api

import (
	"errors"
	"net/http"
	"strconv"
	"time"

	"github.com/Scrin/siikabot/commands/remind"
	"github.com/Scrin/siikabot/db"
	"github.com/Scrin/siikabot/matrix"
	"github.com/gin-gonic/gin"
	"github.com/rs/zerolog/log"
)

// ReminderResponse represents a single reminder in the API response
type ReminderResponse struct {
	ID         int64  `json:"id"`
	RemindTime string `json:"remind_time"`
	RoomID     string `json:"room_id"`
	RoomName   string `json:"room_name,omitempty"`
	Message    string `json:"message"`
}

// RemindersResponse is the response for the reminders endpoint
type RemindersResponse struct {
	Reminders []ReminderResponse `json:"reminders"`
}

// RemindersHandler returns the authenticated user's active reminders
// GET /api/reminders
// Requires Authorization: Bearer <token> header (use with AuthMiddleware)
func RemindersHandler(c *gin.Context) {
	ctx := c.Request.Context()

	userID, ok := GetUserIDFromContext(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, ErrorResponse{Error: "Not authenticated"})
		return
	}

	reminders, err := db.GetRemindersByUserID(ctx, userID)
	if err != nil {
		log.Error().Ctx(ctx).Err(err).Str("user_id", userID).Msg("Failed to fetch reminders")
		c.JSON(http.StatusInternalServerError, ErrorResponse{Error: "Failed to fetch reminders"})
		return
	}

	response := RemindersResponse{
		Reminders: make([]ReminderResponse, len(reminders)),
	}
	for i, rem := range reminders {
		roomName := matrix.GetRoomName(ctx, rem.RoomID)
		response.Reminders[i] = ReminderResponse{
			ID:         rem.ID,
			RemindTime: rem.RemindTime.UTC().Format(time.RFC3339),
			RoomID:     rem.RoomID,
			RoomName:   roomName,
			Message:    rem.Message,
		}
	}

	c.JSON(http.StatusOK, response)
}

// DeleteReminderHandler deletes one of the authenticated user's reminders, in whichever room it
// was set, and stops it from going off. Nothing is posted in the room.
// DELETE /api/reminders/:id
// Requires Authorization: Bearer <token> header (use with AuthMiddleware)
func DeleteReminderHandler(c *gin.Context) {
	ctx := c.Request.Context()

	userID, ok := GetUserIDFromContext(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, ErrorResponse{Error: "Not authenticated"})
		return
	}

	reminderID, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{Error: "Invalid reminder ID"})
		return
	}

	_, err = remind.Delete(ctx, db.ReminderScope{UserID: userID}, reminderID, remind.ViaDashboard)
	if err != nil {
		status, message := deleteReminderFailure(err)
		if status == http.StatusInternalServerError {
			log.Error().Ctx(ctx).Err(err).Str("user_id", userID).Int64("reminder_id", reminderID).Msg("Failed to delete reminder")
		}
		c.JSON(status, ErrorResponse{Error: message})
		return
	}

	c.JSON(http.StatusOK, gin.H{"status": "ok"})
}

// deleteReminderFailure is the status and message for a reminder that couldn't be deleted
func deleteReminderFailure(err error) (int, string) {
	switch {
	case errors.Is(err, remind.ErrNotFound):
		return http.StatusNotFound, "The reminder no longer exists"
	case errors.Is(err, remind.ErrAlreadyDelivered):
		return http.StatusConflict, "The reminder has already gone off"
	default:
		return http.StatusInternalServerError, "Failed to delete reminder"
	}
}
