package constants

// WebhookType represents a webhook source
type WebhookType string

const (
	WebhookGitHub       WebhookType = "github"
	WebhookAlertmanager WebhookType = "alertmanager"
)

// AllWebhookTypes contains all valid webhook type values
var AllWebhookTypes = []WebhookType{WebhookGitHub, WebhookAlertmanager}

// WebhookEventType represents a specific event within a webhook
type WebhookEventType string

const (
	WebhookEventGitHubConfig      WebhookEventType = "config"
	WebhookEventGitHubPush        WebhookEventType = "push"
	WebhookEventGitHubPullRequest WebhookEventType = "pull_request"
	WebhookEventGitHubUnknown     WebhookEventType = "unknown"
	WebhookEventAlertFiring       WebhookEventType = "firing"
	WebhookEventAlertResolved     WebhookEventType = "resolved"
)

// WebhookEventTypes maps webhook types to their valid event types
var WebhookEventTypes = map[WebhookType][]WebhookEventType{
	WebhookGitHub: {
		WebhookEventGitHubConfig, WebhookEventGitHubPush,
		WebhookEventGitHubPullRequest, WebhookEventGitHubUnknown,
	},
	WebhookAlertmanager: {
		WebhookEventAlertFiring, WebhookEventAlertResolved,
	},
}
