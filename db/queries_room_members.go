package db

import (
	"context"
	"errors"
	"time"

	pgx "github.com/jackc/pgx/v5"
	"github.com/rs/zerolog/log"
	"maunium.net/go/mautrix/event"
	mid "maunium.net/go/mautrix/id"
)

// RoomMember is someone joined to, or invited into, a room the bot is in
type RoomMember struct {
	RoomID     string           `db:"room_id"`
	UserID     string           `db:"user_id"`
	Membership event.Membership `db:"membership"`
	// DisplayName is their display name in the room, nil if they have none
	DisplayName *string `db:"display_name"`
	// Since is when their current membership began, as far as the bot knows. A membership the bot
	// only learned about later, such as when the table is rebuilt, may have begun earlier.
	Since time.Time `db:"since"`
}

// isTracked reports whether a membership is one the table keeps
func isTracked(membership event.Membership) bool {
	return membership == event.MembershipJoin || membership == event.MembershipInvite
}

// upsertRoomMember records a joined or invited member. The time their membership began is kept
// through changes that don't change the membership itself, such as a new display name.
const upsertRoomMember = `INSERT INTO room_members (room_id, user_id, membership, display_name, since)
	VALUES ($1, $2, $3, $4, $5)
	ON CONFLICT (room_id, user_id) DO UPDATE SET
		membership = EXCLUDED.membership,
		display_name = EXCLUDED.display_name,
		since = CASE WHEN room_members.membership = EXCLUDED.membership THEN room_members.since ELSE EXCLUDED.since END`

// SetRoomMembership records a member's current membership of a room. Joined and invited members are
// kept, and anyone whose membership is anything else is removed.
func SetRoomMembership(ctx context.Context, member RoomMember) error {
	if !isTracked(member.Membership) {
		_, err := pool.Exec(ctx, "DELETE FROM room_members WHERE room_id = $1 AND user_id = $2",
			member.RoomID, member.UserID)
		if err != nil {
			log.Error().Ctx(ctx).Err(err).
				Str("room_id", member.RoomID).
				Str("user_id", member.UserID).
				Str("membership", string(member.Membership)).
				Msg("Failed to remove a room member")
		}
		return err
	}

	_, err := pool.Exec(ctx, upsertRoomMember,
		member.RoomID, member.UserID, member.Membership, member.DisplayName, member.Since)
	if err != nil {
		log.Error().Ctx(ctx).Err(err).
			Str("room_id", member.RoomID).
			Str("user_id", member.UserID).
			Str("membership", string(member.Membership)).
			Msg("Failed to save a room member")
	}
	return err
}

// ReplaceRoomMembers sets a room's members to the given ones, which is everyone the homeserver says
// has joined or is invited. Members whose membership hasn't changed keep the time it began.
//
// Returns whether the members the table had for the room may have had access they shouldn't have:
// someone was removed, or the table knew nobody in the room.
func ReplaceRoomMembers(ctx context.Context, roomID string, members []RoomMember) (bool, error) {
	tx, err := pool.Begin(ctx)
	if err != nil {
		log.Error().Ctx(ctx).Err(err).Str("room_id", roomID).Msg("Failed to begin transaction for replacing room members")
		return false, err
	}
	defer tx.Rollback(ctx)

	var known int
	if err := tx.QueryRow(ctx, "SELECT count(*) FROM room_members WHERE room_id = $1", roomID).Scan(&known); err != nil {
		log.Error().Ctx(ctx).Err(err).Str("room_id", roomID).Msg("Failed to count room members")
		return false, err
	}

	userIDs := make([]string, 0, len(members))
	for _, member := range members {
		if !isTracked(member.Membership) {
			continue
		}
		userIDs = append(userIDs, member.UserID)
		if _, err := tx.Exec(ctx, upsertRoomMember,
			roomID, member.UserID, member.Membership, member.DisplayName, member.Since); err != nil {
			log.Error().Ctx(ctx).Err(err).
				Str("room_id", roomID).
				Str("user_id", member.UserID).
				Msg("Failed to save a room member")
			return false, err
		}
	}

	removed, err := tx.Exec(ctx, "DELETE FROM room_members WHERE room_id = $1 AND user_id <> ALL($2)",
		roomID, userIDs)
	if err != nil {
		log.Error().Ctx(ctx).Err(err).Str("room_id", roomID).Msg("Failed to remove former room members")
		return false, err
	}

	if err := tx.Commit(ctx); err != nil {
		log.Error().Ctx(ctx).Err(err).Str("room_id", roomID).Msg("Failed to commit replacing room members")
		return false, err
	}
	return known == 0 || removed.RowsAffected() > 0, nil
}

// ForgetRoomMembers removes every member of a room, for a room the bot is no longer in
func ForgetRoomMembers(ctx context.Context, roomID string) error {
	_, err := pool.Exec(ctx, "DELETE FROM room_members WHERE room_id = $1", roomID)
	if err != nil {
		log.Error().Ctx(ctx).Err(err).Str("room_id", roomID).Msg("Failed to forget the members of a room")
	}
	return err
}

