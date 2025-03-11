package chat

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strings"
	"time"

	"github.com/Scrin/siikabot/openrouter"
	"github.com/rs/zerolog/log"
)

// GitHubIssueToolDefinition returns the tool definition for the GitHub issue retrieval tool
var GitHubIssueToolDefinition = openrouter.ToolDefinition{
	Type: "function",
	Function: openrouter.FunctionSchema{
		Name:        "get_github_issue",
		Description: "Retrieve details about a GitHub issue or pull request",
		Parameters: json.RawMessage(`{
				"type": "object",
				"properties": {
					"repo": {
						"type": "string",
						"description": "The GitHub repository in the format 'owner/repo' (e.g., 'golang/go')"
					},
					"issue_number": {
						"type": "integer",
						"description": "The issue or pull request number"
					}
				},
				"required": ["repo", "issue_number"]
			}`),
	},
	Handler: handleGitHubIssueToolCall,
}

// handleGitHubIssueToolCall handles GitHub issue tool calls
func handleGitHubIssueToolCall(ctx context.Context, arguments string) (string, error) {
	// Parse the arguments
	var args struct {
		Repo        string `json:"repo"`
		IssueNumber int    `json:"issue_number"`
	}

	// Log the raw arguments for debugging
	log.Debug().Ctx(ctx).Str("arguments", arguments).Msg("Received GitHub issue tool call")

	if err := json.Unmarshal([]byte(arguments), &args); err != nil {
		log.Error().Ctx(ctx).Err(err).Str("arguments", arguments).Msg("Failed to parse tool arguments")
		return "", fmt.Errorf("failed to parse arguments: %w", err)
	}

	if args.Repo == "" {
		return "", fmt.Errorf("repository is required")
	}

	if args.IssueNumber <= 0 {
		return "", fmt.Errorf("valid issue number is required")
	}

	// Validate repository format (owner/repo)
	repoPattern := regexp.MustCompile(`^[a-zA-Z0-9_.-]+/[a-zA-Z0-9_.-]+$`)
	if !repoPattern.MatchString(args.Repo) {
		return "", fmt.Errorf("invalid repository format, must be 'owner/repo'")
	}

	// Fetch issue details from GitHub API
	issueData, err := getGitHubIssueData(ctx, args.Repo, args.IssueNumber)
	if err != nil {
		return "", err
	}

	// Fetch comments if there are any
	var comments []GitHubComment
	if issueData.Comments > 0 {
		comments, err = getGitHubIssueComments(ctx, args.Repo, args.IssueNumber)
		if err != nil {
			log.Warn().Ctx(ctx).Err(err).
				Str("repo", args.Repo).
				Int("issue_number", args.IssueNumber).
				Msg("Failed to fetch comments, continuing with issue data only")
			// Don't return error, just continue with the issue data
		}
	}

	return formatGitHubIssueData(issueData, comments), nil
}

// GitHubIssue represents the data structure for a GitHub issue or pull request
type GitHubIssue struct {
	Number      int           `json:"number"`
	Title       string        `json:"title"`
	State       string        `json:"state"`
	HTMLURL     string        `json:"html_url"`
	Body        string        `json:"body"`
	User        GitHubUser    `json:"user"`
	CreatedAt   time.Time     `json:"created_at"`
	UpdatedAt   time.Time     `json:"updated_at"`
	ClosedAt    *time.Time    `json:"closed_at"`
	Labels      []GitHubLabel `json:"labels"`
	Assignees   []GitHubUser  `json:"assignees"`
	Comments    int           `json:"comments"`
	PullRequest *struct{}     `json:"pull_request,omitempty"`
}

// GitHubUser represents a GitHub user
type GitHubUser struct {
	Login     string `json:"login"`
	HTMLURL   string `json:"html_url"`
	AvatarURL string `json:"avatar_url"`
}

// GitHubLabel represents a GitHub issue label
type GitHubLabel struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	Color       string `json:"color"`
}

// GitHubComment represents a comment on a GitHub issue
type GitHubComment struct {
	ID        int        `json:"id"`
	User      GitHubUser `json:"user"`
	CreatedAt time.Time  `json:"created_at"`
	UpdatedAt time.Time  `json:"updated_at"`
	Body      string     `json:"body"`
}

