package db

import (
	"context"

	"github.com/rs/zerolog/log"
	mid "maunium.net/go/mautrix/id"
)

// GetAllRooms returns all distinct room IDs known to the bot
func GetAllRooms(ctx context.Context) ([]mid.RoomID, error) {
	rows, err := pool.Query(ctx, "SELECT DISTINCT room_id FROM room_members ORDER BY room_id")
	if err != nil {
		log.Error().Ctx(ctx).Err(err).Msg("Failed to get all rooms")
		return nil, err
	}
	defer rows.Close()

	var roomIDs []mid.RoomID
	for rows.Next() {
		var roomID mid.RoomID
		if err := rows.Scan(&roomID); err != nil {
			log.Error().Ctx(ctx).Err(err).Msg("Failed to scan room ID")
			continue
		}
		roomIDs = append(roomIDs, roomID)
	}
	return roomIDs, nil
}
