package db

import (
	"context"
	"errors"
	"time"

	pgx "github.com/jackc/pgx/v5"
	"github.com/rs/zerolog/log"
)

type Reminder struct {
	ID         int64     `db:"id"`
	RemindTime time.Time `db:"remind_time"`
	UserID     string    `db:"user_id"`
	RoomID     string    `db:"room_id"`
	Message    string    `db:"message"`
}

func GetReminders(ctx context.Context) ([]Reminder, error) {
	rows, err := pool.Query(ctx, "SELECT id, remind_time, user_id, room_id, message FROM reminders")
	if err != nil {
		log.Error().Ctx(ctx).Err(err).Msg("Failed to query reminders")
		return nil, err
	}
	return pgx.CollectRows(rows, pgx.RowToStructByName[Reminder])
}

func AddReminder(ctx context.Context, reminder Reminder) (int64, error) {
	var id int64
	err := pool.QueryRow(ctx,
		"INSERT INTO reminders (remind_time, user_id, room_id, message) VALUES ($1, $2, $3, $4) RETURNING id",
		reminder.RemindTime, reminder.UserID, reminder.RoomID, reminder.Message).Scan(&id)
	if err != nil {
		log.Error().Ctx(ctx).Err(err).
			Time("remind_time", reminder.RemindTime).
			Str("user_id", reminder.UserID).
			Str("room_id", reminder.RoomID).
			Msg("Failed to insert reminder")
		return 0, err
	}
	return id, nil
}

func RemoveReminder(ctx context.Context, id int64) error {
	_, err := pool.Exec(ctx, "DELETE FROM reminders WHERE id = $1", id)
	if err != nil {
		log.Error().Ctx(ctx).Err(err).Int64("id", id).Msg("Failed to delete reminder")
		return err
	}
	return nil
}

func GetRemindersByUserID(ctx context.Context, userID string) ([]Reminder, error) {
	rows, err := pool.Query(ctx,
		"SELECT id, remind_time, user_id, room_id, message FROM reminders WHERE user_id = $1 AND remind_time > NOW() ORDER BY remind_time ASC",
		userID)
	if err != nil {
		log.Error().Ctx(ctx).Err(err).Str("user_id", userID).Msg("Failed to query reminders by user ID")
		return nil, err
	}
	return pgx.CollectRows(rows, pgx.RowToStructByName[Reminder])
}

// MaxListedReminders caps how many reminders GetUpcomingRemindersIn returns
const MaxListedReminders = 50

// GetUpcomingRemindersIn returns the reminders userID set in roomID that are still to go off,
// soonest first
func GetUpcomingRemindersIn(ctx context.Context, userID, roomID string) ([]Reminder, error) {
	rows, err := pool.Query(ctx,
		`SELECT id, remind_time, user_id, room_id, message FROM reminders
		WHERE user_id = $1 AND room_id = $2 AND remind_time > NOW()
		ORDER BY remind_time ASC LIMIT $3`,
		userID, roomID, MaxListedReminders)
	if err != nil {
		log.Error().Ctx(ctx).Err(err).Str("user_id", userID).Str("room_id", roomID).Msg("Failed to query reminders in room")
		return nil, err
	}
	return pgx.CollectRows(rows, pgx.RowToStructByName[Reminder])
}

// ReminderScope is whose reminders, and from which room, someone may delete. The chat always gives
// the room its turn happens in, since it may act only on reminders set in that room. The dashboard
// leaves RoomID empty, since people manage their own reminders in every room there.
type ReminderScope struct {
	UserID string
	RoomID string
}

// DeleteReminder deletes the reminder with the given ID if it is within scope, and returns it. It
// returns nil if there was no such reminder.
func DeleteReminder(ctx context.Context, scope ReminderScope, id int64) (*Reminder, error) {
	rows, err := pool.Query(ctx,
		`DELETE FROM reminders WHERE id = $1 AND user_id = $2 AND ($3::text = '' OR room_id = $3)
		RETURNING id, remind_time, user_id, room_id, message`,
		id, scope.UserID, scope.RoomID)
	if err != nil {
		log.Error().Ctx(ctx).Err(err).Int64("id", id).Str("user_id", scope.UserID).Str("room_id", scope.RoomID).Msg("Failed to delete reminder")
		return nil, err
	}
	rem, err := pgx.CollectExactlyOneRow(rows, pgx.RowToStructByName[Reminder])
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		log.Error().Ctx(ctx).Err(err).Int64("id", id).Str("user_id", scope.UserID).Str("room_id", scope.RoomID).Msg("Failed to read deleted reminder")
		return nil, err
	}
	return &rem, nil
}
