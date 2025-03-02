package config

import (
	"fmt"
	"os"

	"github.com/joho/godotenv"
)

func LoadEnv() (err error) {
	godotenv.Load()
	return loadConfig()
}

var (
	HomeserverURL            = ""
	UserID                   = ""
	AccessToken              = ""
	HookSecret               = ""
	DataPath                 = ""
	Admin                    = ""
	OpenrouterAPIKey         = ""
	PostgresConnectionString = ""
)

func loadConfig() error {
	HomeserverURL = os.Getenv("SIIKABOT_HOMESERVER_URL")
	UserID = os.Getenv("SIIKABOT_USER_ID")
	AccessToken = os.Getenv("SIIKABOT_ACCESS_TOKEN")
	HookSecret = os.Getenv("SIIKABOT_HOOK_SECRET")
	DataPath = os.Getenv("SIIKABOT_DATA_PATH")
	Admin = os.Getenv("SIIKABOT_ADMIN")
	OpenrouterAPIKey = os.Getenv("SIIKABOT_OPENROUTER_API_KEY")
	PostgresConnectionString = os.Getenv("SIIKABOT_POSTGRES_CONNECTION_STRING")
	if HomeserverURL == "" {
		return fmt.Errorf("SIIKABOT_HOMESERVER_URL is not set")
	}
	if UserID == "" {
		return fmt.Errorf("SIIKABOT_USER_ID is not set")
	}
	if AccessToken == "" {
		return fmt.Errorf("SIIKABOT_ACCESS_TOKEN is not set")
	}
	if HookSecret == "" {
		return fmt.Errorf("SIIKABOT_HOOK_SECRET is not set")
	}
	if DataPath == "" {
		return fmt.Errorf("SIIKABOT_DATA_PATH is not set")
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
	return nil
}
