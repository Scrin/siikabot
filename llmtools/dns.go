package llmtools

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"strings"
	"time"

	"github.com/Scrin/siikabot/aigateway"
	"github.com/Scrin/siikabot/webfetch"
	"github.com/rs/zerolog/log"
	"golang.org/x/net/publicsuffix"
)

// dnsTimeout bounds a whole DNS lookup, every record type included
const dnsTimeout = 10 * time.Second

// DNSToolDefinition returns the tool definition for the DNS lookup tool
var DNSToolDefinition = aigateway.ToolDefinition{
	Type: "function",
	Function: aigateway.FunctionSchema{
		Name:        "dns_lookup",
		Description: "Look up DNS records for a public domain name. Returns A (IPv4), AAAA (IPv6), MX (mail servers), TXT, CNAME, and NS (nameserver) records.",
		Parameters: json.RawMessage(`{
			"type": "object",
			"properties": {
				"domain": {
					"type": "string",
					"description": "The domain name to look up (e.g., example.com, google.com)"
				},
				"record_type": {
					"type": "string",
					"enum": ["A", "AAAA", "MX", "TXT", "CNAME", "NS", "ALL"],
					"description": "The type of DNS record to look up. Use ALL to get all record types. Defaults to ALL if not specified."
				}
			},
			"required": ["domain"]
		}`),
	},
	Handler:          handleDNSToolCall,
	ValidityDuration: 5 * time.Minute,
}

// handleDNSToolCall handles DNS lookup tool calls
func handleDNSToolCall(ctx context.Context, arguments string) (string, error) {
	var args struct {
		Domain     string `json:"domain"`
		RecordType string `json:"record_type"`
	}

	log.Debug().Ctx(ctx).Str("arguments", arguments).Msg("Received DNS lookup tool call")

	if err := json.Unmarshal([]byte(arguments), &args); err != nil {
		log.Error().Ctx(ctx).Err(err).Str("arguments", arguments).Msg("Failed to parse DNS tool arguments")
		return "", fmt.Errorf("failed to parse arguments: %w", err)
	}

	if args.Domain == "" {
		return "", fmt.Errorf("domain is required")
	}

	domain := strings.TrimSpace(args.Domain)
	recordType := strings.ToUpper(strings.TrimSpace(args.RecordType))
	if recordType == "" {
		recordType = "ALL"
	}
	lookups, ok := dnsLookups[recordType]
	if !ok {
		return "", fmt.Errorf("invalid record type: %s (valid types: A, AAAA, MX, TXT, CNAME, NS, ALL)", recordType)
	}

	// A lookup reaches whoever runs the name's DNS, so it's a way to send data out, like a fetch
	if !webfetch.ProvenanceFrom(ctx).AllowsName(domain) {
		log.Info().Ctx(ctx).Str("domain", domain).Msg("Refused a DNS lookup of a name the turn hasn't seen")
		return webfetch.DeniedNameMessage, nil
	}
	if !isPublicDomain(domain) {
		log.Info().Ctx(ctx).Str("domain", domain).Msg("Refused a DNS lookup of a name that isn't public")
		return fmt.Sprintf("%s isn't a public domain name. This tool looks up only names under a public suffix, "+
			"such as .com or .fi, and never the bot's own network's names.", domain), nil
	}

	log.Debug().Ctx(ctx).Str("domain", domain).Str("record_type", recordType).Msg("Performing DNS lookup")

	ctx, cancel := context.WithTimeout(ctx, dnsTimeout)
	defer cancel()
	lookup := &dnsLookup{domain: domain, resolver: net.DefaultResolver}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("**DNS lookup for %s**\n\n", domain))
	// Whoever runs the domain writes its records, TXT records especially
	sb.WriteString(webfetch.UntrustedLabel + "\n\n")
	for i, record := range lookups {
		if i > 0 {
			sb.WriteString("\n")
		}
		sb.WriteString(record(ctx, lookup))
	}

	result := sb.String()
	log.Debug().Ctx(ctx).Str("domain", domain).Str("record_type", recordType).Int("response_length", len(result)).Msg("DNS lookup completed")

	return result, nil
}

// dnsLookups are the lookups to make for each record type the tool takes
var dnsLookups = map[string][]func(context.Context, *dnsLookup) string{
	"A":     {lookupA},
	"AAAA":  {lookupAAAA},
	"MX":    {lookupMX},
	"TXT":   {lookupTXT},
	"CNAME": {lookupCNAME},
	"NS":    {lookupNS},
	"ALL":   {lookupA, lookupAAAA, lookupMX, lookupTXT, lookupCNAME, lookupNS},
}

// isPublicDomain reports whether a name is under a public suffix, such as .com, .fi or github.io. Anything
// else, such as a single word, .internal or .local, can only be one of the bot's own network's names, which the
// resolver the bot uses would give away.
func isPublicDomain(domain string) bool {
	domain = strings.ToLower(strings.TrimSuffix(domain, "."))
	if domain == "" || net.ParseIP(domain) != nil {
		return false
	}
	suffix, icann := publicsuffix.PublicSuffix(domain)
	if suffix == "home.arpa" || strings.HasSuffix(suffix, ".home.arpa") {
		// Reserved for home networks, though ICANN manages .arpa
		return false
	}
	return icann || strings.Contains(suffix, ".")
}

// dnsLookup is a lookup of one domain, which A and AAAA records share
type dnsLookup struct {
	domain   string
	resolver *net.Resolver

	ips    []net.IPAddr
	ipsErr error
	looked bool
}

