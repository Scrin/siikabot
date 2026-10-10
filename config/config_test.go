package config

import (
	"strings"
	"testing"
)

// requiredVars is every environment variable loadConfig refuses to start without.
//
// Kept as an explicit list rather than derived from the code, so that removing a validation check
// fails this test instead of silently weakening startup.
var requiredVars = []string{
	"SIIKABOT_HOMESERVER_URL",
	"SIIKABOT_USER_ID",
	"SIIKABOT_PASSWORD",
	"SIIKABOT_HOOK_SECRET",
	"SIIKABOT_ADMIN",
	"SIIKABOT_CLOUDFLARE_ACCOUNT_ID",
	"SIIKABOT_CLOUDFLARE_API_TOKEN",
	"SIIKABOT_CLOUDFLARE_AI_GATEWAY_ID",
	"SIIKABOT_POSTGRES_CONNECTION_STRING",
	"SIIKABOT_PICKLE_KEY",
	"SIIKABOT_TIMEZONE",
	"SIIKABOT_GOOGLE_API_KEY",
	"SIIKABOT_GOOGLE_SEARCH_ENGINE_ID",
	"SIIKABOT_ALERTMANAGER_USER",
	"SIIKABOT_ALERTMANAGER_PASSWORD",
	"SIIKABOT_TEMPO_ENDPOINT",
	"SIIKABOT_TEMPO_USER",
	"SIIKABOT_TEMPO_PASSWORD",
	"SIIKABOT_SOURCE_URL",
	"SIIKABOT_FRONTEND_URL",
}

// urlVars are the required variables that are validated as URLs, with a valid placeholder for each
var urlVars = map[string]string{
	"SIIKABOT_TEMPO_ENDPOINT": "https://tempo.example.org:443",
	"SIIKABOT_SOURCE_URL":     "https://git.example.org/siikabot",
	"SIIKABOT_FRONTEND_URL":   "https://siikabot.example.org/",
}

// setAllRequired populates every required variable with a placeholder, restored by t.Setenv
func setAllRequired(t *testing.T) {
	t.Helper()
	for _, name := range requiredVars {
		t.Setenv(name, "test-value")
	}
	// These are validated as URLs, so a placeholder will not do for them
	for name, value := range urlVars {
		t.Setenv(name, value)
	}
}

// TestTempoEndpointMustBeAURL guards the shape of the endpoint rather than merely its presence.
//
// The OTLP exporter treats an endpoint it cannot parse as a reason to fall back to localhost:4317,
// and a bare "host:port" parses without error but yields no host at all. Either way the bot runs with
// tracing that appears configured and exports nowhere, so the value is rejected here at boot instead.
func TestTempoEndpointMustBeAURL(t *testing.T) {
	rejected := map[string]string{
		"no scheme":       "tempo.example.org:443",
		"scheme only":     "https://",
		"wrong scheme":    "grpc://tempo.example.org:443",
		"host only":       "tempo.example.org",
		"unparseable":     "https://tempo.example.org:port",
		"leading garbage": "://tempo.example.org",
	}

	for name, endpoint := range rejected {
		t.Run(name, func(t *testing.T) {
			setAllRequired(t)
			t.Setenv("SIIKABOT_TEMPO_ENDPOINT", endpoint)

			err := loadConfig()
			if err == nil {
				t.Fatalf("%q was accepted as a Tempo endpoint", endpoint)
			}
			if !strings.Contains(err.Error(), "SIIKABOT_TEMPO_ENDPOINT") {
				t.Errorf("error should name the variable, got: %v", err)
			}
		})
	}
}

// TestTempoEndpointAcceptsValidURLs verifies plaintext endpoints stay usable, since a local Tempo
// without TLS is the ordinary way to try this out
func TestTempoEndpointAcceptsValidURLs(t *testing.T) {
	for _, endpoint := range []string{
		"https://tempo.example.org:443",
		"https://tempo.example.org",
		"http://localhost:4317",
	} {
		t.Run(endpoint, func(t *testing.T) {
			setAllRequired(t)
			t.Setenv("SIIKABOT_TEMPO_ENDPOINT", endpoint)

			if err := loadConfig(); err != nil {
				t.Fatalf("%q should be a valid endpoint, got: %v", endpoint, err)
			}
		})
	}
}

