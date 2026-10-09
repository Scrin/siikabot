package webfetch

import (
	"fmt"
	"strings"
	"testing"
	"time"
	"unicode/utf8"
)

func TestWindowCutsAtTheLastParagraphBreak(t *testing.T) {
	content := strings.Repeat("a", 70) + "\n\n" + strings.Repeat("b", 10) + "\n\n" + strings.Repeat("c", 100)

	part, start, end := window(content, 0, 100)

	if want := strings.Repeat("a", 70) + "\n\n" + strings.Repeat("b", 10); part != want || start != 0 || end != 84 {
		t.Errorf("window = %q, %d, %d; want the first two paragraphs, 0, 84", part, start, end)
	}
}

func TestWindowFallsBackToLinesSentencesAndWords(t *testing.T) {
	for _, test := range []struct {
		name    string
		content string
		wantEnd int
	}{
		{"line", strings.Repeat("a", 80) + "\n" + strings.Repeat("b", 100), 81},
		// A sentence's end beats a later gap between words
		{"sentence", strings.Repeat("a", 75) + ". " + strings.Repeat("b", 3) + " " + strings.Repeat("c", 100), 77},
		{"word", strings.Repeat("a", 75) + " " + strings.Repeat("b", 100), 76},
		// A paragraph break too early in the window isn't worth wasting the rest of it for
		{"early_break", strings.Repeat("a", 20) + "\n\n" + strings.Repeat("b", 75) + " " + strings.Repeat("c", 100), 98},
		{"no_break", strings.Repeat("a", 200), 100},
	} {
		t.Run(test.name, func(t *testing.T) {
			if _, _, end := window(test.content, 0, 100); end != test.wantEnd {
				t.Errorf("the window ends at %d, want %d", end, test.wantEnd)
			}
		})
	}
}

// Windows never cut a character in two, whatever the limit and offset
func TestWindowKeepsCharactersWhole(t *testing.T) {
	content := strings.Repeat("äö€😀", 50)
	for limit := 1; limit < 12; limit++ {
		for offset := 0; offset < 12; offset++ {
			part, _, end := window(content, offset, limit)
			if !utf8.ValidString(part) || part == "" || (end < len(content) && !utf8.RuneStart(content[end])) {
				t.Fatalf("window(%d, %d) = %q ending at %d", offset, limit, part, end)
			}
		}
	}
}

// Reading on from where each window ends goes through the whole content
func TestWindowsCoverTheContent(t *testing.T) {
	var content strings.Builder
	for i := range 200 {
		fmt.Fprintf(&content, "Paragraph %d has some words in it, and one sentence. Then another one.\n\n", i)
	}

	offset, windows := 0, 0
	for offset < content.Len() {
		_, start, end := window(content.String(), offset, 1000)
		if start != offset || end <= start {
			t.Fatalf("a window from %d covers %d to %d", offset, start, end)
		}
		offset = end
		windows++
	}
	if windows < content.Len()/1000 {
		t.Errorf("%d windows for %d bytes", windows, content.Len())
	}
}

func TestRenderPages(t *testing.T) {
	result := Result{
		URL:       "https://yle.fi/a/74-20227051",
		Outcome:   OutcomeOK,
		Title:     "Hautaamisen hinnat",
		Site:      "Yle Uutiset",
		Published: time.Date(2026, 5, 22, 8, 27, 0, 0, time.FixedZone("", 3*3600)),
		Language:  "fi",
		Content:   strings.Repeat("Sana sanalta. ", 100),
	}

	first := Render(result, 0, 500)
	if want := "URL: https://yle.fi/a/74-20227051\n" +
		"Title: Hautaamisen hinnat | Site: Yle Uutiset | Published: 2026-05-22 08:27 +03:00 | Language: fi\n" +
		UntrustedLabel + "\n---\n"; !strings.HasPrefix(first, want) {
		t.Errorf("the page starts:\n%s\nwant:\n%s", first, want)
	}
	if want := "\n---\n[Showing bytes 0–490 of 1400. Call again with offset=490 to read on.]"; !strings.HasSuffix(first, want) {
		t.Errorf("the first part ends:\n%s\nwant:\n%s", first[len(first)-100:], want)
	}

	last := Render(result, 1000, 500)
	if want := "[Showing bytes 1000–1400 of 1400, the end of the content.]"; !strings.HasSuffix(last, want) {
		t.Errorf("the last part ends:\n%s\nwant %s", last[len(last)-100:], want)
	}

	whole := Render(result, 0, 2000)
	if strings.Contains(whole, "[Showing") {
		t.Errorf("a page shown whole says it's in parts:\n%s", whole)
	}

	past := Render(result, 5000, 500)
	if !strings.HasSuffix(past, "The offset 5000 is past the end: the content is 1400 bytes long.") {
		t.Errorf("an offset past the end gives:\n%s", past)
	}
}

