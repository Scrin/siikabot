package llmtools

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"strings"
	"time"

	"github.com/Scrin/siikabot/openrouter"
	"github.com/rs/zerolog/log"
)

// DNSToolDefinition returns the tool definition for the DNS lookup tool
var DNSToolDefinition = openrouter.ToolDefinition{
	Type: "function",
	Function: openrouter.FunctionSchema{
		Name:        "dns_lookup",
		Description: "Look up DNS records for a domain name. Returns A (IPv4), AAAA (IPv6), MX (mail servers), TXT, CNAME, and NS (nameserver) records.",
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

	log.Debug().Ctx(ctx).Str("domain", domain).Str("record_type", recordType).Msg("Performing DNS lookup")

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("**DNS lookup for %s**\n\n", domain))

	switch recordType {
	case "A":
		sb.WriteString(lookupA(ctx, domain))
	case "AAAA":
		sb.WriteString(lookupAAAA(ctx, domain))
	case "MX":
		sb.WriteString(lookupMX(ctx, domain))
	case "TXT":
		sb.WriteString(lookupTXT(ctx, domain))
	case "CNAME":
		sb.WriteString(lookupCNAME(ctx, domain))
	case "NS":
		sb.WriteString(lookupNS(ctx, domain))
	case "ALL":
		sb.WriteString(lookupA(ctx, domain))
		sb.WriteString("\n")
		sb.WriteString(lookupAAAA(ctx, domain))
		sb.WriteString("\n")
		sb.WriteString(lookupMX(ctx, domain))
		sb.WriteString("\n")
		sb.WriteString(lookupTXT(ctx, domain))
		sb.WriteString("\n")
		sb.WriteString(lookupCNAME(ctx, domain))
		sb.WriteString("\n")
		sb.WriteString(lookupNS(ctx, domain))
	default:
		return "", fmt.Errorf("invalid record type: %s (valid types: A, AAAA, MX, TXT, CNAME, NS, ALL)", recordType)
	}

	result := sb.String()
	log.Debug().Ctx(ctx).Str("domain", domain).Str("record_type", recordType).Int("response_length", len(result)).Msg("DNS lookup completed")

	return result, nil
}

// lookupA returns A (IPv4) records for a domain
func lookupA(ctx context.Context, domain string) string {
	ips, err := net.LookupIP(domain)
	if err != nil {
		log.Debug().Ctx(ctx).Err(err).Str("domain", domain).Msg("Failed to look up A records")
		return fmt.Sprintf("**A Records (IPv4):**\nNo A records found or lookup failed: %s\n", err.Error())
	}

	var ipv4s []string
	for _, ip := range ips {
		if ipv4 := ip.To4(); ipv4 != nil {
			ipv4s = append(ipv4s, ipv4.String())
		}
	}

	if len(ipv4s) == 0 {
		return "**A Records (IPv4):**\nNo A records found\n"
	}

	return fmt.Sprintf("**A Records (IPv4):**\n%s\n", strings.Join(ipv4s, "\n"))
}

// lookupAAAA returns AAAA (IPv6) records for a domain
func lookupAAAA(ctx context.Context, domain string) string {
	ips, err := net.LookupIP(domain)
	if err != nil {
		log.Debug().Ctx(ctx).Err(err).Str("domain", domain).Msg("Failed to look up AAAA records")
		return fmt.Sprintf("**AAAA Records (IPv6):**\nNo AAAA records found or lookup failed: %s\n", err.Error())
	}

	var ipv6s []string
	for _, ip := range ips {
		if ip.To4() == nil && ip.To16() != nil {
			ipv6s = append(ipv6s, ip.String())
		}
	}

	if len(ipv6s) == 0 {
		return "**AAAA Records (IPv6):**\nNo AAAA records found\n"
	}

	return fmt.Sprintf("**AAAA Records (IPv6):**\n%s\n", strings.Join(ipv6s, "\n"))
}

// lookupMX returns MX (mail exchange) records for a domain
func lookupMX(ctx context.Context, domain string) string {
	mxRecords, err := net.LookupMX(domain)
	if err != nil {
		log.Debug().Ctx(ctx).Err(err).Str("domain", domain).Msg("Failed to look up MX records")
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
func lookupTXT(ctx context.Context, domain string) string {
	txtRecords, err := net.LookupTXT(domain)
	if err != nil {
		log.Debug().Ctx(ctx).Err(err).Str("domain", domain).Msg("Failed to look up TXT records")
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
func lookupCNAME(ctx context.Context, domain string) string {
	cname, err := net.LookupCNAME(domain)
	if err != nil {
		log.Debug().Ctx(ctx).Err(err).Str("domain", domain).Msg("Failed to look up CNAME record")
		return fmt.Sprintf("**CNAME Record:**\nNo CNAME record found or lookup failed: %s\n", err.Error())
	}

	// If the CNAME is the same as the domain (with trailing dot), there's no CNAME
	if cname == domain+"." || cname == domain {
		return "**CNAME Record:**\nNo CNAME record (domain resolves directly)\n"
	}

	return fmt.Sprintf("**CNAME Record:**\n%s\n", cname)
}

// lookupNS returns NS (nameserver) records for a domain
func lookupNS(ctx context.Context, domain string) string {
	nsRecords, err := net.LookupNS(domain)
	if err != nil {
		log.Debug().Ctx(ctx).Err(err).Str("domain", domain).Msg("Failed to look up NS records")
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
