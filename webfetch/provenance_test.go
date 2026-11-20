package webfetch

import (
	"context"
	"net/url"
	"testing"
)

func mustParse(t *testing.T, rawURL string) *url.URL {
	t.Helper()
	u, err := url.Parse(rawURL)
	if err != nil {
		t.Fatal(err)
	}
	return u
}

// Before a turn reads anything from outside the room, the model may fetch URLs it builds itself
func TestProvenanceAllowsAnyURLUntilRestricted(t *testing.T) {
	for _, p := range []Provenance{{}, NewProvenance(false, "nothing here")} {
		if !p.Allows(mustParse(t, "https://en.wikipedia.org/wiki/Siika")) {
			t.Error("an unrestricted turn refuses a URL")
		}
	}
	if !ProvenanceFrom(context.Background()).Allows(mustParse(t, "https://example.com/")) {
		t.Error("a context without a rule refuses a URL")
	}
}

func TestProvenanceAllowsOnlySeenURLs(t *testing.T) {
	p := NewProvenance(true,
		"Read https://yle.fi/a/74-20227051. Also (see https://example.com/page_(disambiguation)) and "+
			"[the docs](https://docs.example.com/guide/?b=2&a=1#install), or <https://angle.example.com/x>!",
		"Search results:\n### [Title](https://www.hs.fi/talous/art-2000011.html)\n*www.hs.fi*",
		"Check yle.fi and www.is.fi/uutiset/ please",
		"http://plain.example.com:8080/path/",
	)

	for _, test := range []struct {
		url     string
		allowed bool
	}{
		{"https://yle.fi/a/74-20227051", true},
		{"https://example.com/page_(disambiguation)", true},
		{"https://docs.example.com/guide?a=1&b=2", true},
		{"https://docs.example.com/guide/?b=2&a=1", true},
		{"https://angle.example.com/x", true},
		{"https://www.hs.fi/talous/art-2000011.html", true},
		{"https://hs.fi/talous/art-2000011.html", true},
		{"http://www.hs.fi/talous/art-2000011.html#comments", true},
		{"https://www.hs.fi/", true},
		{"https://yle.fi/", true},
		{"https://www.yle.fi", true},
		{"https://is.fi/uutiset", true},
		{"https://plain.example.com:8080/path", true},
		{"https://YLE.fi/a/74-20227051", true},

		// What the model would make up to carry something out
		{"https://yle.fi/a/74-20227051?q=secret", false},
		{"https://yle.fi/a/74-20227051/secret", false},
		{"https://secret.yle.fi/", false},
		{"https://attacker.example/?data=secret", false},
		{"https://docs.example.com/guide?a=1&b=2&c=3", false},
		{"https://plain.example.com:8081/path", false},
		{"https://plain.example.com/path", false},
		{"https://www.hs.fi/talous/", false},
	} {
		if got := p.Allows(mustParse(t, test.url)); got != test.allowed {
			t.Errorf("Allows(%s) = %v, want %v", test.url, got, test.allowed)
		}
	}
}

// A URL that matches a seen one is fetched as it was seen. Each of these differs from the seen URL in a way
// that urlKey treats as the same, and the difference could carry a bit of data out if it reached the request.
func TestProvenanceResolvesToTheURLAsSeen(t *testing.T) {
	seen := "https://attacker.example/aaaaaaaa?a=1&b=2"
	p := NewProvenance(true, "see "+seen)

	for _, asked := range []string{
		"https://attacker.example/%61a%61aa%61aa?a=1&b=2",
		"https://attacker.example/aaaaaaaa/?a=1&b=2",
		"http://attacker.example/aaaaaaaa?a=1&b=2",
		"https://www.attacker.example/aaaaaaaa?a=1&b=2",
		"https://attacker.example:443/aaaaaaaa?a=1&b=2",
		"https://attacker.example./aaaaaaaa?a=1&b=2",
		"https://ATTACKER.example/aaaaaaaa?a=1&b=2",
		"https://attacker.example/aaaaaaaa?b=2&a=1",
		"https://attacker.example/aaaaaaaa?a=%31&b=2",
		"https://attacker.example/aaaaaaaa?a=1&b=2&",
		"https://secret@attacker.example/aaaaaaaa?a=1&b=2",
		"https://attacker.example/aaaaaaaa?a=1&b=2#secret",
	} {
		resolved, ok := p.Resolve(mustParse(t, asked))
		if !ok {
			t.Errorf("Resolve(%s) refused a URL that matches the seen one", asked)
			continue
		}
		if resolved.String() != seen {
			t.Errorf("Resolve(%s) = %s, want the URL as seen, %s", asked, resolved, seen)
		}
	}
}

