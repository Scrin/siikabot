package api

import (
	"net/http"
	"strconv"
	"time"

	"github.com/Scrin/siikabot/db"
	"github.com/gin-gonic/gin"
	"github.com/rs/zerolog/log"
)

// MemoryResponse represents a single memory in the API response
type MemoryResponse struct {
	ID        int64  `json:"id"`
	Memory    string `json:"memory"`
	CreatedAt string `json:"created_at"`
}

// MemoriesResponse is the response for the memories endpoint
type MemoriesResponse struct {
	Memories []MemoryResponse `json:"memories"`
}

// DeleteAllMemoriesResponse is the response for the delete all memories endpoint
type DeleteAllMemoriesResponse struct {
	DeletedCount int64 `json:"deleted_count"`
}

// MemoriesHandler returns the authenticated user's memories
// GET /api/memories
// Requires Authorization: Bearer <token> header (use with AuthMiddleware)
func MemoriesHandler(c *gin.Context) {
	ctx := c.Request.Context()

	userID, ok := GetUserIDFromContext(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, ErrorResponse{Error: "Not authenticated"})
		return
	}

	memories, err := db.GetUserMemories(ctx, userID)
	if err != nil {
		log.Error().Ctx(ctx).Err(err).Str("user_id", userID).Msg("Failed to fetch memories")
		c.JSON(http.StatusInternalServerError, ErrorResponse{Error: "Failed to fetch memories"})
		return
	}

	response := MemoriesResponse{
		Memories: make([]MemoryResponse, len(memories)),
	}
	for i, mem := range memories {
		response.Memories[i] = MemoryResponse{
			ID:        mem.ID,
			Memory:    mem.Memory,
			CreatedAt: mem.CreatedAt.UTC().Format(time.RFC3339),
		}
	}

	c.JSON(http.StatusOK, response)
}

// DeleteMemoryHandler deletes a specific memory
// DELETE /api/memories/:id
// Requires Authorization: Bearer <token> header (use with AuthMiddleware)
func DeleteMemoryHandler(c *gin.Context) {
	ctx := c.Request.Context()

	userID, ok := GetUserIDFromContext(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, ErrorResponse{Error: "Not authenticated"})
		return
	}

	memoryIDStr := c.Param("id")
	memoryID, err := strconv.ParseInt(memoryIDStr, 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{Error: "Invalid memory ID"})
		return
	}

	if err := db.DeleteMemory(ctx, userID, memoryID); err != nil {
		log.Error().Ctx(ctx).Err(err).Str("user_id", userID).Int64("memory_id", memoryID).Msg("Failed to delete memory")
		c.JSON(http.StatusInternalServerError, ErrorResponse{Error: "Failed to delete memory"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"status": "ok"})
}

// DeleteAllMemoriesHandler deletes all memories for the authenticated user
// DELETE /api/memories
// Requires Authorization: Bearer <token> header (use with AuthMiddleware)
func DeleteAllMemoriesHandler(c *gin.Context) {
	ctx := c.Request.Context()

	userID, ok := GetUserIDFromContext(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, ErrorResponse{Error: "Not authenticated"})
		return
	}

	count, err := db.DeleteAllMemories(ctx, userID)
	if err != nil {
		log.Error().Ctx(ctx).Err(err).Str("user_id", userID).Msg("Failed to delete all memories")
		c.JSON(http.StatusInternalServerError, ErrorResponse{Error: "Failed to delete memories"})
		return
	}

	c.JSON(http.StatusOK, DeleteAllMemoriesResponse{DeletedCount: count})
}
