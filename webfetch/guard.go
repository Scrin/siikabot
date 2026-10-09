package webfetch

import (
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/netip"
	"syscall"
	"time"

	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
)

// errAddressNotAllowed is returned when a connection would go to an address the bot mustn't reach
var errAddressNotAllowed = errors.New("address not allowed")

// allowedAddress reports whether the bot may connect to an address. A variable so tests can let the bot reach
// their local servers.
var allowedAddress = func(address netip.AddrPort) bool {
	return isPublic(address.Addr())
}

// nonPublicPrefixes are the ranges isPublic refuses on top of what netip already classifies as loopback,
// private, link-local, multicast or unspecified
var nonPublicPrefixes = []netip.Prefix{
	netip.MustParsePrefix("0.0.0.0/8"),       // "this network"
	netip.MustParsePrefix("100.64.0.0/10"),   // shared address space, used for carrier-grade NAT
	netip.MustParsePrefix("192.0.0.0/24"),    // IETF protocol assignments
	netip.MustParsePrefix("192.0.2.0/24"),    // documentation
	netip.MustParsePrefix("198.18.0.0/15"),   // benchmarking
	netip.MustParsePrefix("198.51.100.0/24"), // documentation
	netip.MustParsePrefix("203.0.113.0/24"),  // documentation
	netip.MustParsePrefix("240.0.0.0/4"),     // reserved, including the broadcast address
	netip.MustParsePrefix("2001:db8::/32"),   // documentation
	netip.MustParsePrefix("fec0::/10"),       // deprecated site-local
}

// nat64 is the well-known NAT64 prefix. An address in it carries an IPv4 address, which has to be public too.
var nat64 = netip.MustParsePrefix("64:ff9b::/96")

// isPublic reports whether an address is an ordinary public unicast address
func isPublic(addr netip.Addr) bool {
	addr = addr.Unmap()
	if !addr.IsValid() || addr.IsLoopback() || addr.IsPrivate() || addr.IsLinkLocalUnicast() ||
		addr.IsLinkLocalMulticast() || addr.IsInterfaceLocalMulticast() || addr.IsMulticast() || addr.IsUnspecified() {
		return false
	}
	for _, prefix := range nonPublicPrefixes {
		if prefix.Contains(addr) {
			return false
		}
	}
	if nat64.Contains(addr) {
		embedded := addr.As16()
		return isPublic(netip.AddrFrom4([4]byte(embedded[12:])))
	}
	return true
}

// guardConnection checks the address a connection is about to be made to. Checking here, rather than checking
// the URL's host before the request, covers redirects, and DNS answers that change between a check and the
// connection.
func guardConnection(_, address string, _ syscall.RawConn) error {
	addrPort, err := netip.ParseAddrPort(address)
	if err != nil {
		return err
	}
	if !allowedAddress(addrPort) {
		return fmt.Errorf("%w: %s", errAddressNotAllowed, addrPort.Addr())
	}
	return nil
}

// newClient makes the HTTP client fetches use. It refuses non-public addresses, and it ignores proxy settings,
// because a proxy would hide the real destination from the address check.
func newClient() *http.Client {
	dialer := &net.Dialer{Timeout: requestTimeout, Control: guardConnection}
	transport := &http.Transport{
		Proxy:                 nil,
		DialContext:           dialer.DialContext,
		ForceAttemptHTTP2:     true,
		TLSHandshakeTimeout:   requestTimeout,
		MaxIdleConns:          10,
		IdleConnTimeout:       90 * time.Second,
		ExpectContinueTimeout: time.Second,
	}
	return &http.Client{
		Timeout:   requestTimeout,
		Transport: otelhttp.NewTransport(transport),
		CheckRedirect: func(_ *http.Request, via []*http.Request) error {
			if len(via) >= maxRedirects {
				return errTooManyRedirects
			}
			return nil
		},
	}
}

var errTooManyRedirects = fmt.Errorf("stopped after %d redirects", maxRedirects)
