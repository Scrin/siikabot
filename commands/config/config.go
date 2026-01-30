package config

import (
	"context"
	"fmt"
	"slices"
	"strings"

	"github.com/Scrin/siikabot/config"
	"github.com/Scrin/siikabot/db"
	"github.com/Scrin/siikabot/matrix"
	"github.com/rs/zerolog/log"
)

// Handle handles the !config command (admin-only)
// mentionedUsers contains user IDs extracted from m.mentions in the event
func Handle(ctx context.Context, roomID, senderID, msg string, mentionedUsers []string) {
	if senderID != config.Admin {
		matrix.SendNotice(roomID, "You don't have permission to use this command")
		return
	}

	args := strings.Fields(msg)
	if len(args) < 2 {
		showHelp(roomID)
		return
	}

	switch args[1] {
	case "help":
		showHelp(roomID)
	case "command":
		handleCommand(ctx, roomID, args[2:])
	case "user":
		handleUser(ctx, roomID, args[2:], mentionedUsers)
	default:
		matrix.SendNotice(roomID, "Unknown config type. Use !config help for usage.")
	}
}

func showHelp(roomID string) {
	help := `Usage:
!config command <enable|disable> <command> - Enable or disable a command in this room
!config user list - List all users with their authorizations
!config user show <user-id> - Show information about a specific user
!config user authorize <user-id> <feature> - Grant feature access to a user
!config user unauthorize <user-id> <feature> - Revoke feature access from a user

Supported features: ` + strings.Join(db.GetSupportedFeatures(), ", ")

	matrix.SendNotice(roomID, help)
}

func handleCommand(ctx context.Context, roomID string, args []string) {
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

func handleUser(ctx context.Context, roomID string, args []string, mentionedUsers []string) {
	if len(args) < 1 {
		matrix.SendNotice(roomID, "Usage: !config user <list|show|authorize|unauthorize> ...")
		return
	}

	switch args[0] {
	case "list":
		handleUserList(ctx, roomID)
	case "show":
		handleUserShow(ctx, roomID, args[1:], mentionedUsers)
	case "authorize":
		handleUserAuthorize(ctx, roomID, args[1:], true, mentionedUsers)
	case "unauthorize":
		handleUserAuthorize(ctx, roomID, args[1:], false, mentionedUsers)
	default:
		matrix.SendNotice(roomID, "Unknown user action. Use: list, show, authorize, unauthorize")
	}
}

func handleUserList(ctx context.Context, roomID string) {
	users, err := db.GetAllUsersWithAuthorizations(ctx)
	if err != nil {
		log.Error().Ctx(ctx).Err(err).Msg("Failed to list user authorizations")
		matrix.SendNotice(roomID, "Failed to list user authorizations")
		return
	}

	if len(users) == 0 {
		matrix.SendNotice(roomID, "No users with authorizations found")
		return
	}

	var sb strings.Builder
	sb.WriteString("User authorizations:\n")
	for _, u := range users {
		sb.WriteString(fmt.Sprintf("• %s - grafana: %s\n", u.UserID, boolToYesNo(u.Grafana)))
	}

	matrix.SendNotice(roomID, sb.String())
}

func handleUserShow(ctx context.Context, roomID string, args []string, mentionedUsers []string) {
	if len(args) < 1 {
		matrix.SendNotice(roomID, "Usage: !config user show <user-id>")
		return
	}

	userID := resolveUserID(args[0], mentionedUsers)
	if userID == "" {
		matrix.SendNotice(roomID, "Invalid user ID format. Expected: @user:domain.com or a mention")
		return
	}

	auth, err := db.GetUserAuthorizations(ctx, userID)
	if err != nil {
		log.Error().Ctx(ctx).Err(err).Str("user_id", userID).Msg("Failed to get user authorizations")
		matrix.SendNotice(roomID, "Failed to get user information")
		return
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("User: %s\n", userID))
	sb.WriteString(fmt.Sprintf("Is admin: %s\n", boolToYesNo(userID == config.Admin)))
	sb.WriteString("Authorizations:\n")
	sb.WriteString(fmt.Sprintf("  • grafana: %s", boolToYesNo(auth.Grafana)))

	matrix.SendNotice(roomID, sb.String())
}

func handleUserAuthorize(ctx context.Context, roomID string, args []string, authorize bool, mentionedUsers []string) {
	if len(args) < 2 {
		action := "authorize"
		if !authorize {
			action = "unauthorize"
		}
		matrix.SendNotice(roomID, fmt.Sprintf("Usage: !config user %s <user-id> <feature>", action))
		return
	}

	userID := resolveUserID(args[0], mentionedUsers)
	feature := strings.ToLower(args[1])

	if userID == "" {
		matrix.SendNotice(roomID, "Invalid user ID format. Expected: @user:domain.com or a mention")
		return
	}

	// Validate feature
	supportedFeatures := db.GetSupportedFeatures()
	if !slices.Contains(supportedFeatures, feature) {
		matrix.SendNotice(roomID, fmt.Sprintf("Unknown feature '%s'. Supported features: %s", feature, strings.Join(supportedFeatures, ", ")))
		return
	}

	err := db.SetUserAuthorization(ctx, userID, feature, authorize)
	if err != nil {
		log.Error().Ctx(ctx).Err(err).
			Str("user_id", userID).
			Str("feature", feature).
			Bool("authorize", authorize).
			Msg("Failed to update user authorization")
		matrix.SendNotice(roomID, "Failed to update user authorization")
		return
	}

	action := "authorized"
	if !authorize {
		action = "unauthorized"
	}
	matrix.SendNotice(roomID, fmt.Sprintf("User %s %s for %s", userID, action, feature))
}

func boolToYesNo(b bool) string {
	if b {
		return "yes"
	}
	return "no"
}

// resolveUserID resolves a user ID from either a direct @user:domain format
// or from the mentioned users list (when a Matrix client sends a formatted mention)
func resolveUserID(arg string, mentionedUsers []string) string {
	// If the argument is already a valid user ID, use it directly
	if strings.HasPrefix(arg, "@") && strings.Contains(arg, ":") {
		return arg
	}

	// Otherwise, try to use the first mentioned user
	// (when a client sends a mention, the body contains the display name
	// but the actual user ID is in m.mentions.user_ids)
	if len(mentionedUsers) > 0 {
		return mentionedUsers[0]
	}

	return ""
}
