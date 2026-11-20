package llmtools

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"strings"
	"time"

	"github.com/Scrin/siikabot/aigateway"
	"github.com/Scrin/siikabot/webfetch"
	"github.com/likexian/whois"
	"github.com/rs/zerolog/log"
)

// WhoisToolDefinition returns the tool definition for the whois tool
var WhoisToolDefinition = aigateway.ToolDefinition{
	Type: "function",
	Function: aigateway.FunctionSchema{
		Name:        "whois_lookup",
		Description: "Look up WHOIS information for a domain name or IP address. Returns registration details, nameservers, and other public registration data.",
		Parameters: json.RawMessage(`{
			"type": "object",
			"properties": {
				"query": {
					"type": "string",
					"description": "The domain name or IP address to look up (e.g., example.com, 8.8.8.8)"
				}
			},
			"required": ["query"]
		}`),
	},
	Handler:          handleWhoisToolCall,
	ValidityDuration: 1 * time.Hour,
}

// handleWhoisToolCall handles whois tool calls
func handleWhoisToolCall(ctx context.Context, arguments string) (string, error) {
	var args struct {
		Query string `json:"query"`
	}

	log.Debug().Ctx(ctx).Str("arguments", arguments).Msg("Received whois tool call")

	if err := json.Unmarshal([]byte(arguments), &args); err != nil {
		log.Error().Ctx(ctx).Err(err).Str("arguments", arguments).Msg("Failed to parse whois tool arguments")
		return "", fmt.Errorf("failed to parse arguments: %w", err)
	}

	if args.Query == "" {
		return "", fmt.Errorf("query is required")
	}

	query := strings.TrimSpace(args.Query)

	// A query reaches the whois servers for the name, and through their referrals, servers that whoever holds
	// the name or address can choose. So it's a way to send data out, like a fetch.
	if !webfetch.ProvenanceFrom(ctx).AllowsName(query) {
		log.Info().Ctx(ctx).Str("query", query).Msg("Refused a WHOIS lookup of a name the turn hasn't seen")
		return webfetch.DeniedNameMessage, nil
	}

	log.Debug().Ctx(ctx).Str("query", query).Msg("Performing WHOIS lookup")

	result, err := newWhoisClient(ctx).Whois(query)
	if err != nil {
		log.Error().Ctx(ctx).Err(err).Str("query", query).Msg("Failed to perform WHOIS lookup")
		return "", fmt.Errorf("WHOIS lookup failed: %w", err)
	}

	// Truncate if the response is too long
	const maxResponseLength = 8000
	if len(result) > maxResponseLength {
		result = result[:maxResponseLength] + "\n\n[Response truncated]"
	}

	log.Debug().Ctx(ctx).Str("query", query).Int("response_length", len(result)).Msg("WHOIS lookup completed")

	// Whoever holds the name or address writes much of its record
	return fmt.Sprintf("**WHOIS lookup for %s**\n\n%s\n\n```\n%s\n```", query, webfetch.UntrustedLabel, result), nil
}

const (
	// whoisPort is the port whois servers answer on. Referrals can name any port, and the bot connects to
	// no other.
	whoisPort = "43"
	// whoisTimeout bounds each connection of a lookup: the first, and the referral after it
	whoisTimeout = 10 * time.Second
	// maxWhoisResponseBytes is the most read from one whois server
	maxWhoisResponseBytes = 256 * 1024
)

// newWhoisClient makes a whois client for one lookup. The client follows referrals, and a record's holder can
// choose where some of them point, so it connects only to public addresses on the whois port, reads only so
// much, and gives up when the turn does.
func newWhoisClient(ctx context.Context) *whois.Client {
	return whois.NewClient().
		SetDialer(whoisDialer{ctx: ctx, dialer: webfetch.GuardedDialer(whoisTimeout)}).
		SetTimeout(whoisTimeout)
}

// whoisDialer connects a whois client to whois servers (newWhoisClient)
type whoisDialer struct {
	ctx    context.Context
	dialer *net.Dialer
}

// Dial connects to a whois server on a public address and the whois port
func (d whoisDialer) Dial(network, address string) (net.Conn, error) {
	_, port, err := net.SplitHostPort(address)
	if err != nil {
		return nil, err
	}
	if port != whoisPort {
		return nil, fmt.Errorf("whois servers are reached on port %s only, not %s", whoisPort, port)
	}
	conn, err := d.dialer.DialContext(d.ctx, network, address)
	if err != nil {
		return nil, err
	}
	stop := context.AfterFunc(d.ctx, func() { conn.Close() })
	return &limitedConn{Conn: conn, remaining: maxWhoisResponseBytes, stop: stop}, nil
}

// limitedConn is a connection that reads at most so many bytes, and stops closing itself when the turn ends
// once it's closed
type limitedConn struct {
	net.Conn
	remaining int
	stop      func() bool
}

func (c *limitedConn) Read(b []byte) (int, error) {
	if c.remaining <= 0 {
		return 0, io.EOF
	}
	if len(b) > c.remaining {
		b = b[:c.remaining]
	}
	n, err := c.Conn.Read(b)
	c.remaining -= n
	return n, err
}

func (c *limitedConn) Close() error {
	c.stop()
	return c.Conn.Close()
}
