package api

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/Scrin/siikabot/config"
	"github.com/Scrin/siikabot/db"
	"github.com/Scrin/siikabot/webhooks"
)

const (
	alertsRoom = "!alerts:example.com"
	leftRoom   = "!left:example.com"
	goodToken  = "correct-horse-battery-staple"
)

var botJoined = map[string]bool{alertsRoom: true, "!other:example.com": true}

func TestWebhookResponsesSayWhereEachWebhookPostsAndWhetherItCan(t *testing.T) {
	saved := config.FrontendURL
	config.FrontendURL = "https://siikabot.example.com/"
	defer func() { config.FrontendURL = saved }()

	created := time.Date(2026, 10, 28, 9, 30, 0, 0, time.FixedZone("EET", 2*60*60))
	used := time.Date(2026, 10, 28, 12, 0, 0, 0, time.UTC)
	all := []db.Webhook{
		{ID: "abc", Name: "hermes", RoomID: alertsRoom, TokenHash: webhooks.HashToken(goodToken), CreatedAt: created, LastUsedAt: &used},
		{ID: "def", Name: "backups", RoomID: alertsRoom, CreatedAt: created},
		{ID: "ghi", Name: "old", RoomID: leftRoom, CreatedAt: created},
	}

	lookups := map[string]int{}
	responses := webhookResponses(all, botJoined, func(roomID string) string {
		lookups[roomID]++
		if roomID == alertsRoom {
			return "Alerts"
		}
		return ""
	})

	hermes := responses[0]
	if hermes.URL != "https://siikabot.example.com/hooks/message/abc" {
		t.Errorf("URL = %q", hermes.URL)
	}
	if hermes.RoomName != "Alerts" || !hermes.BotInRoom {
		t.Errorf("hermes reads as %#v, want its room's name and the bot in it", hermes)
	}
	if hermes.CreatedAt != "2026-10-28T07:30:00Z" {
		t.Errorf("CreatedAt = %q, want UTC", hermes.CreatedAt)
	}
	if hermes.LastUsedAt == nil || *hermes.LastUsedAt != "2026-10-28T12:00:00Z" {
		t.Errorf("LastUsedAt = %v", hermes.LastUsedAt)
	}
	if responses[1].LastUsedAt != nil {
		t.Errorf("a webhook never used reads as last used %q", *responses[1].LastUsedAt)
	}
	if responses[2].BotInRoom || responses[2].RoomName != "" {
		t.Errorf("a webhook in a room the bot left reads as %#v", responses[2])
	}
	if lookups[alertsRoom] != 1 || lookups[leftRoom] != 1 {
		t.Errorf("room names looked up %v, want each room once", lookups)
	}
}

func TestWebhookURLPathEscapesTheID(t *testing.T) {
	if got := webhookURL("https://bot.example.com", "a/b"); got != "https://bot.example.com/hooks/message/a%2Fb" {
		t.Errorf("webhookURL = %q", got)
	}
}

func TestNewWebhookChecksEveryField(t *testing.T) {
	webhook, err := newWebhook(CreateWebhookRequest{Name: "  hermes ", RoomID: alertsRoom, Token: goodToken}, botJoined)
	if err != nil {
		t.Fatal(err)
	}
	if webhook.Name != "hermes" || webhook.RoomID != alertsRoom || !webhooks.TokenMatches(webhook.TokenHash, goodToken) {
		t.Errorf("webhook = %#v", webhook)
	}

	tests := []struct {
		name string
		req  CreateWebhookRequest
		err  error
	}{
		{"no name", CreateWebhookRequest{Name: " ", RoomID: alertsRoom, Token: goodToken}, webhooks.ErrEmptyName},
		{"two-line name", CreateWebhookRequest{Name: "a\nb", RoomID: alertsRoom, Token: goodToken}, webhooks.ErrNameNotOneLine},
		{"room the bot isn't in", CreateWebhookRequest{Name: "hermes", RoomID: leftRoom, Token: goodToken}, errRoomNotJoined},
		{"no room", CreateWebhookRequest{Name: "hermes", Token: goodToken}, errRoomNotJoined},
		{"short token", CreateWebhookRequest{Name: "hermes", RoomID: alertsRoom, Token: "short"}, webhooks.ErrTokenLength},
		{"no token", CreateWebhookRequest{Name: "hermes", RoomID: alertsRoom}, webhooks.ErrTokenLength},
		{"token with a space", CreateWebhookRequest{Name: "hermes", RoomID: alertsRoom, Token: "correct horse battery"}, webhooks.ErrTokenCharacters},
	}
	for _, tt := range tests {
		if _, err := newWebhook(tt.req, botJoined); !errors.Is(err, tt.err) {
			t.Errorf("%s: err = %v, want %v", tt.name, err, tt.err)
		}
	}
}

func TestWebhookChangesChecksOnlyWhatChanges(t *testing.T) {
	name, room, token := " hermes 2 ", "!other:example.com", goodToken
	changes, err := webhookChanges(UpdateWebhookRequest{Name: &name, RoomID: &room, Token: &token}, botJoined)
	if err != nil {
		t.Fatal(err)
	}
	if *changes.Name != "hermes 2" || *changes.RoomID != room || !webhooks.TokenMatches(changes.TokenHash, goodToken) {
		t.Errorf("changes = %#v", changes)
	}

	// Renaming a webhook whose room the bot has left leaves the room out of it
	changes, err = webhookChanges(UpdateWebhookRequest{Name: &name}, botJoined)
	if err != nil {
		t.Fatalf("renaming alone failed: %v", err)
	}
	if changes.RoomID != nil || changes.TokenHash != nil {
		t.Errorf("renaming alone changes %#v", changes)
	}

	left, short, blank := leftRoom, "short", ""
	tests := []struct {
		name string
		req  UpdateWebhookRequest
		err  error
	}{
		{"nothing", UpdateWebhookRequest{}, errNoChanges},
		{"blank name", UpdateWebhookRequest{Name: &blank}, webhooks.ErrEmptyName},
		{"room the bot isn't in", UpdateWebhookRequest{RoomID: &left}, errRoomNotJoined},
		{"short token", UpdateWebhookRequest{Token: &short}, webhooks.ErrTokenLength},
		{"empty token", UpdateWebhookRequest{Token: &blank}, webhooks.ErrTokenLength},
	}
	for _, tt := range tests {
		if _, err := webhookChanges(tt.req, botJoined); !errors.Is(err, tt.err) {
			t.Errorf("%s: err = %v, want %v", tt.name, err, tt.err)
		}
	}
}

func TestErrorFromWritesASentence(t *testing.T) {
	if got := ErrorFrom(errors.New("the name can't be empty")).Error; got != "The name can't be empty" {
		t.Errorf("ErrorFrom = %q", got)
	}
	if got := ErrorFrom(errors.New("ä is first")).Error; !strings.HasPrefix(got, "Ä") {
		t.Errorf("ErrorFrom = %q, want the first letter capitalized", got)
	}
}
