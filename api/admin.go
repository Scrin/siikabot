package api

import (
	"net/http"

	"github.com/Scrin/siikabot/config"
	"github.com/Scrin/siikabot/db"
	"github.com/Scrin/siikabot/matrix"
	"github.com/gin-gonic/gin"
	"github.com/rs/zerolog/log"
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

// AdminRoomsHandler returns every room the bot is in (admin only)
// GET /api/admin/rooms
func AdminRoomsHandler(c *gin.Context) {
	ctx := c.Request.Context()

	roomIDs, err := db.FindJoinedRooms(ctx, config.UserID)
	if err != nil {
		log.Error().Ctx(ctx).Err(err).Msg("Failed to fetch all rooms")
		c.JSON(http.StatusInternalServerError, ErrorResponse{Error: "Failed to fetch rooms"})
		return
	}

	c.JSON(http.StatusOK, roomsResponse(roomIDs, func(roomID string) string {
		return matrix.GetRoomName(ctx, roomID)
	}))
}

// AdminRoomMembersHandler returns the joined members of any room the bot is in (admin only)
// GET /api/admin/rooms/:roomId/members
func AdminRoomMembersHandler(c *gin.Context) {
	ctx := c.Request.Context()

	roomID := c.Param("roomId")
	if roomID == "" {
		c.JSON(http.StatusBadRequest, ErrorResponse{Error: "Missing room ID"})
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
