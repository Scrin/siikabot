package api

import (
	"context"
	"errors"
	"net/http"
	"sort"
	"strconv"
	"time"

	"github.com/Scrin/siikabot/commands/chat"
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
	// CreatedTraceID is the trace of the pass that formed the memory, and UpdatedTraceID of the one that
	// last revised it. Null when it wasn't recorded.
	CreatedTraceID *string `json:"created_trace_id"`
	UpdatedTraceID *string `json:"updated_trace_id"`
}

// RoomMemoryStatus is how a room stands with the bot's passes over it
type RoomMemoryStatus struct {
	// State is up_to_date, waiting (for the room to be quiet for long enough), retrying (after a failed
	// pass), due (at the next check) or off (with new conversations while the passes are off)
	State string `json:"state"`
	// DueAt is when the room is due a pass, null when it has nothing new or the passes are off
	DueAt *string `json:"due_at"`
	// LatestActivityAt is when the newest conversation the bot hasn't thought back over happened, null
	// if there is none
	LatestActivityAt *string `json:"latest_activity_at"`
	// FailedAttempts is how many passes have failed in a row
	FailedAttempts int `json:"failed_attempts"`
	// LastAttemptAt, LastOutcome and LastTraceID are when the latest pass ran, how it went, such as
	// unchanged or request_failed, and its trace. Null when not recorded.
	LastAttemptAt *string `json:"last_attempt_at"`
	LastOutcome   *string `json:"last_outcome"`
	LastTraceID   *string `json:"last_trace_id"`
}

// RoomMemoriesEntry is a room the bot remembers things about or has been thinking back over, with what
// it remembers and how the room stands with its passes
type RoomMemoriesEntry struct {
	RoomID   string `json:"room_id"`
	RoomName string `json:"room_name,omitempty"`
	// LastReflectedAt is when a pass over the room last completed, null if none has
	LastReflectedAt *string              `json:"last_reflected_at"`
	Status          RoomMemoryStatus     `json:"status"`
	Memories        []RoomMemoryResponse `json:"memories"`
}

// RoomMemoriesResponse is the response for the room memories endpoint
type RoomMemoriesResponse struct {
	Rooms []RoomMemoriesEntry `json:"rooms"`
}

// DeleteRoomMemoriesResponse is the response for deleting all of a room's memories
type DeleteRoomMemoriesResponse struct {
	DeletedCount int `json:"deleted_count"`
}

// ReflectResponse is how a pass the admin started went
type ReflectResponse struct {
	// Outcome is changed or unchanged for a pass that completed, and otherwise why it was skipped, such
	// as nothing_new, or why it failed, such as request_failed
	Outcome                string `json:"outcome"`
	Skipped                bool   `json:"skipped"`
	Failed                 bool   `json:"failed"`
	ConversationCount      int    `json:"conversation_count"`
	ShownConversationCount int    `json:"shown_conversation_count"`
	Added                  int    `json:"added"`
	Updated                int    `json:"updated"`
	Deleted                int    `json:"deleted"`
	Rejected               int    `json:"rejected"`
	// TraceID is the pass's trace, empty without tracing
	TraceID string `json:"trace_id"`
}

// RoomMemoriesHandler returns the bot's memories of every group room, and how each stands with its
// passes (admin only)
// GET /api/admin/room-memories
func RoomMemoriesHandler(c *gin.Context) {
	ctx := c.Request.Context()

	memories, err := db.GetAllRoomMemories(ctx)
	if err != nil {
		log.Error().Ctx(ctx).Err(err).Msg("Failed to fetch room memories")
		c.JSON(http.StatusInternalServerError, ErrorResponse{Error: "Failed to fetch room memories"})
		return
	}
	states, err := db.GetAllRoomMemoryStates(ctx)
	if err != nil {
		log.Error().Ctx(ctx).Err(err).Msg("Failed to fetch room memory states")
		c.JSON(http.StatusInternalServerError, ErrorResponse{Error: "Failed to fetch room memories"})
		return
	}
	cfg, err := db.GetChatConfig(ctx)
	if err != nil {
		log.Error().Ctx(ctx).Err(err).Msg("Failed to fetch the chat config for room memories")
		c.JSON(http.StatusInternalServerError, ErrorResponse{Error: "Failed to fetch room memories"})
		return
	}

	now := time.Now()
	c.JSON(http.StatusOK, RoomMemoriesResponse{
		Rooms: roomMemoryEntries(memories, states,
			func(state db.RoomMemoryState) chat.ReflectionSchedule {
				return chat.ScheduleOf(state, cfg.RoomMemoryDelay, now)
			},
			func(roomID string) string { return matrix.GetRoomName(ctx, roomID) },
			func(roomID string) bool { return chat.IsDirectChat(ctx, roomID) },
		),
	})
}

