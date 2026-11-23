//go:build !e2e

// Package testhooks is where an end-to-end test build (the e2e build tag) reaches into the bot: it sends the
// bot's outbound HTTP to a stand-in for the network, lets the fetch guard reach that stand-in, turns tracing off,
// and lets a test have the bot look for rooms due a pass over their conversations more often. In every other
// build, which includes production's, each hook leaves things as they are, and nothing reads the SIIKABOT_E2E_*
// variables, so no configuration can turn them on.
package testhooks

import (
	"net/http"
	"net/netip"
	"time"
)

// Enabled reports whether this is an end-to-end test build
const Enabled = false

// Transport is the transport a client that reaches the internet uses
func Transport(rt http.RoundTripper) http.RoundTripper { return rt }

// AllowAddress reports whether the fetch guard lets a connection reach an address it would otherwise refuse
func AllowAddress(netip.AddrPort) bool { return false }

// NoTracing reports whether tracing is off
func NoTracing() bool { return false }

// ReflectionInterval is how often the bot looks for rooms due a pass over their conversations
func ReflectionInterval(d time.Duration) time.Duration { return d }
