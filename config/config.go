package config

import (
	"fmt"
	"os"
	"strings"

	"github.com/joho/godotenv"
)

func LoadEnv() (err error) {
	godotenv.Load()
	return loadConfig()
}

var (
	HomeserverURL            = ""
	UserID                   = ""
	Password                 = ""
	HookSecret               = ""
	Admin                    = ""
	OpenrouterAPIKey         = ""
	PostgresConnectionString = ""
	PickleKey                = ""
	Debug                    = false
)

func loadConfig() error {
	HomeserverURL = os.Getenv("SIIKABOT_HOMESERVER_URL")
	UserID = os.Getenv("SIIKABOT_USER_ID")
	Password = os.Getenv("SIIKABOT_PASSWORD")
	HookSecret = os.Getenv("SIIKABOT_HOOK_SECRET")
	Admin = os.Getenv("SIIKABOT_ADMIN")
	OpenrouterAPIKey = os.Getenv("SIIKABOT_OPENROUTER_API_KEY")
	PostgresConnectionString = os.Getenv("SIIKABOT_POSTGRES_CONNECTION_STRING")
	PickleKey = os.Getenv("SIIKABOT_PICKLE_KEY")

	// Check if debug mode is enabled
	debugEnv := strings.ToLower(os.Getenv("SIIKABOT_DEBUG"))
	Debug = debugEnv == "true" || debugEnv == "1" || debugEnv == "yes"

	if HomeserverURL == "" {
		return fmt.Errorf("SIIKABOT_HOMESERVER_URL is not set")
	}
	if UserID == "" {
		return fmt.Errorf("SIIKABOT_USER_ID is not set")
	}
	if Password == "" {
		return fmt.Errorf("SIIKABOT_PASSWORD is not set")
	}
	if HookSecret == "" {
		return fmt.Errorf("SIIKABOT_HOOK_SECRET is not set")
	}
	if Admin == "" {
		return fmt.Errorf("SIIKABOT_ADMIN is not set")
	}
	if OpenrouterAPIKey == "" {
		return fmt.Errorf("SIIKABOT_OPENROUTER_API_KEY is not set")
	}
	if PostgresConnectionString == "" {
		return fmt.Errorf("SIIKABOT_POSTGRES_CONNECTION_STRING is not set")
	}
	if PickleKey == "" {
		return fmt.Errorf("SIIKABOT_PICKLE_KEY is not set")
	}
	return nil
}
