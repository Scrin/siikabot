package api

import (
	"encoding/json"
	"net/http"

	"github.com/Scrin/siikabot/db"
	"github.com/Scrin/siikabot/matrix"
	"github.com/rs/zerolog/log"
	mid "maunium.net/go/mautrix/id"
)

// RoomResponse represents a single room in the API response
type RoomResponse struct {
	RoomID   string `json:"room_id"`
	RoomName string `json:"room_name,omitempty"`
}

// RoomsResponse is the response for the rooms endpoint
type RoomsResponse struct {
	Rooms []RoomResponse `json:"rooms"`
}

// RoomsHandler returns the rooms shared between the bot and the authenticated user
// GET /api/rooms
// Requires Authorization: Bearer <token> header (use with AuthMiddleware)
func RoomsHandler(w http.ResponseWriter, r *http.Request) {
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

	roomIDs, err := db.FindSharedRooms(ctx, mid.UserID(userID))
	if err != nil {
		log.Error().Ctx(ctx).Err(err).Str("user_id", userID).Msg("Failed to fetch rooms")
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(ErrorResponse{Error: "Failed to fetch rooms"})
		return
	}

	response := RoomsResponse{
		Rooms: make([]RoomResponse, len(roomIDs)),
	}
	for i, roomID := range roomIDs {
		roomName := matrix.GetRoomName(ctx, string(roomID))
		response.Rooms[i] = RoomResponse{
			RoomID:   string(roomID),
			RoomName: roomName,
		}
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(response); err != nil {
		log.Error().Ctx(ctx).Err(err).Msg("Failed to encode rooms response")
	}
}
