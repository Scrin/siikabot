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
}

// setAllRequired populates every required variable with a placeholder, restored by t.Setenv
func setAllRequired(t *testing.T) {
	t.Helper()
	for _, name := range requiredVars {
		t.Setenv(name, "test-value")
	}
}

func TestLoadConfigSucceedsWhenEverythingIsSet(t *testing.T) {
	setAllRequired(t)

	if err := loadConfig(); err != nil {
		t.Fatalf("expected a complete configuration to load, got: %v", err)
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