// KeepOnlyRooms removes the members of every room but the given ones, which are the rooms the bot
// is in
func KeepOnlyRooms(ctx context.Context, roomIDs []string) error {
	// A nil list would be NULL, which matches no row, rather than an empty list, which matches all
	if roomIDs == nil {
		roomIDs = []string{}
	}
	_, err := pool.Exec(ctx, "DELETE FROM room_members WHERE room_id <> ALL($1)", roomIDs)
	if err != nil {
		log.Error().Ctx(ctx).Err(err).Int("room_count", len(roomIDs)).Msg("Failed to remove the members of rooms the bot has left")
	}
	return err
}

// GetRoomMember returns someone's membership of a room, and whether they are joined or invited
func GetRoomMember(ctx context.Context, roomID, userID string) (RoomMember, bool, error) {
	rows, err := pool.Query(ctx,
		"SELECT room_id, user_id, membership, display_name, since FROM room_members WHERE room_id = $1 AND user_id = $2",
		roomID, userID)
	if err != nil {
		log.Error().Ctx(ctx).Err(err).Str("room_id", roomID).Str("user_id", userID).Msg("Failed to get a room member")
		return RoomMember{}, false, err
	}
	member, err := pgx.CollectExactlyOneRow(rows, pgx.RowToStructByName[RoomMember])
	if errors.Is(err, pgx.ErrNoRows) {
		return RoomMember{}, false, nil
	}
	if err != nil {
		log.Error().Ctx(ctx).Err(err).Str("room_id", roomID).Str("user_id", userID).Msg("Failed to read a room member")
		return RoomMember{}, false, err
	}
	return member, true, nil
}

// GetJoinedMembers returns the members who have joined a room, ordered by user ID
func GetJoinedMembers(ctx context.Context, roomID string) ([]RoomMember, error) {
	rows, err := pool.Query(ctx,
		`SELECT room_id, user_id, membership, display_name, since FROM room_members
		WHERE room_id = $1 AND membership = 'join' ORDER BY user_id`,
		roomID)
	if err != nil {
		log.Error().Ctx(ctx).Err(err).Str("room_id", roomID).Msg("Failed to get joined room members")
		return nil, err
	}
	members, err := pgx.CollectRows(rows, pgx.RowToStructByName[RoomMember])
	if err != nil {
		log.Error().Ctx(ctx).Err(err).Str("room_id", roomID).Msg("Failed to read joined room members")
		return nil, err
	}
	return members, nil
}

// FindJoinedRooms returns the rooms, among those the bot is in, that a user has joined, ordered by
// room ID. For the bot itself, that is every room it is in.
func FindJoinedRooms(ctx context.Context, userID string) ([]string, error) {
	rows, err := pool.Query(ctx,
		"SELECT room_id FROM room_members WHERE user_id = $1 AND membership = 'join' ORDER BY room_id",
		userID)
	if err != nil {
		log.Error().Ctx(ctx).Err(err).Str("user_id", userID).Msg("Failed to find joined rooms")
		return nil, err
	}
	roomIDs, err := pgx.CollectRows(rows, pgx.RowTo[string])
	if err != nil {
		log.Error().Ctx(ctx).Err(err).Str("user_id", userID).Msg("Failed to read joined rooms")
		return nil, err
	}
	return roomIDs, nil
}

// GetRoomMembers returns everyone joined to or invited into a room, the people its room keys are
// shared with
func GetRoomMembers(ctx context.Context, roomID mid.RoomID) ([]mid.UserID, error) {
	rows, err := pool.Query(ctx, "SELECT user_id FROM room_members WHERE room_id = $1", roomID)
	if err != nil {
		log.Error().Ctx(ctx).Err(err).Str("room_id", string(roomID)).Msg("Failed to get room members")
		return nil, err
	}
	members, err := pgx.CollectRows(rows, pgx.RowTo[mid.UserID])
	if err != nil {
		log.Error().Ctx(ctx).Err(err).Str("room_id", string(roomID)).Msg("Failed to read room members")
		return nil, err
	}
	return members, nil
}

// FindSharedRooms returns the rooms a user is joined to or invited into, among those the bot is in.
// The crypto machine rotates the room keys of these rooms when the user's devices change.
func FindSharedRooms(ctx context.Context, userID mid.UserID) ([]mid.RoomID, error) {
	rows, err := pool.Query(ctx, "SELECT room_id FROM room_members WHERE user_id = $1 ORDER BY room_id", userID)
	if err != nil {
		log.Error().Ctx(ctx).Err(err).Str("user_id", string(userID)).Msg("Failed to find shared rooms")
		return nil, err
	}
	roomIDs, err := pgx.CollectRows(rows, pgx.RowTo[mid.RoomID])
	if err != nil {
		log.Error().Ctx(ctx).Err(err).Str("user_id", string(userID)).Msg("Failed to read shared rooms")
		return nil, err
	}
	return roomIDs, nil
}
