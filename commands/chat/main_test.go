package chat

import (
	"os"
	"testing"

	"github.com/Scrin/siikabot/config"
)

// testBotUserID is the bot's user ID in these tests. Headers, quotes and the room section of the
// system prompt all treat the bot specially, so it has to be set.
const testBotUserID = "@siikabot:example.com"

// Where the bot's source code and web dashboard are in these tests
const (
	testSourceURL   = "https://git.example.org/siikabot"
	testFrontendURL = "https://siikabot.example.org/"
)

func TestMain(m *testing.M) {
	config.UserID = testBotUserID
	config.SourceURL = testSourceURL
	config.FrontendURL = testFrontendURL
	os.Exit(m.Run())
}
