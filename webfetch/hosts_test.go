package webfetch

import (
	"testing"
	"time"
)

func TestDeniedSite(t *testing.T) {
	for host, want := range map[string]string{
		"reddit.com":      "Reddit",
		"www.reddit.com":  "Reddit",
		"old.Reddit.com":  "Reddit",
		"reddit.com.":     "Reddit",
		"redd.it":         "Reddit",
		"i.redd.it":       "Reddit",
		"notreddit.com":   "",
		"reddit.com.evil": "",
		"yle.fi":          "",
	} {
		if got := deniedSite(host); got != want {
			t.Errorf("deniedSite(%q) = %q, want %q", host, got, want)
		}
	}
}

func TestBlockMemory(t *testing.T) {
	forgetBlocks(t)

	rememberBlock("Example.com", blockedHost{statusCode: 429})
	if block, ok := recentBlock("example.com"); !ok || block.statusCode != 429 {
		t.Fatalf("recentBlock = %+v, %v; want the block", block, ok)
	}
	if _, ok := recentBlock("other.example.com"); ok {
		t.Error("another host is remembered as blocked")
	}

	// Once the memory runs out, the host is asked again
	blockedHostsMutex.Lock()
	block := blockedHosts["example.com"]
	block.until = time.Now().Add(-time.Second)
	blockedHosts["example.com"] = block
	blockedHostsMutex.Unlock()
	if _, ok := recentBlock("example.com"); ok {
		t.Error("an expired block is still remembered")
	}
}

func TestRemembersBlock(t *testing.T) {
	for _, test := range []struct {
		status int
		reason string
		want   bool
	}{
		{429, "", true},
		{403, "Cloudflare", true},
		{200, "reCAPTCHA", true},
		{403, "", false},
		{401, "", false},
	} {
		if got := remembersBlock(test.status, test.reason); got != test.want {
			t.Errorf("remembersBlock(%d, %q) = %v, want %v", test.status, test.reason, got, test.want)
		}
	}
}