// roomMemoryEntries lists each room the bot remembers things about or has been thinking back over, with
// its memories, its name and how it stands with the passes. Direct chats, which the bot keeps no
// memories of, are left out. The room with the most recent activity, of its memories, its passes or its
// conversations, comes first.
func roomMemoryEntries(memories []db.RoomMemory, states []db.RoomMemoryState,
	schedule func(db.RoomMemoryState) chat.ReflectionSchedule, roomName func(string) string,
	isDirectChat func(string) bool,
) []RoomMemoriesEntry {
	byRoom := make(map[string][]db.RoomMemory)
	latest := make(map[string]time.Time)
	later := func(roomID string, t *time.Time) {
		if t != nil && t.After(latest[roomID]) {
			latest[roomID] = *t
		}
	}
	for _, memory := range memories {
		byRoom[memory.RoomID] = append(byRoom[memory.RoomID], memory)
		later(memory.RoomID, &memory.UpdatedAt)
	}

	entries := []RoomMemoriesEntry{}
	for _, state := range states {
		if isDirectChat(state.RoomID) {
			continue
		}
		later(state.RoomID, state.LatestAt)
		later(state.RoomID, state.LastAttemptAt)

		s := schedule(state)
		entry := RoomMemoriesEntry{
			RoomID:          state.RoomID,
			RoomName:        roomName(state.RoomID),
			LastReflectedAt: formatTime(state.ReflectedAt),
			Status: RoomMemoryStatus{
				State:            s.Status,
				DueAt:            formatTime(s.DueAt),
				LatestActivityAt: formatTime(state.LatestAt),
				FailedAttempts:   state.FailedAttempts,
				LastAttemptAt:    formatTime(state.LastAttemptAt),
				LastOutcome:      state.LastOutcome,
				LastTraceID:      state.LastTraceID,
			},
			Memories: []RoomMemoryResponse{},
		}
		for _, memory := range byRoom[state.RoomID] {
			entry.Memories = append(entry.Memories, RoomMemoryResponse{
				ID:             memory.ID,
				Memory:         memory.Memory,
				CreatedAt:      memory.CreatedAt.UTC().Format(time.RFC3339),
				UpdatedAt:      memory.UpdatedAt.UTC().Format(time.RFC3339),
				CreatedTraceID: memory.CreatedTraceID,
				UpdatedTraceID: memory.UpdatedTraceID,
			})
		}
		entries = append(entries, entry)
	}

	sort.SliceStable(entries, func(a, b int) bool {
		return latest[entries[a].RoomID].After(latest[entries[b].RoomID])
	})
	return entries
}

// formatTime renders a time for the API, null for none
func formatTime(t *time.Time) *string {
	if t == nil {
		return nil
	}
	formatted := t.UTC().Format(time.RFC3339)
	return &formatted
}

// ReflectRoomHandler thinks back over a room's new conversations at once, whether or not the room is
// due a pass, and says how it went (admin only)
// POST /api/admin/rooms/:roomId/reflect
func ReflectRoomHandler(c *gin.Context) {
	ctx := c.Request.Context()

	roomID := c.Param("roomId")
	if roomID == "" {
		c.JSON(http.StatusBadRequest, ErrorResponse{Error: "Room ID is required"})
		return
	}

	// The pass goes on if the admin leaves the page: what it decides is saved either way
	result, err := chat.ReflectNow(context.WithoutCancel(ctx), roomID)
	if errors.Is(err, chat.ErrReflectionBusy) {
		c.JSON(http.StatusConflict, ErrorResponse{Error: "Another pass is running. Try again in a moment."})
		return
	}
	if err != nil {
		log.Error().Ctx(ctx).Err(err).Str("room_id", roomID).Msg("Failed to start a pass over a room")
		c.JSON(http.StatusInternalServerError, ErrorResponse{Error: "Failed to start the pass"})
		return
	}

	c.JSON(http.StatusOK, reflectResponse(result))
}

// reflectResponse is how a pass went, in its API form
func reflectResponse(result chat.ReflectionResult) ReflectResponse {
	return ReflectResponse{
		Outcome:                result.Outcome,
		Skipped:                result.Skipped,
		Failed:                 result.Failed,
		ConversationCount:      result.Conversations,
		ShownConversationCount: result.Shown,
		Added:                  result.Added,
		Updated:                result.Updated,
		Deleted:                result.Deleted,
		Rejected:               result.Rejected,
		TraceID:                result.TraceID,
	}
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
	if deleted == nil {
		c.JSON(http.StatusNotFound, ErrorResponse{Error: "The memory no longer exists"})
		return
	}

	metrics.RecordRoomMemoriesDeleted(1)
	logDeletedRoomMemory(c, *deleted)
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

	deleted, err := db.DeleteRoomMemories(ctx, roomID)
	if err != nil {
		log.Error().Ctx(ctx).Err(err).Str("room_id", roomID).Msg("Failed to delete room memories")
		c.JSON(http.StatusInternalServerError, ErrorResponse{Error: "Failed to delete the memories"})
		return
	}

	metrics.RecordRoomMemoriesDeleted(len(deleted))
	for _, memory := range deleted {
		logDeletedRoomMemory(c, memory)
	}
	log.Info().Ctx(ctx).Str("room_id", roomID).Int("deleted_count", len(deleted)).Msg("All of a room's memories deleted on the dashboard")
	c.JSON(http.StatusOK, DeleteRoomMemoriesResponse{DeletedCount: len(deleted)})
}

// logDeletedRoomMemory logs a memory the admin deleted on the dashboard, with what it said and the trace
// of the pass that last wrote it, so the deletion can be told apart from one a pass made
func logDeletedRoomMemory(c *gin.Context, memory db.RoomMemory) {
	event := log.Info().Ctx(c.Request.Context()).
		Str("room_id", memory.RoomID).
		Int64("memory_id", memory.ID).
		Str("memory", memory.Memory)
	if memory.UpdatedTraceID != nil {
		event = event.Str("memory_trace_id", *memory.UpdatedTraceID)
	}
	if userID, ok := GetUserIDFromContext(c); ok {
		event = event.Str("deleted_by", userID)
	}
	event.Msg("Room memory deleted on the dashboard")
}
