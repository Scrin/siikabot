package main

import (
	"log"
	"os"
	"siikabot/bot"
	"strings"
)

func main() {
	homeserverURL := ""
	userID := ""
	accessToken := ""
	hookSecret := ""
	dataPath := ""
	admin := ""

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
		}
	}

	if len(os.Args) > 6 {
		homeserverURL = os.Args[1]
		userID = os.Args[2]
		accessToken = os.Args[3]
		hookSecret = os.Args[4]
		dataPath = os.Args[5]
		admin = os.Args[6]
	}

	if homeserverURL == "" || userID == "" || accessToken == "" || hookSecret == "" || dataPath == "" || admin == "" {
		log.Fatal("invalid config")
	}

	log.Fatal(bot.Run(homeserverURL, userID, accessToken, hookSecret, dataPath, admin))
}
