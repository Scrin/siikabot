package federation

import (
	"context"
	"fmt"
	"html"
	"sort"
	"strings"
	"time"

	"github.com/Scrin/siikabot/matrix"
	"github.com/rs/zerolog/log"
	"maunium.net/go/mautrix/id"
)

// serverInfo holds information about a server and its users
type serverInfo struct {
	name    string
	users   []string // Full user IDs
	version string
	err     error
}

// Handle handles the federation command
func Handle(ctx context.Context, roomID, msg string) {
	// Get all members in the room
	members, err := matrix.GetRoomMembers(ctx, roomID)
	if err != nil {
		log.Error().Ctx(ctx).Err(err).
			Str("room_id", roomID).
			Msg("Failed to get room members")
		matrix.SendNotice(ctx, roomID, "Failed to get room members")
		return
	}

	// Map to store users by homeserver
	usersByServer := make(map[string][]string)

	// Extract homeservers from member IDs and group users
	for _, member := range members {
		userID := id.UserID(member)
		if homeserver := userID.Homeserver(); homeserver != "" {
			// Get display name for the user
			displayName := matrix.GetDisplayName(ctx, member)
			if displayName == "" {
				displayName = string(userID.Localpart())
			}
			// Store the full user ID for proper linking later
			usersByServer[homeserver] = append(usersByServer[homeserver], member)
		}
	}

	// Send typing indicator while we check each homeserver
	matrix.SendTyping(ctx, roomID, true, 30*time.Second)
	defer matrix.SendTyping(ctx, roomID, false, 0)

	// Collect server information
	var servers []serverInfo
	for homeserver, users := range usersByServer {
		sort.Strings(users) // Sort users for consistent output
		info := serverInfo{
			name:  homeserver,
			users: users,
		}

		// Get server version
		version, err := matrix.GetServerVersion(ctx, homeserver)
		if err != nil {
			log.Error().Ctx(ctx).Err(err).
				Str("room_id", roomID).
				Str("homeserver", homeserver).
				Msg("Failed to get server version")
			info.err = err
		} else {
			info.version = version
		}
		servers = append(servers, info)
	}

	// Group servers by version
	serversByVersion := make(map[string][]serverInfo)
	var errorServers []serverInfo
	for _, server := range servers {
		if server.err != nil {
			errorServers = append(errorServers, server)
		} else {
			serversByVersion[server.version] = append(serversByVersion[server.version], server)
		}
	}

	// Get sorted list of versions
	var versions []string
	for version := range serversByVersion {
		versions = append(versions, version)
	}
	sort.Slice(versions, func(i, j int) bool {
		return versions[i] > versions[j]
	})

	// Build response message
	var response strings.Builder
	response.WriteString("<h3>Homeserver versions in this room:</h3>")

	// Add servers grouped by version
	for _, version := range versions {
		servers := serversByVersion[version]
		// Sort servers within this version
		sort.Slice(servers, func(i, j int) bool {
			return servers[i].name < servers[j].name
		})

		response.WriteString(fmt.Sprintf("<h4>%s</h4><ul>", html.EscapeString(version)))
		for _, server := range servers {
			// Convert user IDs to HTML links
			var userLinks []string
			for _, userID := range server.users {
				displayName := matrix.GetDisplayName(ctx, userID)
				if displayName == "" {
					displayName = string(id.UserID(userID).Localpart())
				}
				userLinks = append(userLinks, userLink(userID, displayName))
			}
			response.WriteString(fmt.Sprintf("<li>%s with %s</li>",
				html.EscapeString(server.name),
				strings.Join(userLinks, ", "),
			))
		}
		response.WriteString("</ul>")
	}

	// Add servers with errors at the end
	if len(errorServers) > 0 {
		response.WriteString("<h4>Servers with errors</h4>\n")
		sort.Slice(errorServers, func(i, j int) bool {
			return errorServers[i].name < errorServers[j].name
		})
		for _, server := range errorServers {
			response.WriteString(fmt.Sprintf("%s: %s<br>\n", html.EscapeString(server.name), html.EscapeString(fmt.Sprint(server.err))))
			// Convert user IDs to HTML links
			var userLinks []string
			for _, userID := range server.users {
				displayName := matrix.GetDisplayName(ctx, userID)
				if displayName == "" {
					displayName = string(id.UserID(userID).Localpart())
				}
				userLinks = append(userLinks, userLink(userID, displayName))
			}
			response.WriteString(fmt.Sprintf("Users: %s<br><br>\n", strings.Join(userLinks, ", ")))
		}
	}

	// Send the response
	matrix.SendFormattedNotice(ctx, roomID, response.String())
}

// userLink is a link to a user, named by their display name. Remote servers report the versions and errors,
// and people choose their display names, so all of it is escaped in the HTML.
func userLink(userID, displayName string) string {
	return fmt.Sprintf("<a href=\"https://matrix.to/#/%s\">%s</a>", html.EscapeString(userID), html.EscapeString(displayName))
}
