package config

import (
	"context"
	"fmt"
	"strings"

	"github.com/Scrin/siikabot/config"
	"github.com/Scrin/siikabot/db"
	"github.com/Scrin/siikabot/matrix"
	"github.com/rs/zerolog/log"
)

func Handle(ctx context.Context, roomID, senderID, msg string) {
	if senderID != config.Admin {
		matrix.SendNotice(roomID, "You don't have permission to use this command")
		return
	}

	args := strings.Fields(msg)
	if len(args) < 2 {
		matrix.SendNotice(roomID, "Usage: !config command <enable|disable> <command>")
		return
	}

	switch args[1] {
	case "command":
		handleCommand(ctx, roomID, senderID, args[2:])
	default:
		matrix.SendNotice(roomID, "Unknown config type. Available types: command")
	}
}

func handleCommand(ctx context.Context, roomID, senderID string, args []string) {
	if len(args) < 2 {
		matrix.SendNotice(roomID, "Usage: !config command <enable|disable> <command>")
		return
	}

	action := args[0]
	cmdName := args[1]
	if !strings.HasPrefix(cmdName, "!") {
		cmdName = "!" + cmdName
	}

	switch action {
	case "enable":
		enabled, err := db.IsCommandEnabled(ctx, roomID, cmdName)
		if err != nil {
			log.Error().Ctx(ctx).Err(err).Str("room_id", roomID).Str("command", cmdName).Msg("Failed to check command status")
			matrix.SendNotice(roomID, "Failed to check command status")
			return
		}
		if enabled {
			matrix.SendNotice(roomID, fmt.Sprintf("Command %s is already enabled", cmdName))
			return
		}
		if err := db.SetCommandEnabled(ctx, roomID, cmdName, true); err != nil {
			log.Error().Ctx(ctx).Err(err).Str("room_id", roomID).Str("command", cmdName).Msg("Failed to enable command")
			matrix.SendNotice(roomID, "Failed to enable command")
			return
		}
		matrix.SendNotice(roomID, fmt.Sprintf("Command %s has been enabled", cmdName))

	case "disable":
		enabled, err := db.IsCommandEnabled(ctx, roomID, cmdName)
		if err != nil {
			log.Error().Ctx(ctx).Err(err).Str("room_id", roomID).Str("command", cmdName).Msg("Failed to check command status")
			matrix.SendNotice(roomID, "Failed to check command status")
			return
		}
		if !enabled {
			matrix.SendNotice(roomID, fmt.Sprintf("Command %s is already disabled", cmdName))
			return
		}
		if err := db.SetCommandEnabled(ctx, roomID, cmdName, false); err != nil {
			log.Error().Ctx(ctx).Err(err).Str("room_id", roomID).Str("command", cmdName).Msg("Failed to disable command")
			matrix.SendNotice(roomID, "Failed to disable command")
			return
		}
		matrix.SendNotice(roomID, fmt.Sprintf("Command %s has been disabled", cmdName))

	default:
		matrix.SendNotice(roomID, "Invalid action. Use 'enable' or 'disable'")
	}
}
