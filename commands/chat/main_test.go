package chat

import (
	"os"
	"testing"

	"github.com/Scrin/siikabot/config"
)

// testBotUserID is the bot's user ID in these tests. Headers, quotes and the room section of the
// system prompt all treat the bot specially, so it has to be set.
const testBotUserID = "@siikabot:example.com"

func TestMain(m *testing.M) {
	config.UserID = testBotUserID
	os.Exit(m.Run())
}
