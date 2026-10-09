package webfetch

import (
	"bytes"
	"context"
	"encoding/json"
	"strings"
	"unicode/utf8"

	"codeberg.org/readeck/go-readability/v2/render"
	"github.com/Scrin/siikabot/tracing"
	"github.com/rs/zerolog/log"
	"golang.org/x/net/html"
)

const (
	// thinText is the least text a page can have and still count as read. Less is usually an app that builds
	// its content with JavaScript.
	thinText = 200
	// botCheckText is the most text a page with a bot check's signature can have and still be taken for the
	// check itself. Ordinary pages can carry the same widgets, in a contact form for example, around a lot
	// more text.
	botCheckText = 1000
	// maxExcerpt is how much of an error response the model gets. APIs often explain in it exactly what was
	// wrong.
	maxExcerpt = 500
)

// botChecks are the signatures bot checks leave in a page, by who runs the check, matched against the page's
// lowercased markup. Cloudflare's challenge-platform script isn't one: Cloudflare adds it to ordinary pages
// too.
var botChecks = []struct{ name, marker string }{
	{"Cloudflare", "<title>just a moment"},
	{"Cloudflare", "_cf_chl_opt"},
	{"Cloudflare", "attention required! | cloudflare"},
	{"Cloudflare Turnstile", `class="cf-turnstile`},
	{"DataDome", "captcha-delivery.com"},
	{"HUMAN", "px-captcha"},
	{"Akamai", "errors.edgesuite.net"},
	{"Vercel", "vercel security checkpoint"},
	{"Reddit", "js_challenge"},
	{"Reddit", "blocked by network security"},
	{"hCaptcha", `class="h-captcha`},
	{"reCAPTCHA", `class="g-recaptcha`},
}

// botCheck names who runs the bot check a page shows, or returns "" when it shows none
func botCheck(body []byte) string {
	lower := bytes.ToLower(body)
	for _, check := range botChecks {
		if bytes.Contains(lower, []byte(check.marker)) {
			return check.name
		}
	}
	return ""
}

// judge reads an answer and decides how the fetch went
func judge(ctx context.Context, page response, result Result) Result {
	status := page.statusCode
	if status >= 400 {
		return judgeError(page, result)
	}
	if status < 200 || status >= 300 {
		// A redirect without a destination, or some other answer that isn't a page
		result.Outcome, result.Reason = OutcomeError, "unexpected_status"
		return result
	}

	mediaType := page.mediaType()
	switch {
	case isHTML(mediaType):
		return readHTML(ctx, page, result)
	case mediaType == "application/pdf":
		result.MediaType = mediaType
		return readPDF(ctx, page, result)
	case isText(mediaType):
		result.MediaType = mediaType
		result.Content = strings.TrimSpace(page.text())
		result.Outcome = OutcomeOK
		if result.Content == "" {
			result.Outcome, result.Reason = OutcomeThin, "empty"
		}
		return result
	}
	result.Outcome, result.Reason = OutcomeUnsupported, mediaType
	return result
}

// judgeError decides what an error status means
func judgeError(page response, result Result) Result {
	check := botCheck(page.body)
	switch status := page.statusCode; {
	case status == 401 || status == 403 || status == 429:
		result.Outcome, result.Reason = OutcomeBlocked, check
	case status == 404 || status == 410:
		result.Outcome = OutcomeNotFound
		return result
	case status == 503 && check != "":
		result.Outcome, result.Reason = OutcomeBlocked, check
	case status < 500:
		result.Outcome = OutcomeRejected
	default:
		result.Outcome = OutcomeError
	}
	// A bot check's text says nothing about the page
	if check == "" {
		result.Content = excerpt(page)
	}
	return result
}

// excerpt is the start of an error response's text, for what it says went wrong. Markup goes, HTML's and
// XML's alike: an XML error, like those of FMI's API, has its explanation after a long preamble of it.
func excerpt(page response) string {
	text := page.text()
	if mediaType := page.mediaType(); isHTML(mediaType) || mediaType == "application/xml" ||
		mediaType == "text/xml" || strings.HasSuffix(mediaType, "+xml") {
		if doc, err := html.Parse(strings.NewReader(text)); err == nil {
			removeElements(doc, unwantedElements)
			text = render.InnerText(doc)
		}
	}
	text = strings.Join(strings.Fields(text), " ")
	if utf8.RuneCountInString(text) <= maxExcerpt {
		return text
	}
	cut := 0
	for range maxExcerpt {
		_, size := utf8.DecodeRuneInString(text[cut:])
		cut += size
	}
	return text[:cut] + "…"
}

// readHTML finds an HTML page's main content and judges it
func readHTML(ctx context.Context, page response, result Result) Result {
	doc, err := html.Parse(strings.NewReader(page.text()))
	if err != nil {
		// The page is already in memory, so this is markup the parser refuses, such as elements nested
		// hundreds deep
		log.Warn().Ctx(ctx).Err(err).Str("url", tracing.RedactURL(page.url.String())).Msg("Failed to parse a web page")
		result.Outcome, result.Reason = OutcomeError, "unparseable"
		return result
	}
	facts := factsOf(doc)
	result.Title, result.Language = facts.title, facts.language
	removeElements(doc, invisibleElements)
	pageText := textLength(doc)

	if pageText < botCheckText {
		if check := botCheck(page.body); check != "" {
			result.Outcome, result.Reason = OutcomeBlocked, check
			return result
		}
	}

	content, length, isArticle := mainContent(ctx, doc, page.url, &result, pageText)
	result.Content = content
	switch {
	case facts.paywalled:
		result.Outcome = OutcomePaywalled
	case length < thinText:
		result.Outcome = OutcomeThin
		if facts.asksForJavaScript || facts.scripts > 0 {
			result.Reason = "javascript"
		}
	case !isArticle && facts.asksForJavaScript:
		// No main content, only whatever surrounds it, on a page that asks for JavaScript: an app that
		// builds its content in the browser
		result.Outcome, result.Reason = OutcomeThin, "javascript"
	default:
		result.Outcome = OutcomeOK
	}
	return result
}

// paywalled reports whether a page's JSON-LD says the page isn't free to read. Only what it says about the
// page as a whole counts: news sites also mark the parts of a page that are for subscribers, and a free
// article can have those.
func paywalled(jsonLD string) bool {
	var data any
	if err := json.Unmarshal([]byte(jsonLD), &data); err != nil {
		return false
	}
	for _, item := range topLevelItems(data) {
		switch free := item["isAccessibleForFree"].(type) {
		case bool:
			if !free {
				return true
			}
		case string:
			if strings.EqualFold(free, "false") {
				return true
			}
		}
	}
	return false
}

// topLevelItems lists the items a JSON-LD block describes directly: the block itself, the items of an array,
// and the items of an @graph
func topLevelItems(data any) []map[string]any {
	var items []map[string]any
	switch value := data.(type) {
	case []any:
		for _, element := range value {
			items = append(items, topLevelItems(element)...)
		}
	case map[string]any:
		items = append(items, value)
		if graph, ok := value["@graph"]; ok {
			items = append(items, topLevelItems(graph)...)
		}
	}
	return items
}
