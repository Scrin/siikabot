// Package webfetch reads web pages for the chat model. It fetches a URL, finds the page's main content and
// judges how the fetch went, so the model gets either the page as markdown or a plain account of why it
// couldn't be read and what to do instead.
//
// Everything a page holds was written by strangers, and the bot may sit in rooms whose members are strangers
// too. The package never connects to addresses that aren't public (guard.go), and once a turn has read content
// from outside the room, it fetches only URLs that the turn has already seen (provenance.go).
package webfetch

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/Scrin/siikabot/metrics"
	"github.com/Scrin/siikabot/tracing"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
)

const (
	// requestTimeout bounds a fetch, redirects and reading the body included. Fetches run inside a chat turn,
	// which has a deadline of its own.
	requestTimeout = 10 * time.Second
	// maxRedirects is how many redirects a fetch follows
	maxRedirects = 5
	// maxBodyBytes caps how much of a response is read. It doesn't depend on how much the model gets back:
	// some pages carry hundreds of kilobytes of script before their text starts.
	maxBodyBytes = 10 << 20
	// tier names how a page was fetched, in metrics and spans. Phase 3 of the web plan adds a browser.
	tier = "direct"
)

// The request headers. The User-Agent says honestly what the bot is.
const (
	userAgent      = "Mozilla/5.0 (compatible; Siikabot/2.0; +https://github.com/Scrin/siikabot)"
	acceptHeader   = "text/html,application/xhtml+xml,application/xml;q=0.9,application/pdf;q=0.8,text/plain;q=0.8,application/json;q=0.8,*/*;q=0.5"
	acceptLanguage = "fi,en;q=0.8"
)

// Outcome is how a fetch went. It's also the outcome label of the fetch metrics.
type Outcome string

const (
	// OutcomeOK means the page was read
	OutcomeOK Outcome = "ok"
	// OutcomeThin means the page was read but has almost no text, usually because it needs JavaScript
	OutcomeThin Outcome = "thin"
	// OutcomePaywalled means only the free part of an article was there to read
	OutcomePaywalled Outcome = "paywalled"
	// OutcomeBlocked means the site refused the bot, or showed a bot check instead of the page
	OutcomeBlocked Outcome = "blocked"
	// OutcomeNotFound means there's no page at the URL
	OutcomeNotFound Outcome = "not_found"
	// OutcomeRejected means the site refused the request for another reason, such as a bad query
	OutcomeRejected Outcome = "rejected"
	// OutcomeBadCertificate means the site's certificate isn't valid, so the page wasn't read
	OutcomeBadCertificate Outcome = "bad_certificate"
	// OutcomeUnsupported means the URL points to something the bot can't read, such as an image
	OutcomeUnsupported Outcome = "unsupported"
	// OutcomeError means the site failed to answer, or answered with a server error
	OutcomeError Outcome = "error"
	// OutcomeInvalidURL means the URL isn't an http or https address
	OutcomeInvalidURL Outcome = "invalid_url"
	// OutcomeDeniedAddress means the URL leads to an address that isn't public, which the bot never fetches
	OutcomeDeniedAddress Outcome = "denied_address"
	// OutcomeDeniedHost means the bot doesn't fetch from the URL's site at all
	OutcomeDeniedHost Outcome = "denied_host"
	// OutcomeDeniedProvenance means the turn has read outside content and the URL isn't one it has seen
	OutcomeDeniedProvenance Outcome = "denied_provenance"
)

// allOutcomes lists every outcome, so the metrics can start each one at zero
var allOutcomes = []Outcome{
	OutcomeOK, OutcomeThin, OutcomePaywalled, OutcomeBlocked, OutcomeNotFound, OutcomeRejected,
	OutcomeBadCertificate, OutcomeUnsupported, OutcomeError, OutcomeInvalidURL, OutcomeDeniedAddress,
	OutcomeDeniedHost, OutcomeDeniedProvenance,
}

