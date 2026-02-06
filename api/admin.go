package api

import (
	"net/http"

	"github.com/Scrin/siikabot/config"
	"github.com/Scrin/siikabot/db"
	"github.com/Scrin/siikabot/matrix"
	"github.com/gin-gonic/gin"
	"github.com/rs/zerolog/log"
	mid "maunium.net/go/mautrix/id"
)

// AdminAuthMiddleware checks if the authenticated user is the configured admin
func AdminAuthMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		userID, ok := GetUserIDFromContext(c)
		if !ok {
			c.AbortWithStatusJSON(http.StatusUnauthorized, ErrorResponse{Error: "Not authenticated"})
			return
		}

		if userID != config.Admin {
			c.AbortWithStatusJSON(http.StatusForbidden, ErrorResponse{Error: "Admin access required"})
			return
		}

		c.Next()
	}
}

// AdminRoomsHandler returns all rooms known to the bot (admin only)
// GET /api/admin/rooms
func AdminRoomsHandler(c *gin.Context) {
	ctx := c.Request.Context()

	roomIDs, err := db.GetAllRooms(ctx)
	if err != nil {
		log.Error().Ctx(ctx).Err(err).Msg("Failed to fetch all rooms")
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

// AdminRoomMembersHandler returns members of any room (admin only)
// GET /api/admin/rooms/:roomId/members
func AdminRoomMembersHandler(c *gin.Context) {
	ctx := c.Request.Context()

	roomID := c.Param("roomId")
	if roomID == "" {
		c.JSON(http.StatusBadRequest, ErrorResponse{Error: "Missing room ID"})
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