// addresses looks up the domain's addresses once, for both A and AAAA records
func (l *dnsLookup) addresses(ctx context.Context) ([]net.IPAddr, error) {
	if !l.looked {
		l.ips, l.ipsErr = l.resolver.LookupIPAddr(ctx, l.domain)
		l.looked = true
	}
	return l.ips, l.ipsErr
}

// lookupA returns A (IPv4) records for a domain
func lookupA(ctx context.Context, l *dnsLookup) string {
	ips, err := l.addresses(ctx)
	if err != nil {
		log.Debug().Ctx(ctx).Err(err).Str("domain", l.domain).Msg("Failed to look up A records")
		return fmt.Sprintf("**A Records (IPv4):**\nNo A records found or lookup failed: %s\n", err.Error())
	}

	var ipv4s []string
	for _, ip := range ips {
		if ipv4 := ip.IP.To4(); ipv4 != nil {
			ipv4s = append(ipv4s, ipv4.String())
		}
	}

	if len(ipv4s) == 0 {
		return "**A Records (IPv4):**\nNo A records found\n"
	}

	return fmt.Sprintf("**A Records (IPv4):**\n%s\n", strings.Join(ipv4s, "\n"))
}

// lookupAAAA returns AAAA (IPv6) records for a domain
func lookupAAAA(ctx context.Context, l *dnsLookup) string {
	ips, err := l.addresses(ctx)
	if err != nil {
		log.Debug().Ctx(ctx).Err(err).Str("domain", l.domain).Msg("Failed to look up AAAA records")
		return fmt.Sprintf("**AAAA Records (IPv6):**\nNo AAAA records found or lookup failed: %s\n", err.Error())
	}

	var ipv6s []string
	for _, ip := range ips {
		if ip.IP.To4() == nil && ip.IP.To16() != nil {
			ipv6s = append(ipv6s, ip.IP.String())
		}
	}

	if len(ipv6s) == 0 {
		return "**AAAA Records (IPv6):**\nNo AAAA records found\n"
	}

	return fmt.Sprintf("**AAAA Records (IPv6):**\n%s\n", strings.Join(ipv6s, "\n"))
}

// lookupMX returns MX (mail exchange) records for a domain
func lookupMX(ctx context.Context, l *dnsLookup) string {
	mxRecords, err := l.resolver.LookupMX(ctx, l.domain)
	if err != nil {
		log.Debug().Ctx(ctx).Err(err).Str("domain", l.domain).Msg("Failed to look up MX records")
		return fmt.Sprintf("**MX Records (Mail Servers):**\nNo MX records found or lookup failed: %s\n", err.Error())
	}

	if len(mxRecords) == 0 {
		return "**MX Records (Mail Servers):**\nNo MX records found\n"
	}

	var records []string
	for _, mx := range mxRecords {
		records = append(records, fmt.Sprintf("%s (priority: %d)", mx.Host, mx.Pref))
	}

	return fmt.Sprintf("**MX Records (Mail Servers):**\n%s\n", strings.Join(records, "\n"))
}

// lookupTXT returns TXT records for a domain
func lookupTXT(ctx context.Context, l *dnsLookup) string {
	txtRecords, err := l.resolver.LookupTXT(ctx, l.domain)
	if err != nil {
		log.Debug().Ctx(ctx).Err(err).Str("domain", l.domain).Msg("Failed to look up TXT records")
		return fmt.Sprintf("**TXT Records:**\nNo TXT records found or lookup failed: %s\n", err.Error())
	}

	if len(txtRecords) == 0 {
		return "**TXT Records:**\nNo TXT records found\n"
	}

	var records []string
	for _, txt := range txtRecords {
		records = append(records, fmt.Sprintf("`%s`", txt))
	}

	return fmt.Sprintf("**TXT Records:**\n%s\n", strings.Join(records, "\n"))
}

// lookupCNAME returns the CNAME record for a domain
func lookupCNAME(ctx context.Context, l *dnsLookup) string {
	cname, err := l.resolver.LookupCNAME(ctx, l.domain)
	if err != nil {
		log.Debug().Ctx(ctx).Err(err).Str("domain", l.domain).Msg("Failed to look up CNAME record")
		return fmt.Sprintf("**CNAME Record:**\nNo CNAME record found or lookup failed: %s\n", err.Error())
	}

	// If the CNAME is the same as the domain (with trailing dot), there's no CNAME
	if cname == l.domain+"." || cname == l.domain {
		return "**CNAME Record:**\nNo CNAME record (domain resolves directly)\n"
	}

	return fmt.Sprintf("**CNAME Record:**\n%s\n", cname)
}

// lookupNS returns NS (nameserver) records for a domain
func lookupNS(ctx context.Context, l *dnsLookup) string {
	nsRecords, err := l.resolver.LookupNS(ctx, l.domain)
	if err != nil {
		log.Debug().Ctx(ctx).Err(err).Str("domain", l.domain).Msg("Failed to look up NS records")
		return fmt.Sprintf("**NS Records (Nameservers):**\nNo NS records found or lookup failed: %s\n", err.Error())
	}

	if len(nsRecords) == 0 {
		return "**NS Records (Nameservers):**\nNo NS records found\n"
	}

	var records []string
	for _, ns := range nsRecords {
		records = append(records, ns.Host)
	}

	return fmt.Sprintf("**NS Records (Nameservers):**\n%s\n", strings.Join(records, "\n"))
}
