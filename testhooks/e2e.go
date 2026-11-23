//go:build e2e

package testhooks

import (
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"os"
	"strconv"
	"time"
)

// The end-to-end tests' stand-in for the network: every request the bot makes to the internet goes to it, with
// the host it was for kept in the Host header and the scheme in X-E2E-Scheme
var netURL = func() *url.URL {
	raw := os.Getenv("SIIKABOT_E2E_NET")
	if raw == "" {
		panic("testhooks: an e2e build needs SIIKABOT_E2E_NET, the address of the tests' stand-in for the network")
	}
	u, err := url.Parse(raw)
	if err != nil || u.Host == "" {
		panic("testhooks: SIIKABOT_E2E_NET isn't a URL: " + raw)
	}
	return u
}()

// Enabled reports whether this is an end-to-end test build
const Enabled = true

// Transport is the transport a client that reaches the internet uses: rt, with every request sent to the stand-in
func Transport(rt http.RoundTripper) http.RoundTripper {
	return redirect{next: rt}
}

type redirect struct {
	next http.RoundTripper
}

func (r redirect) RoundTrip(req *http.Request) (*http.Response, error) {
	req = req.Clone(req.Context())
	req.Host = req.URL.Host
	req.Header.Set("X-E2E-Scheme", req.URL.Scheme)
	req.URL.Scheme, req.URL.Host = netURL.Scheme, netURL.Host
	return r.next.RoundTrip(req)
}

// AllowAddress reports whether the fetch guard lets a connection reach an address it would otherwise refuse: the
// stand-in's, which is on loopback
func AllowAddress(address netip.AddrPort) bool {
	host, port, err := net.SplitHostPort(netURL.Host)
	if err != nil {
		return false
	}
	addr, err := netip.ParseAddr(host)
	return err == nil && address.Addr() == addr && strconv.Itoa(int(address.Port())) == port
}

// NoTracing reports whether tracing is off
func NoTracing() bool { return true }

// ReflectionInterval is how often the bot looks for rooms due a pass over their conversations: as often as
// SIIKABOT_E2E_REFLECTION_INTERVAL says when a test sets it, so the test needn't wait minutes, and every d otherwise
func ReflectionInterval(d time.Duration) time.Duration {
	if interval, err := time.ParseDuration(os.Getenv("SIIKABOT_E2E_REFLECTION_INTERVAL")); err == nil && interval > 0 {
		return interval
	}
	return d
}
