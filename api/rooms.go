package api

import (
	"net/http"
	"slices"

	"github.com/Scrin/siikabot/db"
	"github.com/Scrin/siikabot/matrix"
	"github.com/gin-gonic/gin"
	"github.com/rs/zerolog/log"
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
	// DisplayName is their display name in the room, left out if they have none
	DisplayName string `json:"display_name,omitempty"`
}

// RoomMembersResponse is the response for room members endpoint
type RoomMembersResponse struct {
	Members []RoomMemberResponse `json:"members"`
}

// roomsResponse lists rooms with their names
func roomsResponse(roomIDs []string, roomName func(roomID string) string) RoomsResponse {
	response := RoomsResponse{Rooms: make([]RoomResponse, len(roomIDs))}
	for i, roomID := range roomIDs {
		response.Rooms[i] = RoomResponse{RoomID: roomID, RoomName: roomName(roomID)}
	}
	return response
}

// membersResponse lists the joined members of a room
func membersResponse(members []db.RoomMember) RoomMembersResponse {
	response := RoomMembersResponse{Members: make([]RoomMemberResponse, len(members))}
	for i, member := range members {
		response.Members[i] = RoomMemberResponse{UserID: member.UserID}
		if member.DisplayName != nil {
			response.Members[i].DisplayName = *member.DisplayName
		}
	}
	return response
}

// RoomsHandler returns the rooms shared between the bot and the authenticated user: the rooms the
// bot is in that the user has joined. Someone who is only invited, or has left, doesn't see a room.
// GET /api/rooms
// Requires Authorization: Bearer <token> header (use with AuthMiddleware)
func RoomsHandler(c *gin.Context) {
	ctx := c.Request.Context()

	userID, ok := GetUserIDFromContext(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, ErrorResponse{Error: "Not authenticated"})
		return
	}

	roomIDs, err := db.FindJoinedRooms(ctx, userID)
	if err != nil {
		log.Error().Ctx(ctx).Err(err).Str("user_id", userID).Msg("Failed to fetch rooms")
		c.JSON(http.StatusInternalServerError, ErrorResponse{Error: "Failed to fetch rooms"})
		return
	}

	c.JSON(http.StatusOK, roomsResponse(roomIDs, func(roomID string) string {
		return matrix.GetRoomName(ctx, roomID)
	}))
}

// RoomMembersHandler returns the joined members of a room shared with the user
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
	joinedRooms, err := db.FindJoinedRooms(ctx, userID)
	if err != nil {
		log.Error().Ctx(ctx).Err(err).Str("user_id", userID).Msg("Failed to verify room access")
		c.JSON(http.StatusInternalServerError, ErrorResponse{Error: "Failed to verify access"})
		return
	}
	if !slices.Contains(joinedRooms, roomID) {
		c.JSON(http.StatusForbidden, ErrorResponse{Error: "Access denied to this room"})
		return
	}

	members, err := db.GetJoinedMembers(ctx, roomID)
	if err != nil {
		log.Error().Ctx(ctx).Err(err).Str("room_id", roomID).Msg("Failed to fetch room members")
		c.JSON(http.StatusInternalServerError, ErrorResponse{Error: "Failed to fetch room members"})
		return
	}

	c.JSON(http.StatusOK, membersResponse(members))
}
