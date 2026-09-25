package llmtools

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/Scrin/siikabot/aigateway"
	"github.com/rs/zerolog/log"
)

// maxMemberMatches caps how many members one lookup lists
const maxMemberMatches = 20

// RoomMember is a member of the room a chat turn happens in, as the chat turn hands the room's
// members to the member lookup tool
type RoomMember struct {
	UserID string
	// Name is the name the model knows them by, their user ID if they have none
	Name string
	// IsBot marks the bot itself
	IsBot bool
}

// RoomMembersToolDefinition finds members of the room a chat turn happens in. A room too large for
// its members to be listed in full shows the model only the ones relevant to each message, and
// anyone else a message refers to by name can be looked up with it.
//
// It sees the members of that one room only, which the chat turn puts in the context, so that no
// lookup reaches into another room the bot is in.
var RoomMembersToolDefinition = aigateway.ToolDefinition{
	Type: "function",
	Function: aigateway.FunctionSchema{
		Name:        "find_room_members",
		Description: "Find members of this room by part of their name or user ID. Use it in a room too large for all of its members to be listed for you, to find someone a message refers to.",
		Parameters: json.RawMessage(`{
			"type": "object",
			"properties": {
				"query": {
					"type": "string",
					"description": "Part of the display name or user ID to look for, case insensitively"
				}
			},
			"required": ["query"]
		}`),
	},
	Handler: handleRoomMembersToolCall,
	// Membership changes, but slowly
	ValidityDuration: time.Hour,
}

func handleRoomMembersToolCall(ctx context.Context, arguments string) (string, error) {
	var args struct {
		Query string `json:"query"`
	}

	log.Debug().Ctx(ctx).Str("arguments", arguments).Msg("Received room members tool call")

	if err := json.Unmarshal([]byte(arguments), &args); err != nil {
		log.Error().Ctx(ctx).Err(err).Str("arguments", arguments).Msg("Failed to parse room members tool arguments")
		return "", fmt.Errorf("failed to parse arguments: %w", err)
	}
	query := strings.TrimSpace(args.Query)
	if query == "" {
		return "", errors.New("query is required")
	}

	members, ok := ctx.Value("room_members").([]RoomMember)
	if !ok {
		log.Error().Ctx(ctx).Msg("No room members in the tool context")
		return "", errors.New("the members of this room aren't known")
	}

	return findMembers(members, query), nil
}

// findMembers lists the members whose name or user ID contains the query, case insensitively
func findMembers(members []RoomMember, query string) string {
	query = strings.ToLower(query)
	var matches []RoomMember
	for _, member := range members {
		if strings.Contains(strings.ToLower(member.Name), query) || strings.Contains(strings.ToLower(member.UserID), query) {
			matches = append(matches, member)
		}
	}

	if len(matches) == 0 {
		return fmt.Sprintf("No member of this room matches %q.", query)
	}

	var b strings.Builder
	fmt.Fprintf(&b, "Members of this room matching %q:\n", query)
	for i, member := range matches {
		if i == maxMemberMatches {
			fmt.Fprintf(&b, "(and %d more: ask with a longer query to narrow it down)\n", len(matches)-maxMemberMatches)
			break
		}
		line := "- " + member.Name
		if member.Name != member.UserID {
			line += " (" + member.UserID + ")"
		}
		if member.IsBot {
			line += " (you)"
		}
		b.WriteString(line + "\n")
	}
	return strings.TrimSuffix(b.String(), "\n")
}