// getGitHubIssueData fetches issue data from the GitHub API
func getGitHubIssueData(ctx context.Context, repo string, issueNumber int) (*GitHubIssue, error) {
	// Construct the URL for the GitHub API
	apiURL := fmt.Sprintf("https://api.github.com/repos/%s/issues/%d", repo, issueNumber)

	log.Debug().Ctx(ctx).Str("url", apiURL).Str("repo", repo).Int("issue_number", issueNumber).Msg("Fetching GitHub issue data")

	// Create HTTP request
	req, err := http.NewRequestWithContext(ctx, "GET", apiURL, nil)
	if err != nil {
		log.Error().Ctx(ctx).Err(err).Str("url", apiURL).Msg("Failed to create GitHub API request")
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	// Set User-Agent header (required by GitHub API)
	req.Header.Set("User-Agent", "Siikabot-GitHub-Tool")
	req.Header.Set("Accept", "application/vnd.github.v3+json")

	// Execute the request
	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		log.Error().Ctx(ctx).Err(err).Str("url", apiURL).Msg("Failed to fetch GitHub issue data")
		return nil, fmt.Errorf("failed to fetch GitHub issue data: %w", err)
	}
	defer resp.Body.Close()

	// Read the response body
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		log.Error().Ctx(ctx).Err(err).Str("url", apiURL).Msg("Failed to read GitHub API response")
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		log.Error().Ctx(ctx).Int("status_code", resp.StatusCode).Str("url", apiURL).Str("response", string(body)).Msg("GitHub API returned non-OK status")

		if resp.StatusCode == http.StatusNotFound {
			return nil, fmt.Errorf("issue #%d not found in repository %s", issueNumber, repo)
		}

		return nil, fmt.Errorf("GitHub API returned status code %d", resp.StatusCode)
	}

	// Parse the JSON response
	var issue GitHubIssue
	if err := json.Unmarshal(body, &issue); err != nil {
		log.Error().Ctx(ctx).Err(err).Str("url", apiURL).Str("response", string(body)).Msg("Failed to parse GitHub API response")
		return nil, fmt.Errorf("failed to parse GitHub API response: %w", err)
	}

	return &issue, nil
}

// getGitHubIssueComments fetches comments for an issue from the GitHub API
func getGitHubIssueComments(ctx context.Context, repo string, issueNumber int) ([]GitHubComment, error) {
	// Construct the URL for the GitHub API
	apiURL := fmt.Sprintf("https://api.github.com/repos/%s/issues/%d/comments", repo, issueNumber)

	log.Debug().Ctx(ctx).Str("url", apiURL).Str("repo", repo).Int("issue_number", issueNumber).Msg("Fetching GitHub issue comments")

	// Create HTTP request
	req, err := http.NewRequestWithContext(ctx, "GET", apiURL, nil)
	if err != nil {
		log.Error().Ctx(ctx).Err(err).Str("url", apiURL).Msg("Failed to create GitHub API request for comments")
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	// Set User-Agent header (required by GitHub API)
	req.Header.Set("User-Agent", "Siikabot-GitHub-Tool")
	req.Header.Set("Accept", "application/vnd.github.v3+json")

	// Execute the request
	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		log.Error().Ctx(ctx).Err(err).Str("url", apiURL).Msg("Failed to fetch GitHub issue comments")
		return nil, fmt.Errorf("failed to fetch GitHub issue comments: %w", err)
	}
	defer resp.Body.Close()

	// Read the response body
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		log.Error().Ctx(ctx).Err(err).Str("url", apiURL).Msg("Failed to read GitHub API response for comments")
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		log.Error().Ctx(ctx).Int("status_code", resp.StatusCode).Str("url", apiURL).Str("response", string(body)).Msg("GitHub API returned non-OK status for comments")
		return nil, fmt.Errorf("GitHub API returned status code %d for comments", resp.StatusCode)
	}

	// Parse the JSON response
	var comments []GitHubComment
	if err := json.Unmarshal(body, &comments); err != nil {
		log.Error().Ctx(ctx).Err(err).Str("url", apiURL).Str("response", string(body)).Msg("Failed to parse GitHub API response for comments")
		return nil, fmt.Errorf("failed to parse GitHub API response for comments: %w", err)
	}

	return comments, nil
}