// Result is a fetched page, or why it couldn't be read
type Result struct {
	// URL is the page's address after redirects, or the requested one when there was no answer
	URL string
	// Host is the URL's host
	Host    string
	Outcome Outcome
	// StatusCode is the HTTP status of the answer, or 0 when there was none
	StatusCode int

	// MediaType is the type of content that isn't a web page, such as application/pdf
	MediaType string
	// The page's details, when it has them
	Title     string
	Site      string
	Byline    string
	Language  string
	Published time.Time

	// Content is the page as markdown, or for a failure, what the answer said about it, when it said anything
	Content string
	// Reason qualifies the outcome for the model: the bot check a site showed, why a certificate isn't valid,
	// what a URL points to that the bot can't read, and so on
	Reason string
	// Remembered says the site blocked an earlier fetch, and wasn't asked again this time
	Remembered bool
}

// failed reports whether the fetch got nothing from the page to show
func (r Result) failed() bool {
	switch r.Outcome {
	case OutcomeOK, OutcomePaywalled, OutcomeThin:
		return false
	}
	return true
}

var client = newClient()

func init() {
	outcomes := make([]string, 0, len(allOutcomes))
	for _, outcome := range allOutcomes {
		outcomes = append(outcomes, string(outcome))
	}
	metrics.InitializeWebFetchTier(tier, outcomes)
}

// Fetch reads a URL. It never returns an error: a fetch that fails is a Result whose outcome says how, since
// the model needs to know what happened either way.
func Fetch(ctx context.Context, rawURL string) Result {
	start := time.Now()
	ctx, span := tracer.Start(ctx, "web.fetch")
	defer span.End()

	result, bytesRead, err := fetch(ctx, rawURL)

	duration := time.Since(start)
	metrics.RecordWebFetch(tier, string(result.Outcome), duration.Seconds())

	span.SetAttributes(
		attribute.String("siikabot.web.host", result.Host),
		attribute.String("siikabot.web.tier", tier),
		attribute.String("siikabot.web.outcome", string(result.Outcome)),
		attribute.Int("siikabot.web.status_code", result.StatusCode),
		attribute.Int("siikabot.web.bytes_read", bytesRead),
		attribute.Int("siikabot.web.content_bytes", len(result.Content)),
	)
	if result.Reason != "" {
		span.SetAttributes(attribute.String("siikabot.web.reason", result.Reason))
	}
	if err != nil {
		span.RecordError(err)
	}
	if result.failed() {
		span.SetStatus(codes.Error, string(result.Outcome))
	}

	level := zerolog.InfoLevel
	if result.failed() {
		level = zerolog.WarnLevel
	}
	// URLs can carry credentials in their queries, which don't belong in the logs
	log.WithLevel(level).Ctx(ctx).Err(redactError(err)).
		Str("url", tracing.RedactURL(rawURL)).
		Str("final_url", tracing.RedactURL(result.URL)).
		Str("host", result.Host).
		Str("tier", tier).
		Str("outcome", string(result.Outcome)).
		Str("reason", result.Reason).
		Int("status_code", result.StatusCode).
		Int("bytes_read", bytesRead).
		Int("content_bytes", len(result.Content)).
		Float64("duration_sec", duration.Seconds()).
		Msg("Fetched a web page")

	return result
}

