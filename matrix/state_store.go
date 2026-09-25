package matrix

import (
	"context"
	"fmt"
	"time"

	"github.com/Scrin/siikabot/config"
	"github.com/Scrin/siikabot/db"
	"github.com/jackc/pgx/v5"
	"github.com/rs/zerolog/log"
	"maunium.net/go/mautrix"
	"maunium.net/go/mautrix/event"
	mid "maunium.net/go/mautrix/id"
)

type StateStore struct {
}

func NewStateStore() *StateStore {
	return &StateStore{}
}

func (store *StateStore) IsEncrypted(ctx context.Context, roomID mid.RoomID) (bool, error) {
	encryptionEvent, err := store.GetEncryptionEvent(ctx, roomID)
	if err == pgx.ErrNoRows {
		return false, nil
	} else if err != nil {
		return false, err
	}
	return encryptionEvent != nil, nil
}

func (store *StateStore) GetEncryptionEvent(ctx context.Context, roomId mid.RoomID) (*event.EncryptionEventContent, error) {
	return db.GetEncryptionEvent(ctx, roomId)
}

func (store *StateStore) SetEncryptionEvent(ctx context.Context, event *event.Event) error {
	return db.SaveEncryptionEvent(ctx, event.RoomID, &event.Content)
}

func (store *StateStore) FindSharedRooms(ctx context.Context, userId mid.UserID) ([]mid.RoomID, error) {
	return db.FindSharedRooms(ctx, userId)
}

// SetMembership records a membership event from sync: the member it is about, their membership and
// their display name. The members of a room the bot leaves are forgotten.
//
// Only rooms the bot has joined are recorded. The state of a room it is merely invited to arrives
// stripped, without the times the table keeps.
func (store *StateStore) SetMembership(ctx context.Context, evt *event.Event) error {
	// A state event in the timeline that the sync marks as superseded by the room's state after it
	if evt.Mautrix.IgnoreState {
		return nil
	}

	roomID := evt.RoomID.String()
	userID := evt.GetStateKey()
	content := evt.Content.AsMember()

	if userID == config.UserID && (content.Membership == event.MembershipLeave || content.Membership == event.MembershipBan) {
		return db.ForgetRoomMembers(ctx, roomID)
	}
	if evt.Mautrix.EventSource&event.SourceJoin == 0 {
		return nil
	}
	return db.SetRoomMembership(ctx, roomMember(roomID, userID, content, evt.Timestamp))
}

// GetRoomMembers returns everyone joined to or invited into a room, the people its room keys are
// shared with
func (store *StateStore) GetRoomMembers(ctx context.Context, roomId mid.RoomID) ([]mid.UserID, error) {
	return db.GetRoomMembers(ctx, roomId)
}

// roomMember reads a membership event as the member it records
func roomMember(roomID, userID string, content *event.MemberEventContent, timestamp int64) db.RoomMember {
	member := db.RoomMember{
		RoomID:     roomID,
		UserID:     userID,
		Membership: content.Membership,
		// An event without a time counts as happening now, which errs towards a membership that
		// began later than it really did
		Since: time.Now(),
	}
	if timestamp > 0 {
		member.Since = time.UnixMilli(timestamp)
	}
	if content.Displayname != "" {
		member.DisplayName = &content.Displayname
	}
	return member
}

// RebuildRoomMembers replaces what the state store knows about the members of the bot's rooms with
// what the homeserver says, and forgets the rooms the bot is no longer in. Sync only tells the bot
// about changes, so this catches up on those it missed. A room that turns out to have lost members
// gets a new room key for its next message.
//
// It has to finish before anything encrypted is sent: room keys are shared with the members the
// store lists.
func RebuildRoomMembers(ctx context.Context) error {
	joined, err := client.JoinedRooms(ctx)
	if err != nil {
		log.Error().Ctx(ctx).Err(err).Msg("Failed to get the joined rooms to rebuild room members")
		return fmt.Errorf("failed to get joined rooms: %w", err)
	}

	roomIDs := make([]string, 0, len(joined.JoinedRooms))
	for _, roomID := range joined.JoinedRooms {
		roomIDs = append(roomIDs, roomID.String())

		// Leavers aren't needed, and a room with a long history has many of them
		resp, err := client.Members(ctx, roomID, mautrix.ReqMembers{NotMembership: event.MembershipLeave})
		if err != nil {
			// The room keeps what the store already had, which sync has kept up to date since
			log.Error().Ctx(ctx).Err(err).Str("room_id", roomID.String()).Msg("Failed to get room members to rebuild them")
			continue
		}

		members := make([]db.RoomMember, 0, len(resp.Chunk))
		for _, evt := range resp.Chunk {
			if evt.Type != event.StateMember {
				continue
			}
			members = append(members, roomMember(roomID.String(), evt.GetStateKey(), evt.Content.AsMember(), evt.Timestamp))
		}
		lostAccess, err := db.ReplaceRoomMembers(ctx, roomID.String(), members)
		if err != nil {
			// Already logged, and the room keeps what the store had
			continue
		}

		// The room's current key may have gone to someone who isn't a member any more, or, before
		// the store tracked members properly, to anyone at all. The next message then starts a new
		// key, which goes only to the members as they are now.
		if lostAccess {
			if err := olmMachine.CryptoStore.RemoveOutboundGroupSession(ctx, roomID); err != nil {
				log.Error().Ctx(ctx).Err(err).Str("room_id", roomID.String()).Msg("Failed to discard the room key after rebuilding room members")
			}
		}
	}

	if err := db.KeepOnlyRooms(ctx, roomIDs); err != nil {
		return fmt.Errorf("failed to forget left rooms: %w", err)
	}
	log.Info().Ctx(ctx).Int("room_count", len(roomIDs)).Msg("Rebuilt room members")
	return nil
}