// formatGitHubIssueData formats the GitHub issue data into a readable string
func formatGitHubIssueData(issue *GitHubIssue, comments []GitHubComment) string {
	var sb strings.Builder

	// Determine if it's an issue or PR
	issueType := "Issue"
	if issue.PullRequest != nil {
		issueType = "Pull Request"
	}

	// Basic information
	sb.WriteString(fmt.Sprintf("## %s #%d: %s\n", issueType, issue.Number, issue.Title))
	sb.WriteString(fmt.Sprintf("**State:** %s\n", strings.ToUpper(issue.State)))
	sb.WriteString(fmt.Sprintf("**URL:** %s\n", issue.HTMLURL))
	sb.WriteString(fmt.Sprintf("**Created by:** %s on %s\n",
		issue.User.Login,
		issue.CreatedAt.Format("2006-01-02 15:04:05")))

	if issue.ClosedAt != nil {
		sb.WriteString(fmt.Sprintf("**Closed at:** %s\n", issue.ClosedAt.Format("2006-01-02 15:04:05")))
	}

	sb.WriteString(fmt.Sprintf("**Last updated:** %s\n", issue.UpdatedAt.Format("2006-01-02 15:04:05")))

	// Labels
	if len(issue.Labels) > 0 {
		sb.WriteString("\n**Labels:** ")
		for i, label := range issue.Labels {
			if i > 0 {
				sb.WriteString(", ")
			}
			sb.WriteString(label.Name)
		}
		sb.WriteString("\n")
	}

	// Assignees
	if len(issue.Assignees) > 0 {
		sb.WriteString("\n**Assignees:** ")
		for i, assignee := range issue.Assignees {
			if i > 0 {
				sb.WriteString(", ")
			}
			sb.WriteString(assignee.Login)
		}
		sb.WriteString("\n")
	}

	// Comments count
	sb.WriteString(fmt.Sprintf("\n**Comments:** %d\n", issue.Comments))

	// Description (body)
	if issue.Body != "" {
		// Truncate body if it's too long
		body := issue.Body
		if len(body) > 1000 {
			body = body[:997] + "..."
		}
		sb.WriteString("\n## Description\n")
		sb.WriteString(body)
	}

	// Comments
	if len(comments) > 0 {
		sb.WriteString("\n\n## Comments\n")

		// Display logic for comments
		totalComments := len(comments)

		// If there are 20 or fewer comments, show all of them
		if totalComments <= 20 {
			sb.WriteString(fmt.Sprintf("*Showing all %d comments*\n\n", totalComments))

			for i, comment := range comments {
				formatComment(&sb, comment, i+1)
			}
		} else {
			// If there are more than 20 comments, show first 10 and last 10
			sb.WriteString(fmt.Sprintf("*Showing first 10 and last 10 of %d comments*\n\n", totalComments))

			// First 10 comments
			sb.WriteString("### First 10 comments:\n\n")
			for i := 0; i < 10; i++ {
				formatComment(&sb, comments[i], i+1)
			}

			// Separator
			middleSkipped := totalComments - 20
			sb.WriteString(fmt.Sprintf("\n*%d comments in the middle not shown*\n\n", middleSkipped))

			// Last 10 comments
			sb.WriteString("### Last 10 comments:\n\n")
			for i := totalComments - 10; i < totalComments; i++ {
				formatComment(&sb, comments[i], i+1)
			}
		}
	}

	return sb.String()
}

// formatComment formats a single comment and appends it to the string builder
func formatComment(sb *strings.Builder, comment GitHubComment, number int) {
	sb.WriteString(fmt.Sprintf("### Comment #%d by %s on %s\n",
		number,
		comment.User.Login,
		comment.CreatedAt.Format("2006-01-02 15:04:05")))

	// Truncate comment body if it's too long
	commentBody := comment.Body
	if len(commentBody) > 500 {
		commentBody = commentBody[:497] + "..."
	}

	sb.WriteString(commentBody)
	sb.WriteString("\n\n")
}
