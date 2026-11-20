package federation

import "testing"

// People choose their display names, and they end up in the bot's HTML
func TestUserLinkEscapesTheDisplayName(t *testing.T) {
	got := userLink("@mallory:example.com", `<a href="https://evil.example">Admin</a>`)
	want := `<a href="https://matrix.to/#/@mallory:example.com">&lt;a href=&#34;https://evil.example&#34;&gt;Admin&lt;/a&gt;</a>`
	if got != want {
		t.Errorf("userLink() = %s, want %s", got, want)
	}
}