func TestRenderNotes(t *testing.T) {
	for _, test := range []struct {
		result Result
		want   string
	}{
		{Result{Outcome: OutcomePaywalled, Content: "Teaser"}, "Note: the article is for subscribers."},
		{Result{Outcome: OutcomeThin, Reason: "javascript", Content: "Menu"}, "What it does show is below."},
		{Result{Outcome: OutcomeThin, Reason: "javascript"}, "Note: the page shows nothing without JavaScript"},
		{Result{Outcome: OutcomeThin, Reason: "no_text", MediaType: "application/pdf"}, "it's probably scanned"},
		{Result{Outcome: OutcomeThin, Content: "Hi"}, "Note: the page has very little text."},
		{Result{Outcome: OutcomeOK, Published: time.Date(2026, 10, 9, 0, 0, 0, 0, time.UTC), Content: "x"}, "Published: 2026-10-09\n"},
	} {
		if got := Render(test.result, 0, 1000); !strings.Contains(got, test.want) {
			t.Errorf("Render(%s, %s) lacks %q:\n%s", test.result.Outcome, test.result.Reason, test.want, got)
		}
	}
}

func TestRenderFailures(t *testing.T) {
	for _, test := range []struct {
		result Result
		want   []string
	}{
		{Result{Outcome: OutcomeBlocked, Host: "downdetector.com", StatusCode: 403},
			[]string{"downdetector.com refused automated access (HTTP 403).", "Don't retry it."}},
		{Result{Outcome: OutcomeBlocked, Host: "example.com", StatusCode: 403, Reason: "Cloudflare"},
			[]string{"example.com showed a bot check (Cloudflare) instead of the page."}},
		{Result{Outcome: OutcomeBlocked, Host: "example.com", StatusCode: 429, Remembered: true},
			[]string{"blocked the bot within the last few hours, so it wasn't asked again."}},
		{Result{Outcome: OutcomeBlocked, Host: "api.example.com", StatusCode: 429, Content: "Rate limit exceeded"},
			[]string{"is limiting how often the bot may ask (HTTP 429).", `The site's response said: "Rate limit exceeded".`}},
		{Result{Outcome: OutcomeNotFound, StatusCode: 404}, []string{"There's no page at this URL (HTTP 404).", "web_search"}},
		{Result{Outcome: OutcomeRejected, Host: "opendata.fmi.fi", StatusCode: 400, Content: "Unrecognized parameter 'PM25_PT1M_1'"},
			[]string{"opendata.fmi.fi rejected the request (HTTP 400).", "Unrecognized parameter 'PM25_PT1M_1'"}},
		{Result{Outcome: OutcomeError, Host: "example.com", StatusCode: 502}, []string{"example.com failed to answer the request (HTTP 502)."}},
		{Result{Outcome: OutcomeError, Host: "exampel.com", Reason: "no_such_host"}, []string{"The host exampel.com doesn't exist."}},
		{Result{Outcome: OutcomeBadCertificate, Host: "ilmanlaatu.hsy.fi", Reason: "expired"},
			[]string{"ilmanlaatu.hsy.fi's security certificate has expired, so the bot won't read the page."}},
		{Result{Outcome: OutcomeUnsupported, Reason: "image/png"}, []string{"The URL points to image/png content, which this tool can't read."}},
		{Result{Outcome: OutcomeUnsupported, Reason: "too_large"}, []string{"The PDF is larger than 10 MB"}},
		{Result{Outcome: OutcomeInvalidURL, Reason: "it has no host name"}, []string{"This URL can't be fetched: it has no host name."}},
		{Result{Outcome: OutcomeDeniedAddress}, []string{"private or local network address"}},
		{Result{Outcome: OutcomeDeniedHost, Reason: "Reddit"}, []string{"The bot can't read Reddit"}},
		{Result{Outcome: OutcomeDeniedProvenance}, []string{"only URLs it has seen", "web_search", "ask the user for the link"}},
	} {
		got := Render(test.result, 0, 1000)
		for _, want := range test.want {
			if !strings.Contains(got, want) {
				t.Errorf("Render(%s) lacks %q:\n%s", test.result.Outcome, want, got)
			}
		}
		if strings.Contains(got, UntrustedLabel) && test.result.Content == "" {
			t.Errorf("Render(%s) labels content that isn't there:\n%s", test.result.Outcome, got)
		}
	}
}