// TestSourceAndFrontendMustBeURLs guards the shape of the source and frontend addresses. The chat
// model gives them out as links, and reads the bot's source code from the repository, so a value
// that is not a URL fails at boot rather than in a conversation.
func TestSourceAndFrontendMustBeURLs(t *testing.T) {
	rejected := map[string]string{
		"no scheme":    "github.com/owner/siikabot",
		"wrong scheme": "git@github.com:owner/siikabot.git",
		"scheme only":  "https://",
		"unparseable":  "https://example.org:port",
	}

	for _, name := range []string{"SIIKABOT_SOURCE_URL", "SIIKABOT_FRONTEND_URL"} {
		for kind, value := range rejected {
			t.Run(name+"/"+kind, func(t *testing.T) {
				setAllRequired(t)
				t.Setenv(name, value)

				err := loadConfig()
				if err == nil {
					t.Fatalf("%q was accepted as %s", value, name)
				}
				if !strings.Contains(err.Error(), name) {
					t.Errorf("error should name the variable, got: %v", err)
				}
			})
		}
	}
}

func TestLoadConfigSucceedsWhenEverythingIsSet(t *testing.T) {
	setAllRequired(t)

	if err := loadConfig(); err != nil {
		t.Fatalf("expected a complete configuration to load, got: %v", err)
	}
	if SourceURL != urlVars["SIIKABOT_SOURCE_URL"] || FrontendURL != urlVars["SIIKABOT_FRONTEND_URL"] {
		t.Errorf("the source and frontend URLs were loaded as %q and %q", SourceURL, FrontendURL)
	}
}

// TestLoadConfigRejectsEachMissingVariable is the guard that matters: every required variable must
// fail startup by name when absent, so a misconfigured deployment stops at boot rather than running
// in a degraded state nobody notices.
func TestLoadConfigRejectsEachMissingVariable(t *testing.T) {
	for _, missing := range requiredVars {
		t.Run(missing, func(t *testing.T) {
			setAllRequired(t)
			t.Setenv(missing, "")

			err := loadConfig()
			if err == nil {
				t.Fatalf("%s was missing but the configuration loaded anyway", missing)
			}
			if !strings.Contains(err.Error(), missing) {
				t.Errorf("error should name the missing variable, got: %v", err)
			}
		})
	}
}

// TestGatewayIDHasNoSilentDefault covers the specific mistake this validation exists to prevent.
//
// The gateway id used to fall back to "default" when unset. Gateways are configured individually and
// only one carries the OpenTelemetry exporter, so that fallback meant an unset variable produced a bot
// that worked perfectly while exporting no traces — a failure with no error attached to it.
func TestGatewayIDHasNoSilentDefault(t *testing.T) {
	setAllRequired(t)
	t.Setenv("SIIKABOT_CLOUDFLARE_AI_GATEWAY_ID", "")

	err := loadConfig()
	if err == nil {
		t.Fatal("an unset gateway id must fail startup, not fall back to a default")
	}
	if CloudflareAIGatewayID != "" {
		t.Errorf("no value should be substituted for an unset gateway id, got %q", CloudflareAIGatewayID)
	}
}

// TestConsoleOutputIsOptional verifies the one genuinely optional setting stays optional
func TestConsoleOutputIsOptional(t *testing.T) {
	setAllRequired(t)

	t.Setenv("SIIKABOT_CONSOLE_OUTPUT", "")
	if err := loadConfig(); err != nil {
		t.Fatalf("console output should be optional, got: %v", err)
	}
	if ConsoleOutput {
		t.Error("expected console output to default to false")
	}

	t.Setenv("SIIKABOT_CONSOLE_OUTPUT", "true")
	if err := loadConfig(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !ConsoleOutput {
		t.Error(`expected "true" to enable console output`)
	}
}
