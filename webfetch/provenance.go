package webfetch

import (
	"context"
	"net/netip"
	"net/url"
	"regexp"
	"strings"
)

// Provenance is the rule for which URLs a chat turn may fetch, and which names it may look up. Once the turn
// has read content from outside the room, such as a web page, it may fetch only URLs it has seen: ones in the
// room's messages or in tool results. Content can then tell the model to fetch a URL, but not get it to make
// one up, so a page can't have the model send what the room said to an address of the page's choosing. Looking
// up a name, as DNS and whois do, reaches whoever runs it just as a fetch does, so names follow the same rule.
//
// What the model wrote itself never counts as seen, even when a tool repeats it back: a tool that echoes its
// arguments, such as a search showing its query or a refusal showing the URL it refused, would otherwise let the
// model make any URL seen by writing it once. So the turn's messages are taken in order, the model's own (Wrote)
// as well as the room's and the tools' (See), and a URL or name the model wrote before the turn had seen it
// stays unseen.
//
// Before the turn reads any outside content, the model may fetch URLs it builds itself, such as a Wikipedia
// article's.
type Provenance struct {
	restricted bool
	// seen holds the URLs the turn may fetch, by urlKey, each as it was first seen
	seen map[string]*url.URL
	// names holds the host names and IP addresses the turn has seen
	names map[string]bool
	// writtenURLs and writtenNames hold what the model wrote before the turn had seen it
	writtenURLs  map[string]bool
	writtenNames map[string]bool
}

// NewProvenance makes the rule for a turn. restricted says the turn has read content from outside the room,
// and texts are where the URLs and names it may use then come from. More can be added in order with See and
// Wrote.
func NewProvenance(restricted bool, texts ...string) Provenance {
	p := Provenance{restricted: restricted}
	if restricted {
		p.seen = map[string]*url.URL{}
		p.names = map[string]bool{}
		p.writtenURLs = map[string]bool{}
		p.writtenNames = map[string]bool{}
	}
	for _, text := range texts {
		p.See(text)
	}
	return p
}

// See takes in text from the room or from a tool: the URLs and names in it become ones the turn may use, except
// those the model wrote before
func (p *Provenance) See(text string) {
	if !p.restricted {
		return
	}
	for _, u := range urlsIn(text) {
		if key := urlKey(u); !p.writtenURLs[key] && p.seen[key] == nil {
			p.seen[key] = u
		}
	}
	for _, name := range namesIn(text) {
		if !p.writtenNames[name] {
			p.names[name] = true
		}
	}
}

// Wrote takes in text the model wrote: its messages, and the arguments of its tool calls. A URL or name in it
// that the turn hasn't seen yet then never becomes seen.
func (p *Provenance) Wrote(text string) {
	if !p.restricted {
		return
	}
	for _, u := range urlsIn(text) {
		if key := urlKey(u); p.seen[key] == nil {
			p.writtenURLs[key] = true
		}
	}
	for _, name := range namesIn(text) {
		if !p.names[name] {
			p.writtenNames[name] = true
		}
	}
}

// DeniedNameMessage is what a tool says when the rule refuses a name it was asked to look up
const DeniedNameMessage = "This name hasn't appeared in the conversation or in a tool result. Once a turn has read " +
	"content from the web, the bot looks up only names it has seen, so that content can't make it look up names of " +
	"its own: a lookup tells whoever runs the name everything in it. Ask the user for the name instead."

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
	_, ok := p.Resolve(u)
	return ok
}

// Resolve returns the URL to fetch for the one asked for, if the rule allows it. Until the turn is restricted,
// that's the URL as asked for. After, it's the URL as the turn saw it: the two only have to match in urlKey's
// terms, and the difference between them, such as how the path is encoded or the order of the query, could
// otherwise carry data out.
func (p Provenance) Resolve(u *url.URL) (*url.URL, bool) {
	if !p.restricted {
		return u, true
	}
	seen := p.seen[urlKey(u)]
	if seen == nil {
		return nil, false
	}
	resolved := *seen
	resolved.User = nil
	resolved.Fragment, resolved.RawFragment = "", ""
	return &resolved, true
}

// AllowsName reports whether the rule lets a host name or IP address be looked up. In a restricted turn, the
// name has to be one the turn has seen, or a domain one it has seen is under.
func (p Provenance) AllowsName(name string) bool {
	if !p.restricted {
		return true
	}
	name, ok := normalizeName(name)
	if !ok {
		return false
	}
	if p.names[name] {
		return true
	}
	if _, err := netip.ParseAddr(name); err == nil {
		return false
	}
	for seen := range p.names {
		if strings.HasSuffix(seen, "."+name) {
			return true
		}
	}
	return false
}

// urlCandidates finds what may be URLs in text: anything starting with http:// or https://, and host names
// followed by an optional path, since people often write a site as "yle.fi" or "www.hs.fi/talous". Taking
// too much in is harmless, as long as everything taken in was in the text.
var urlCandidates = regexp.MustCompile(`(?i)https?://[^\s<>"'` + "`" + `]+|\b(?:[a-z0-9-]+\.)+[a-z]{2,}(?:/[^\s<>"'` + "`" + `]*)?`)

// addressCandidates finds what may be IP addresses in text: IPv4 addresses, and runs of the characters of an
// IPv6 address with at least two colons in them
var addressCandidates = regexp.MustCompile(`\b(?:\d{1,3}\.){3}\d{1,3}\b|[0-9A-Fa-f.:]*:[0-9A-Fa-f.:]*:[0-9A-Fa-f.:]*`)

// urlsIn returns the URLs in text
func urlsIn(text string) []*url.URL {
	var urls []*url.URL
	for _, candidate := range urlCandidates.FindAllString(text, -1) {
		if u, ok := parseCandidate(candidate); ok {
			urls = append(urls, u)
		}
	}
	return urls
}

// namesIn returns the host names and IP addresses in text, normalised: the hosts of its URLs, and the
// addresses in it
func namesIn(text string) []string {
	var names []string
	for _, u := range urlsIn(text) {
		if name, ok := normalizeName(u.Hostname()); ok {
			names = append(names, name)
		}
	}
	for _, candidate := range addressCandidates.FindAllString(text, -1) {
		if addr, err := netip.ParseAddr(strings.Trim(candidate, ".:")); err == nil {
			names = append(names, addr.Unmap().String())
		}
	}
	return names
}

// normalizeName puts a host name or IP address in the form names are compared in: a name in lower case without
// a trailing dot, an address as netip writes it
func normalizeName(name string) (string, bool) {
	name = strings.TrimSuffix(strings.Trim(strings.TrimSpace(name), "[]"), ".")
	if name == "" {
		return "", false
	}
	if addr, err := netip.ParseAddr(name); err == nil {
		return addr.Unmap().String(), true
	}
	return strings.ToLower(name), true
}

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
// scheme doesn't count, so a page written as "yle.fi" can be fetched over https. A URL that matches a seen
// one is fetched as it was seen (Provenance.Resolve), so none of this lets the model make up anything of its
// own.
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
