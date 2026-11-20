// Package fakenet is the end-to-end tests' stand-in for the network. A bot built with the e2e tag sends every
// request it makes to the internet here (see testhooks), with the host it was for in the Host header. Requests
// are routed by that host: the AI gateway's hosts go to the scripted model (Model), and others to whatever a
// test registered. A request for a host nobody registered gets a 502 and is recorded, so a test can never reach
// the real internet, and can check what the bot asked for.
package fakenet

import (
	"bytes"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
)

// Request is a request the bot made
type Request struct {
	Method string
	Host   string
	// URL is the request's path and query
	URL    string
	Header http.Header
	Body   []byte
}

// Net is the stand-in for the network
type Net struct {
	server *httptest.Server
	// Model stands in for the AI gateway
	Model *Model

	mu       sync.Mutex
	handlers map[string]http.Handler
	requests []Request
}

// Start starts the stand-in, with the model on the gateway's hosts
func Start() *Net {
	n := &Net{handlers: map[string]http.Handler{}, Model: newModel()}
	n.handlers[GatewayHost] = n.Model
	n.handlers[CloudflareAPIHost] = n.Model
	n.server = httptest.NewServer(n)
	return n
}

// URL is where the bot sends its requests to
func (n *Net) URL() string {
	return n.server.URL
}

// Close stops the stand-in
func (n *Net) Close() {
	n.server.Close()
}

// Handle serves a host's requests with h until t ends
func (n *Net) Handle(t testing.TB, host string, h http.Handler) {
	t.Helper()
	n.mu.Lock()
	defer n.mu.Unlock()
	if _, taken := n.handlers[host]; taken {
		t.Fatalf("fakenet: %s is already handled", host)
	}
	n.handlers[host] = h
	t.Cleanup(func() {
		n.mu.Lock()
		defer n.mu.Unlock()
		delete(n.handlers, host)
	})
}

// Requests returns the requests the bot made for a host, the model's included
func (n *Net) Requests(host string) []Request {
	n.mu.Lock()
	defer n.mu.Unlock()
	var requests []Request
	for _, r := range n.requests {
		if r.Host == host {
			requests = append(requests, r)
		}
	}
	return requests
}

func (n *Net) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	body, _ := io.ReadAll(r.Body)
	r.Body = io.NopCloser(bytes.NewReader(body))
	host := r.Host
	if h, _, err := net.SplitHostPort(host); err == nil {
		host = h
	}

	n.mu.Lock()
	n.requests = append(n.requests, Request{Method: r.Method, Host: host, URL: r.URL.RequestURI(), Header: r.Header.Clone(), Body: body})
	handler := n.handlers[host]
	n.mu.Unlock()

	if handler == nil {
		http.Error(w, "fakenet: nothing handles "+host, http.StatusBadGateway)
		return
	}
	handler.ServeHTTP(w, r)
}
