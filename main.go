package main

import (
	"os"
	"strings"

	"github.com/Scrin/siikabot/bot"
	"github.com/Scrin/siikabot/logging"
	"github.com/rs/zerolog/log"
)

func main() {
	logging.Setup()

	homeserverURL := ""
	userID := ""
	accessToken := ""
	hookSecret := ""
	dataPath := ""
	admin := ""
	openrouterAPIKey := ""
	postgresConnectionString := ""

	for _, e := range os.Environ() {
		split := strings.SplitN(e, "=", 2)
		switch split[0] {
		case "SIIKABOT_HOMESERVER_URL":
			homeserverURL = split[1]
		case "SIIKABOT_USER_ID":
			userID = split[1]
		case "SIIKABOT_ACCESS_TOKEN":
			accessToken = split[1]
		case "SIIKABOT_HOOK_SECRET":
			hookSecret = split[1]
		case "SIIKABOT_DATA_PATH":
			dataPath = split[1]
		case "SIIKABOT_ADMIN":
			admin = split[1]
		case "SIIKABOT_OPENROUTER_API_KEY":
			openrouterAPIKey = split[1]
		case "SIIKABOT_POSTGRES_CONNECTION_STRING":
			postgresConnectionString = split[1]
		}
	}

	if len(os.Args) > 8 {
		homeserverURL = os.Args[1]
		userID = os.Args[2]
		accessToken = os.Args[3]
		hookSecret = os.Args[4]
		dataPath = os.Args[5]
		admin = os.Args[6]
		openrouterAPIKey = os.Args[7]
		postgresConnectionString = os.Args[8]
	}

	var missingConfig []string
	if homeserverURL == "" {
		missingConfig = append(missingConfig, "SIIKABOT_HOMESERVER_URL")
	}
	if userID == "" {
		missingConfig = append(missingConfig, "SIIKABOT_USER_ID")
	}
	if accessToken == "" {
		missingConfig = append(missingConfig, "SIIKABOT_ACCESS_TOKEN")
	}
	if hookSecret == "" {
		missingConfig = append(missingConfig, "SIIKABOT_HOOK_SECRET")
	}
	if dataPath == "" {
		missingConfig = append(missingConfig, "SIIKABOT_DATA_PATH")
	}
	if admin == "" {
		missingConfig = append(missingConfig, "SIIKABOT_ADMIN")
	}
	if openrouterAPIKey == "" {
		missingConfig = append(missingConfig, "SIIKABOT_OPENROUTER_API_KEY")
	}
	if postgresConnectionString == "" {
		missingConfig = append(missingConfig, "SIIKABOT_POSTGRES_URL")
	}

	if len(missingConfig) > 0 {
		log.Fatal().Strs("missing_keys", missingConfig).Msg("Missing required configuration")
	}

	err := bot.Run(homeserverURL, userID, accessToken, hookSecret, dataPath, admin, openrouterAPIKey, postgresConnectionString)
	log.Fatal().Err(err).Msg("Bot exited")
}
