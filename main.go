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
		}
	}

	if len(os.Args) > 4 {
		homeserverURL = os.Args[1]
		userID = os.Args[2]
		accessToken = os.Args[3]
		hookSecret = os.Args[4]
	}

	if homeserverURL == "" || userID == "" || accessToken == "" || hookSecret == "" {
		log.Fatal("invalid config")
	}

	log.Fatal(bot.Run(homeserverURL, userID, accessToken, hookSecret))
}
