package llmtools

import (
	"bytes"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"sync"
	"testing"
)

// The tools' HTTP, sent to handlers of the test's own instead of the internet

// fakeInternet records what the tools asked for
type fakeInternet struct {
	mu       sync.Mutex
	requests []*http.Request
}

// Requests returns the requests made for a host
func (f *fakeInternet) Requests(host string) []*http.Request {
	f.mu.Lock()
	defer f.mu.Unlock()
	var requests []*http.Request
	for _, r := range f.requests {
		if r.Host == host {
			requests = append(requests, r)
		}
	}
	return requests
}

// useFakeInternet sends every request the tools make through the shared client to the handler for its host, until
// the test ends. A request for any other host gets a 502. The client is shared, so tests that use this can't run
// in parallel.
func useFakeInternet(t *testing.T, handlers map[string]http.HandlerFunc) *fakeInternet {
	t.Helper()
	recorded := &fakeInternet{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		r.Body = io.NopCloser(bytes.NewReader(body))
		recorded.mu.Lock()
		recorded.requests = append(recorded.requests, r.Clone(r.Context()))
		recorded.mu.Unlock()
		handler, ok := handlers[r.Host]
		if !ok {
			http.Error(w, "nothing handles "+r.Host, http.StatusBadGateway)
			return
		}
		handler(w, r)
	}))
	target, _ := url.Parse(server.URL)

	previous := httpClient.Transport
	httpClient.Transport = roundTripFunc(func(req *http.Request) (*http.Response, error) {
		req = req.Clone(req.Context())
		req.Host = req.URL.Host
		req.URL.Scheme, req.URL.Host = target.Scheme, target.Host
		return http.DefaultTransport.RoundTrip(req)
	})
	t.Cleanup(func() {
		httpClient.Transport = previous
		server.Close()
	})

	// The tools that cache keep nothing from another test
	exchangeRatesCacheMutex.Lock()
	exchangeRatesCache = nil
	exchangeRatesCacheMutex.Unlock()
	electricityPriceCacheMutex.Lock()
	electricityPriceCache = map[string][]PriceEntry{}
	electricityPriceCacheMutex.Unlock()
	return recorded
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) { return f(req) }

// serve answers with a fixed body
func serve(contentType, body string) http.HandlerFunc {
	return func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", contentType)
		_, _ = io.WriteString(w, body)
	}
}

// status answers with an HTTP error
func status(code int) http.HandlerFunc {
	return func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(code)
	}
}
