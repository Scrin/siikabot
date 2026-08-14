package llmtools

import (
	"net/http"
	"time"

	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
)

// toolHTTPTimeout bounds an outbound call made by a tool. Tools run inside a chat turn that has its
// own deadline, so a slow upstream must not be allowed to consume the whole budget.
const toolHTTPTimeout = 10 * time.Second

// httpClient is shared by every tool that makes an outbound request.
//
// Previously each tool constructed its own, which meant instrumenting them would have been a dozen
// identical edits and any new tool would silently start out untraced. One client also makes the
// timeout consistent by construction rather than by everyone remembering the same number.
var httpClient = &http.Client{
	Timeout:   toolHTTPTimeout,
	Transport: otelhttp.NewTransport(http.DefaultTransport),
}
