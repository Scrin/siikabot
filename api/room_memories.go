package api

import (
	"net/http"
	"sort"
	"strconv"
	"time"

	"github.com/Scrin/siikabot/db"
	"github.com/Scrin/siikabot/matrix"
	"github.com/Scrin/siikabot/metrics"
	"github.com/gin-gonic/gin"
	"github.com/rs/zerolog/log"
)

// RoomMemoryResponse is one of the bot's memories of a room
type RoomMemoryResponse struct {
	ID        int64  `json:"id"`
	Memory    string `json:"memory"`
	CreatedAt string `json:"created_at"`
	// UpdatedAt is when a later pass last revised the memory, the same as CreatedAt if none has
	UpdatedAt string `json:"updated_at"`
}

// RoomMemoriesEntry is a room the bot remembers things about, with what it remembers
type RoomMemoriesEntry struct {
	RoomID   string `json:"room_id"`
	RoomName string `json:"room_name,omitempty"`
	// LastReflectedAt is when the bot last thought back over the room's conversations, null if it
	// never has
	LastReflectedAt *string              `json:"last_reflected_at"`
	Memories        []RoomMemoryResponse `json:"memories"`
}

// RoomMemoriesResponse is the response for the room memories endpoint
type RoomMemoriesResponse struct {
	Rooms []RoomMemoriesEntry `json:"rooms"`
}

// DeleteRoomMemoriesResponse is the response for deleting all of a room's memories
type DeleteRoomMemoriesResponse struct {
	DeletedCount int64 `json:"deleted_count"`
}

// RoomMemoriesHandler returns the bot's memories of every room (admin only)
// GET /api/admin/room-memories
func RoomMemoriesHandler(c *gin.Context) {
	ctx := c.Request.Context()

	memories, err := db.GetAllRoomMemories(ctx)
	if err != nil {
		log.Error().Ctx(ctx).Err(err).Msg("Failed to fetch room memories")
		c.JSON(http.StatusInternalServerError, ErrorResponse{Error: "Failed to fetch room memories"})
		return
	}
	progress, err := db.GetAllRoomMemoryProgress(ctx)
	if err != nil {
		log.Error().Ctx(ctx).Err(err).Msg("Failed to fetch room memory progress")
		c.JSON(http.StatusInternalServerError, ErrorResponse{Error: "Failed to fetch room memories"})
		return
	}

	c.JSON(http.StatusOK, RoomMemoriesResponse{
		Rooms: roomMemoryEntries(memories, progress, func(roomID string) string {
			return matrix.GetRoomName(ctx, roomID)
		}),
	})
}

// roomMemoryEntries groups the bot's memories by room, with each room's name and when the bot last
// thought back over it. The room whose memories changed most recently comes first. A room the bot
// remembers nothing about is left out.
func roomMemoryEntries(memories []db.RoomMemory, progress []db.RoomMemoryProgress, roomName func(roomID string) string) []RoomMemoriesEntry {
	reflectedAt := make(map[string]time.Time, len(progress))
	for _, p := range progress {
		if p.ReflectedAt != nil {
			reflectedAt[p.RoomID] = *p.ReflectedAt
		}
	}

	byRoom := make(map[string]int)
	latest := make(map[string]time.Time)
	var entries []RoomMemoriesEntry
	for _, memory := range memories {
		i, ok := byRoom[memory.RoomID]
		if !ok {
			i = len(entries)
			byRoom[memory.RoomID] = i
			entry := RoomMemoriesEntry{RoomID: memory.RoomID, RoomName: roomName(memory.RoomID)}
			if at, ok := reflectedAt[memory.RoomID]; ok {
				formatted := at.UTC().Format(time.RFC3339)
				entry.LastReflectedAt = &formatted
			}
			entries = append(entries, entry)
		}
		entries[i].Memories = append(entries[i].Memories, RoomMemoryResponse{
			ID:        memory.ID,
			Memory:    memory.Memory,
			CreatedAt: memory.CreatedAt.UTC().Format(time.RFC3339),
			UpdatedAt: memory.UpdatedAt.UTC().Format(time.RFC3339),
		})
		if memory.UpdatedAt.After(latest[memory.RoomID]) {
			latest[memory.RoomID] = memory.UpdatedAt
		}
	}

	sort.SliceStable(entries, func(a, b int) bool {
		return latest[entries[a].RoomID].After(latest[entries[b].RoomID])
	})
	if entries == nil {
		entries = []RoomMemoriesEntry{}
	}
	return entries
}

// DeleteRoomMemoryHandler deletes one of the bot's memories of a room (admin only)
// DELETE /api/admin/room-memories/:id
func DeleteRoomMemoryHandler(c *gin.Context) {
	ctx := c.Request.Context()

	memoryID, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{Error: "Invalid memory ID"})
		return
	}

	deleted, err := db.DeleteRoomMemory(ctx, memoryID)
	if err != nil {
		log.Error().Ctx(ctx).Err(err).Int64("memory_id", memoryID).Msg("Failed to delete room memory")
		c.JSON(http.StatusInternalServerError, ErrorResponse{Error: "Failed to delete the memory"})
		return
	}
	if !deleted {
		c.JSON(http.StatusNotFound, ErrorResponse{Error: "The memory no longer exists"})
		return
	}

	metrics.RecordRoomMemoriesDeleted(1)
	log.Info().Ctx(ctx).Int64("memory_id", memoryID).Msg("Room memory deleted on the dashboard")
	c.JSON(http.StatusOK, gin.H{"status": "ok"})
}

// DeleteRoomMemoriesHandler deletes all of the bot's memories of a room (admin only)
// DELETE /api/admin/rooms/:roomId/memories
func DeleteRoomMemoriesHandler(c *gin.Context) {
	ctx := c.Request.Context()

	roomID := c.Param("roomId")
	if roomID == "" {
		c.JSON(http.StatusBadRequest, ErrorResponse{Error: "Room ID is required"})
		return
	}

	count, err := db.DeleteRoomMemories(ctx, roomID)
	if err != nil {
		log.Error().Ctx(ctx).Err(err).Str("room_id", roomID).Msg("Failed to delete room memories")
		c.JSON(http.StatusInternalServerError, ErrorResponse{Error: "Failed to delete the memories"})
		return
	}

	metrics.RecordRoomMemoriesDeleted(count)
	log.Info().Ctx(ctx).Str("room_id", roomID).Int64("deleted_count", count).Msg("Room memories deleted on the dashboard")
	c.JSON(http.StatusOK, DeleteRoomMemoriesResponse{DeletedCount: count})
}
