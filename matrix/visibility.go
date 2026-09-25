package matrix

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/Scrin/siikabot/db"
	"github.com/rs/zerolog/log"
	"maunium.net/go/mautrix"
	"maunium.net/go/mautrix/event"
	"maunium.net/go/mautrix/id"
)

// Not every member of a room can see all of its history. In a room that shows its history to
// members only from when they joined, or were invited, a reply or a link can still point at an older
// message: a client can be made to send one by hand. The bot would then read that message out to
// someone who can't see it themselves. So before a message someone refers to is used, VisibleTo
// checks that they could see it.

// historyVisibilityTTL bounds how stale a cached history visibility may be. A change arriving
// through sync drops it from the cache straight away, so this only heals whatever sync missed.
const historyVisibilityTTL = 10 * time.Minute

type historyVisibilityEntry struct {
	visibility event.HistoryVisibility
	fetchedAt  time.Time
}

var historyVisibilityCache = struct {
	sync.Mutex
	entries map[string]historyVisibilityEntry
}{entries: make(map[string]historyVisibilityEntry)}

// historyVisibility returns who a room shows its history to, cached until a change arrives through
// sync. A room that has never set it shows its history to all members, as the spec says.
func historyVisibility(ctx context.Context, roomID string) (event.HistoryVisibility, error) {
	historyVisibilityCache.Lock()
	entry, ok := historyVisibilityCache.entries[roomID]
	historyVisibilityCache.Unlock()
	if ok && time.Since(entry.fetchedAt) < historyVisibilityTTL {
		return entry.visibility, nil
	}

	var content event.HistoryVisibilityEventContent
	err := client.StateEvent(ctx, id.RoomID(roomID), event.StateHistoryVisibility, "", &content)
	switch {
	case errors.Is(err, mautrix.MNotFound):
		content.HistoryVisibility = event.HistoryVisibilityShared
	case err != nil:
		log.Error().Ctx(ctx).Err(err).Str("room_id", roomID).Msg("Failed to get the history visibility of a room")
		return "", fmt.Errorf("failed to get history visibility: %w", err)
	}

	historyVisibilityCache.Lock()
	historyVisibilityCache.entries[roomID] = historyVisibilityEntry{visibility: content.HistoryVisibility, fetchedAt: time.Now()}
	historyVisibilityCache.Unlock()
	return content.HistoryVisibility, nil
}

// invalidateHistoryVisibility drops a room's history visibility from the cache
func invalidateHistoryVisibility(roomID string) {
	historyVisibilityCache.Lock()
	delete(historyVisibilityCache.entries, roomID)
	historyVisibilityCache.Unlock()
}

// VisibleTo reports whether a member of a room can see a message sent there at sentAt. When it
// can't tell, the answer is no.
//
// The check is imperfect in the safe direction: the bot knows when someone's current membership
// began, which for a membership it only learned about later may be after it really did.
func VisibleTo(ctx context.Context, roomID, userID string, sentAt time.Time) bool {
	visibility, err := historyVisibility(ctx, roomID)
	if err != nil {
		// Already logged
		return false
	}
	if historyIsShared(visibility) {
		return true
	}

	member, isMember, err := db.GetRoomMember(ctx, roomID, userID)
	if err != nil {
		// Already logged
		return false
	}
	visible := isMember && !sentAt.Before(member.Since)
	if !visible {
		log.Debug().Ctx(ctx).
			Str("room_id", roomID).
			Str("user_id", userID).
			Str("history_visibility", string(visibility)).
			Bool("is_member", isMember).
			Time("sent_at", sentAt).
			Time("member_since", member.Since).
			Msg("A referenced message is older than the member's membership")
	}
	return visible
}

// historyIsShared reports whether a room shows members all of its history, whenever they joined.
// A setting the bot doesn't know shows them none of it before they joined.
func historyIsShared(visibility event.HistoryVisibility) bool {
	return visibility == event.HistoryVisibilityShared || visibility == event.HistoryVisibilityWorldReadable
}