// fetch does the work of Fetch, returning how many bytes of the answer were read, and the error behind a
// failure when there's one
func fetch(ctx context.Context, rawURL string) (Result, int, error) {
	pageURL, err := parseURL(rawURL)
	if err != nil {
		return Result{URL: rawURL, Outcome: OutcomeInvalidURL, Reason: err.Error()}, 0, err
	}
	result := Result{URL: pageURL.String(), Host: pageURL.Hostname()}

	if !ProvenanceFrom(ctx).Allows(pageURL) {
		result.Outcome = OutcomeDeniedProvenance
		return result, 0, nil
	}
	if site := deniedSite(pageURL.Hostname()); site != "" {
		result.Outcome, result.Reason = OutcomeDeniedHost, site
		return result, 0, nil
	}
	if block, ok := recentBlock(pageURL.Hostname()); ok {
		result.Outcome, result.Reason, result.StatusCode = OutcomeBlocked, block.reason, block.statusCode
		result.Remembered = true
		return result, 0, nil
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, pageURL.String(), nil)
	if err != nil {
		result.Outcome, result.Reason = OutcomeInvalidURL, "it can't be requested"
		return result, 0, err
	}
	req.Header.Set("User-Agent", userAgent)
	req.Header.Set("Accept", acceptHeader)
	req.Header.Set("Accept-Language", acceptLanguage)

	resp, err := client.Do(req)
	if err != nil {
		result.Outcome, result.Reason = requestFailure(err)
		return result, 0, err
	}
	defer resp.Body.Close()

	// From here on, the page is wherever the redirects led
	result.URL, result.Host = resp.Request.URL.String(), resp.Request.URL.Hostname()
	result.StatusCode = resp.StatusCode

	body, err := io.ReadAll(io.LimitReader(resp.Body, maxBodyBytes))
	if err != nil {
		result.Outcome, result.Reason = requestFailure(err)
		return result, len(body), err
	}

	page := response{
		url:         resp.Request.URL,
		statusCode:  resp.StatusCode,
		contentType: resp.Header.Get("Content-Type"),
		body:        body,
		truncated:   len(body) == maxBodyBytes,
	}
	result = judge(ctx, page, result)
	if result.Outcome == OutcomeBlocked && remembersBlock(resp.StatusCode, result.Reason) {
		rememberBlock(result.Host, blockedHost{statusCode: resp.StatusCode, reason: result.Reason})
	}
	return result, len(body), nil
}

// parseURL checks that a URL is an http or https address with a host, and drops its fragment, which only
// matters in a browser. Its errors explain to the model what's wrong with the URL.
func parseURL(rawURL string) (*url.URL, error) {
	pageURL, err := url.Parse(strings.TrimSpace(rawURL))
	if err != nil {
		return nil, errors.New("it isn't a valid URL")
	}
	pageURL.Scheme = strings.ToLower(pageURL.Scheme)
	if pageURL.Scheme != "http" && pageURL.Scheme != "https" {
		return nil, errors.New("it has to start with http:// or https://")
	}
	if pageURL.Hostname() == "" {
		return nil, errors.New("it has no host name")
	}
	if pageURL.User != nil {
		// Whatever the credentials are for, they don't belong in a request the model makes
		return nil, errors.New("it has a user name or password in it")
	}
	pageURL.Fragment, pageURL.RawFragment = "", ""
	return pageURL, nil
}

// requestFailure works out what a failed request means: the address wasn't allowed, the certificate isn't
// valid, or the site couldn't be reached
func requestFailure(err error) (Outcome, string) {
	if errors.Is(err, errAddressNotAllowed) {
		return OutcomeDeniedAddress, ""
	}

	var unknownAuthority x509.UnknownAuthorityError
	var hostname x509.HostnameError
	var invalid x509.CertificateInvalidError
	var verification *tls.CertificateVerificationError
	switch {
	case errors.As(err, &invalid):
		if invalid.Reason == x509.Expired {
			return OutcomeBadCertificate, "expired"
		}
		return OutcomeBadCertificate, "invalid"
	case errors.As(err, &hostname):
		return OutcomeBadCertificate, "wrong_host"
	case errors.As(err, &unknownAuthority):
		return OutcomeBadCertificate, "unknown_authority"
	case errors.As(err, &verification):
		return OutcomeBadCertificate, "invalid"
	}

	var dnsErr *net.DNSError
	switch {
	case errors.Is(err, errTooManyRedirects):
		return OutcomeError, "too_many_redirects"
	case errors.As(err, &dnsErr) && dnsErr.IsNotFound:
		return OutcomeError, "no_such_host"
	case errors.Is(err, context.DeadlineExceeded) || isTimeout(err):
		return OutcomeError, "timeout"
	case errors.Is(err, context.Canceled):
		return OutcomeError, "cancelled"
	}
	return OutcomeError, "connection_failed"
}

// redactError returns the request error inside err with credentials removed from the URL it names, or err
// itself when it holds no request error
func redactError(err error) error {
	var urlErr *url.Error
	if !errors.As(err, &urlErr) {
		return err
	}
	redacted := *urlErr
	redacted.URL = tracing.RedactURL(urlErr.URL)
	return &redacted
}

// isTimeout reports whether an error is a network timeout
func isTimeout(err error) bool {
	var netErr net.Error
	return errors.As(err, &netErr) && netErr.Timeout()
}
