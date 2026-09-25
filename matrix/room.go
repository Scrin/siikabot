package matrix

import (
	"context"
	"fmt"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/rs/zerolog/log"
	"maunium.net/go/mautrix/id"
)

// Member is a user joined to a room
type Member struct {
	UserID string
	// DisplayName is the member's name in this room, empty if they have none
	DisplayName string
}

// Room is what the chat feature knows about a room besides its messages: its name and who is in it
type Room struct {
	Name string
	// Members are the joined members, sorted by user ID
	Members []Member
}

// Member returns the joined member with the given user ID, if there is one
func (r Room) Member(userID string) (Member, bool) {
	i, found := slices.BinarySearchFunc(r.Members, userID, func(m Member, userID string) int {
		return strings.Compare(m.UserID, userID)
	})
	if !found {
		return Member{}, false
	}
	return r.Members[i], true
}

// roomTTL bounds how stale a cached room may be. Membership and name changes arriving through sync
// drop a room from the cache straight away, so this only heals whatever sync missed.
const roomTTL = 10 * time.Minute

type roomEntry struct {
	room      Room
	fetchedAt time.Time
}

var roomCache = struct {
	sync.Mutex
	entries map[string]roomEntry
}{entries: make(map[string]roomEntry)}

// GetRoom returns a room's name and joined members. It is cached until a membership or name change
// for the room arrives through sync.
func GetRoom(ctx context.Context, roomID string) (Room, error) {
	roomCache.Lock()
	entry, ok := roomCache.entries[roomID]
	roomCache.Unlock()
	if ok && time.Since(entry.fetchedAt) < roomTTL {
		return entry.room, nil
	}

	resp, err := client.JoinedMembers(ctx, id.RoomID(roomID))
	if err != nil {
		log.Error().Ctx(ctx).Err(err).Str("room_id", roomID).Msg("Failed to get joined members")
		return Room{}, fmt.Errorf("failed to get joined members: %w", err)
	}

	room := Room{Name: GetRoomName(ctx, roomID)}
	for userID, member := range resp.Joined {
		room.Members = append(room.Members, Member{UserID: userID.String(), DisplayName: member.DisplayName})
	}
	// Map iteration is random, and the member list goes into a prompt prefix that has to stay
	// byte-identical between turns to be served from the cache
	slices.SortFunc(room.Members, func(a, b Member) int {
		return strings.Compare(a.UserID, b.UserID)
	})

	roomCache.Lock()
	roomCache.entries[roomID] = roomEntry{room: room, fetchedAt: time.Now()}
	roomCache.Unlock()

	return room, nil
}

// invalidateRoom drops a room from the cache, so the next lookup fetches it again
func invalidateRoom(roomID string) {
	roomCache.Lock()
	delete(roomCache.entries, roomID)
	roomCache.Unlock()
}

// ResolveRoomAlias returns the room an alias points at
func ResolveRoomAlias(ctx context.Context, alias string) (string, error) {
	resp, err := client.ResolveAlias(ctx, id.RoomAlias(alias))
	if err != nil {
		log.Error().Ctx(ctx).Err(err).Str("room_alias", alias).Msg("Failed to resolve a room alias")
		return "", fmt.Errorf("failed to resolve room alias: %w", err)
	}
	return resp.RoomID.String(), nil
}
