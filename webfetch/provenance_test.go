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
