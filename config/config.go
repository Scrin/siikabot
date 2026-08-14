package config

import (
	"fmt"
	"net/url"
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
	Password                 = ""
	HookSecret               = ""
	Admin                    = ""
	CloudflareAccountID      = ""
	CloudflareAPIToken       = ""
	CloudflareAIGatewayID    = ""
	PostgresConnectionString = ""
	PickleKey                = ""
	ConsoleOutput            = false
	Timezone                 = ""
	GoogleAPIKey             = ""
	GoogleSearchEngineID     = ""
	AlertmanagerUser         = ""
	AlertmanagerPassword     = ""
	TempoEndpoint            = ""
	TempoUser                = ""
	TempoPassword            = ""
)

func loadConfig() error {
	HomeserverURL = os.Getenv("SIIKABOT_HOMESERVER_URL")
	UserID = os.Getenv("SIIKABOT_USER_ID")
	Password = os.Getenv("SIIKABOT_PASSWORD")
	HookSecret = os.Getenv("SIIKABOT_HOOK_SECRET")
	Admin = os.Getenv("SIIKABOT_ADMIN")
	CloudflareAccountID = os.Getenv("SIIKABOT_CLOUDFLARE_ACCOUNT_ID")
	CloudflareAPIToken = os.Getenv("SIIKABOT_CLOUDFLARE_API_TOKEN")
	CloudflareAIGatewayID = os.Getenv("SIIKABOT_CLOUDFLARE_AI_GATEWAY_ID")
	PostgresConnectionString = os.Getenv("SIIKABOT_POSTGRES_CONNECTION_STRING")
	PickleKey = os.Getenv("SIIKABOT_PICKLE_KEY")
	ConsoleOutput = os.Getenv("SIIKABOT_CONSOLE_OUTPUT") == "true"
	Timezone = os.Getenv("SIIKABOT_TIMEZONE")
	GoogleAPIKey = os.Getenv("SIIKABOT_GOOGLE_API_KEY")
	GoogleSearchEngineID = os.Getenv("SIIKABOT_GOOGLE_SEARCH_ENGINE_ID")
	AlertmanagerUser = os.Getenv("SIIKABOT_ALERTMANAGER_USER")
	AlertmanagerPassword = os.Getenv("SIIKABOT_ALERTMANAGER_PASSWORD")
	TempoEndpoint = os.Getenv("SIIKABOT_TEMPO_ENDPOINT")
	TempoUser = os.Getenv("SIIKABOT_TEMPO_USER")
	TempoPassword = os.Getenv("SIIKABOT_TEMPO_PASSWORD")

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
	if CloudflareAccountID == "" {
		return fmt.Errorf("SIIKABOT_CLOUDFLARE_ACCOUNT_ID is not set")
	}
	if CloudflareAPIToken == "" {
		return fmt.Errorf("SIIKABOT_CLOUDFLARE_API_TOKEN is not set")
	}
	if CloudflareAIGatewayID == "" {
		return fmt.Errorf("SIIKABOT_CLOUDFLARE_AI_GATEWAY_ID is not set")
	}
	if PostgresConnectionString == "" {
		return fmt.Errorf("SIIKABOT_POSTGRES_CONNECTION_STRING is not set")
	}
	if PickleKey == "" {
		return fmt.Errorf("SIIKABOT_PICKLE_KEY is not set")
	}
	if Timezone == "" {
		return fmt.Errorf("SIIKABOT_TIMEZONE is not set")
	}
	if GoogleAPIKey == "" {
		return fmt.Errorf("SIIKABOT_GOOGLE_API_KEY is not set")
	}
	if GoogleSearchEngineID == "" {
		return fmt.Errorf("SIIKABOT_GOOGLE_SEARCH_ENGINE_ID is not set")
	}
	if AlertmanagerUser == "" {
		return fmt.Errorf("SIIKABOT_ALERTMANAGER_USER is not set")
	}
	if AlertmanagerPassword == "" {
		return fmt.Errorf("SIIKABOT_ALERTMANAGER_PASSWORD is not set")
	}
	// Tracing is always on, so these are required like everything else. A deployment missing them
	// fails at boot rather than running indefinitely with no traces and no indication of why.
	if TempoEndpoint == "" {
		return fmt.Errorf("SIIKABOT_TEMPO_ENDPOINT is not set")
	}
	// Validated here rather than left to the exporter, which treats an unparseable endpoint as a
	// reason to silently fall back to localhost:4317. url.Parse is permissive enough that a bare
	// "host:port" parses without error but yields no host, so the scheme and host are checked
	// explicitly — the alternative is a confusing DNS error at runtime instead of a clear one now.
	if endpoint, err := url.Parse(TempoEndpoint); err != nil {
		return fmt.Errorf("SIIKABOT_TEMPO_ENDPOINT is not a valid URL: %w", err)
	} else if endpoint.Scheme != "http" && endpoint.Scheme != "https" {
		return fmt.Errorf("SIIKABOT_TEMPO_ENDPOINT must start with http:// or https://, got %q", TempoEndpoint)
	} else if endpoint.Host == "" {
		return fmt.Errorf("SIIKABOT_TEMPO_ENDPOINT has no host: %q", TempoEndpoint)
	}
	if TempoUser == "" {
		return fmt.Errorf("SIIKABOT_TEMPO_USER is not set")
	}
	if TempoPassword == "" {
		return fmt.Errorf("SIIKABOT_TEMPO_PASSWORD is not set")
	}
	return nil
}
