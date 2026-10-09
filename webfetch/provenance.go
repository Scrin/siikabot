package webfetch

import (
	"context"
	"net/url"
	"regexp"
	"strings"
)

// Provenance is the rule for which URLs a chat turn may fetch. Once the turn has read content from outside the
// room, such as a web page, it may fetch only URLs it has seen: ones in the room's messages or in tool
// results. Content can then tell the model to fetch a URL, but not get it to make one up, so a page can't
// have the model send what the room said to an address of the page's choosing.
//
// Before the turn reads any outside content, the model may fetch URLs it builds itself, such as a Wikipedia
// article's.
type Provenance struct {
	restricted bool
	seen       map[string]bool
}

// NewProvenance makes the rule for a turn. restricted says the turn has read content from outside the room,
// and texts are where the URLs it may fetch then come from.
func NewProvenance(restricted bool, texts ...string) Provenance {
	p := Provenance{restricted: restricted, seen: map[string]bool{}}
	if !restricted {
		return p
	}
	for _, text := range texts {
		for _, candidate := range urlCandidates.FindAllString(text, -1) {
			if u, ok := parseCandidate(candidate); ok {
				p.seen[urlKey(u)] = true
			}
		}
	}
	return p
}

type provenanceKey struct{}

// WithProvenance gives a context the rule for its turn
func WithProvenance(ctx context.Context, p Provenance) context.Context {
	return context.WithValue(ctx, provenanceKey{}, p)
}

// ProvenanceFrom returns the rule a context carries. Without one, any URL is allowed: fetches outside a chat
// turn aren't the model's.
func ProvenanceFrom(ctx context.Context) Provenance {
	p, _ := ctx.Value(provenanceKey{}).(Provenance)
	return p
}

// Allows reports whether the rule lets a URL be fetched
func (p Provenance) Allows(u *url.URL) bool {
	return !p.restricted || p.seen[urlKey(u)]
}

// urlCandidates finds what may be URLs in text: anything starting with http:// or https://, and host names
// followed by an optional path, since people often write a site as "yle.fi" or "www.hs.fi/talous". Taking
// too much in is harmless, as long as everything taken in was in the text.
var urlCandidates = regexp.MustCompile(`(?i)https?://[^\s<>"'` + "`" + `]+|\b(?:[a-z0-9-]+\.)+[a-z]{2,}(?:/[^\s<>"'` + "`" + `]*)?`)

// parseCandidate turns a candidate into a URL, after dropping the punctuation around a URL in prose:
// a sentence's full stop, or the closing parenthesis of a markdown link
func parseCandidate(candidate string) (*url.URL, bool) {
	candidate = trimTrailing(candidate)
	if !strings.Contains(strings.ToLower(candidate), "://") {
		candidate = "https://" + candidate
	}
	u, err := url.Parse(candidate)
	if err != nil || u.Hostname() == "" {
		return nil, false
	}
	return u, true
}

// trimTrailing drops trailing punctuation from a URL candidate, and closing brackets that have no opening one
// in it
func trimTrailing(candidate string) string {
	for {
		trimmed := strings.TrimRight(candidate, ".,;:!?*_~")
		switch {
		case strings.HasSuffix(trimmed, ")") && strings.Count(trimmed, ")") > strings.Count(trimmed, "("):
			trimmed = trimmed[:len(trimmed)-1]
		case strings.HasSuffix(trimmed, "]") && strings.Count(trimmed, "]") > strings.Count(trimmed, "["):
			trimmed = trimmed[:len(trimmed)-1]
		}
		if trimmed == candidate {
			return candidate
		}
		candidate = trimmed
	}
}

// urlKey is what two URLs have to share to count as the same: the host, without "www.", with the port when
// it isn't the default; the path, decoded and without a trailing slash; and the query, normalised. The
// scheme doesn't count, so a page written as "yle.fi" can be fetched over https. None of this lets the model
// make up anything of its own.
func urlKey(u *url.URL) string {
	host := strings.TrimPrefix(strings.TrimSuffix(strings.ToLower(u.Hostname()), "."), "www.")
	if port := u.Port(); port != "" && port != "80" && port != "443" {
		host += ":" + port
	}

	key := host + strings.TrimSuffix(u.Path, "/")
	if u.RawQuery != "" {
		query := u.RawQuery
		if values, err := url.ParseQuery(u.RawQuery); err == nil {
			query = values.Encode()
		}
		key += "?" + query
	}
	return key
}