// Until the turn is restricted, the URL is fetched as the model asked for it
func TestProvenanceResolvesAnyURLUntilRestricted(t *testing.T) {
	asked := mustParse(t, "https://en.wikipedia.org/wiki/Siika?x=%61")
	if resolved, ok := NewProvenance(false).Resolve(asked); !ok || resolved != asked {
		t.Errorf("Resolve() = %v, %v, want the URL asked for", resolved, ok)
	}
}

// A tool that repeats what the model wrote, such as a refusal showing the URL it refused or a search showing
// its query, mustn't make it seen
func TestProvenanceNeverCountsWhatTheModelWroteAsSeen(t *testing.T) {
	secret := "https://attacker.example/c?d=room-secret"
	p := NewProvenance(true, "a page with a link to https://example.org/a")
	p.Wrote(`{"url": "` + secret + `"}`)
	p.See("URL: " + secret + "\nThis URL hasn't appeared in the conversation.")
	p.See(`Search results for "` + secret + `"`)

	if p.Allows(mustParse(t, secret)) {
		t.Error("a URL the model wrote counts as seen once a tool repeats it")
	}
	if !p.Allows(mustParse(t, "https://example.org/a")) {
		t.Error("the URL the page gave can't be fetched")
	}
}

// What the turn saw before the model wrote it stays seen: the model passes the URLs it found to the tools
func TestProvenanceKeepsWhatWasSeenBeforeTheModelWroteIt(t *testing.T) {
	p := NewProvenance(true, "a page with a link to https://example.org/a and to news.example.com")
	p.Wrote(`{"url": "https://example.org/a"}`)
	p.Wrote(`{"domain": "news.example.com"}`)
	p.See("URL: https://example.org/a")

	if !p.Allows(mustParse(t, "https://example.org/a")) {
		t.Error("a URL seen before the model wrote it can't be fetched")
	}
	if !p.AllowsName("news.example.com") {
		t.Error("a name seen before the model wrote it can't be looked up")
	}
}

func TestProvenanceAllowsOnlySeenNames(t *testing.T) {
	p := NewProvenance(true,
		"Read https://www.yle.fi/uutiset and mail.example.com, or ask 192.0.2.10 or 2001:db8::1.",
		"See http://[2001:db8::2]:8080/x")
	p.Wrote(`{"domain": "secret.attacker.example"}`)
	p.See("**DNS lookup for secret.attacker.example**")

	for name, allowed := range map[string]bool{
		"www.yle.fi":              true,
		"yle.fi":                  true,
		"WWW.YLE.FI.":             true,
		"mail.example.com":        true,
		"example.com":             true,
		"192.0.2.10":              true,
		"2001:db8::1":             true,
		"2001:DB8:0::1":           true,
		"[2001:db8::2]":           true,
		"news.yle.fi":             false,
		"secret.yle.fi":           false,
		"secret.attacker.example": false,
		"attacker.example":        false,
		"192.0.2.11":              false,
		"192.0":                   false,
		"":                        false,
	} {
		if got := p.AllowsName(name); got != allowed {
			t.Errorf("AllowsName(%q) = %v, want %v", name, got, allowed)
		}
	}
	if !NewProvenance(false).AllowsName("anything.example") {
		t.Error("an unrestricted turn refuses a name")
	}
}

func TestProvenanceTravelsInTheContext(t *testing.T) {
	ctx := WithProvenance(context.Background(), NewProvenance(true, "https://example.com/seen"))
	p := ProvenanceFrom(ctx)

	if !p.Allows(mustParse(t, "https://example.com/seen")) || p.Allows(mustParse(t, "https://example.com/other")) {
		t.Error("the rule taken from the context isn't the one put in it")
	}
}

func TestTrimTrailing(t *testing.T) {
	for candidate, want := range map[string]string{
		"https://example.com/a.":                  "https://example.com/a",
		"https://example.com/a),":                 "https://example.com/a",
		"https://en.wikipedia.org/wiki/A_(b)":     "https://en.wikipedia.org/wiki/A_(b)",
		"https://en.wikipedia.org/wiki/A_(b))":    "https://en.wikipedia.org/wiki/A_(b)",
		"https://example.com/[x]]":                "https://example.com/[x]",
		"https://example.com/*emphasis*":          "https://example.com/*emphasis",
		"https://example.com/path?query=1&b=2...": "https://example.com/path?query=1&b=2",
	} {
		if got := trimTrailing(candidate); got != want {
			t.Errorf("trimTrailing(%q) = %q, want %q", candidate, got, want)
		}
	}
}
