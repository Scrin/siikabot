package api

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/Scrin/siikabot/db"
	"github.com/Scrin/siikabot/matrix"
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
func RemindersHandler(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	userID, ok := GetUserIDFromContext(ctx)
	if !ok {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusUnauthorized)
		json.NewEncoder(w).Encode(ErrorResponse{Error: "Not authenticated"})
		return
	}

	reminders, err := db.GetRemindersByUserID(ctx, userID)
	if err != nil {
		log.Error().Ctx(ctx).Err(err).Str("user_id", userID).Msg("Failed to fetch reminders")
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(ErrorResponse{Error: "Failed to fetch reminders"})
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

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(response); err != nil {
		log.Error().Ctx(ctx).Err(err).Msg("Failed to encode reminders response")
	}
}
