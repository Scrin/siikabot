package api

import (
	"net/http"

	"github.com/Scrin/siikabot/db"
	"github.com/Scrin/siikabot/matrix"
	"github.com/gin-gonic/gin"
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

// RoomMemberResponse represents a single room member
type RoomMemberResponse struct {
	UserID string `json:"user_id"`
}

// RoomMembersResponse is the response for room members endpoint
type RoomMembersResponse struct {
	Members []RoomMemberResponse `json:"members"`
}

// RoomsHandler returns the rooms shared between the bot and the authenticated user
// GET /api/rooms
// Requires Authorization: Bearer <token> header (use with AuthMiddleware)
func RoomsHandler(c *gin.Context) {
	ctx := c.Request.Context()

	userID, ok := GetUserIDFromContext(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, ErrorResponse{Error: "Not authenticated"})
		return
	}

	roomIDs, err := db.FindSharedRooms(ctx, mid.UserID(userID))
	if err != nil {
		log.Error().Ctx(ctx).Err(err).Str("user_id", userID).Msg("Failed to fetch rooms")
		c.JSON(http.StatusInternalServerError, ErrorResponse{Error: "Failed to fetch rooms"})
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

	c.JSON(http.StatusOK, response)
}

// RoomMembersHandler returns members of a specific room shared with the user
// GET /api/rooms/:roomId/members
// Requires Authorization: Bearer <token> header (use with AuthMiddleware)
func RoomMembersHandler(c *gin.Context) {
	ctx := c.Request.Context()

	userID, ok := GetUserIDFromContext(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, ErrorResponse{Error: "Not authenticated"})
		return
	}

	roomID := c.Param("roomId")
	if roomID == "" {
		c.JSON(http.StatusBadRequest, ErrorResponse{Error: "Missing room ID"})
		return
	}

	// Verify user has access to this room
	sharedRooms, err := db.FindSharedRooms(ctx, mid.UserID(userID))
	if err != nil {
		log.Error().Ctx(ctx).Err(err).Str("user_id", userID).Msg("Failed to verify room access")
		c.JSON(http.StatusInternalServerError, ErrorResponse{Error: "Failed to verify access"})
		return
	}

	hasAccess := false
	for _, sharedRoomID := range sharedRooms {
		if string(sharedRoomID) == roomID {
			hasAccess = true
			break
		}
	}

	if !hasAccess {
		c.JSON(http.StatusForbidden, ErrorResponse{Error: "Access denied to this room"})
		return
	}

	members, err := db.GetRoomMembers(ctx, mid.RoomID(roomID))
	if err != nil {
		log.Error().Ctx(ctx).Err(err).Str("room_id", roomID).Msg("Failed to fetch room members")
		c.JSON(http.StatusInternalServerError, ErrorResponse{Error: "Failed to fetch room members"})
		return
	}

	response := RoomMembersResponse{
		Members: make([]RoomMemberResponse, len(members)),
	}
	for i, member := range members {
		response.Members[i] = RoomMemberResponse{
			UserID: string(member),
		}
	}

	c.JSON(http.StatusOK, response)
}
