package webfetch

import (
	"strings"
	"sync"
	"time"
)

// deniedSites are sites the bot doesn't fetch from at all, by domain, with the name the model is told. Reddit
// blocks the bot's requests whatever they look like, so asking only wastes a tool call.
var deniedSites = map[string]string{
	"reddit.com": "Reddit",
	"redd.it":    "Reddit",
}

// deniedSite names the site a host belongs to if the bot doesn't fetch from it, or returns ""
func deniedSite(host string) string {
	host = strings.TrimSuffix(strings.ToLower(host), ".")
	for domain, name := range deniedSites {
		if host == domain || strings.HasSuffix(host, "."+domain) {
			return name
		}
	}
	return ""
}

const (
	// blockMemory is how long the bot remembers that a host blocked it
	blockMemory = 3 * time.Hour
	// maxRememberedBlocks bounds the memory of blocks. Hosts are unbounded.
	maxRememberedBlocks = 1000
)

// blockedHost is a block the bot remembers
type blockedHost struct {
	statusCode int
	// reason names who ran the bot check, if the block was one
	reason string
	until  time.Time
}

var (
	blockedHosts      = map[string]blockedHost{}
	blockedHostsMutex sync.Mutex
)

// remembersBlock reports whether a block is worth remembering: rate limiting and bot checks last a while and
// cover the whole site, while a plain refusal may be about the one page
func remembersBlock(statusCode int, reason string) bool {
	return statusCode == 429 || reason != ""
}

// rememberBlock remembers that a host blocked the bot
func rememberBlock(host string, block blockedHost) {
	block.until = time.Now().Add(blockMemory)

	blockedHostsMutex.Lock()
	defer blockedHostsMutex.Unlock()
	if len(blockedHosts) >= maxRememberedBlocks {
		blockedHosts = map[string]blockedHost{}
	}
	blockedHosts[strings.ToLower(host)] = block
}

// recentBlock returns the block a host gave the bot recently, if it did
func recentBlock(host string) (blockedHost, bool) {
	host = strings.ToLower(host)

	blockedHostsMutex.Lock()
	defer blockedHostsMutex.Unlock()
	block, ok := blockedHosts[host]
	if ok && time.Now().After(block.until) {
		delete(blockedHosts, host)
		return blockedHost{}, false
	}
	return block, ok
}
