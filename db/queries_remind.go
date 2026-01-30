package db

import (
	"context"
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
