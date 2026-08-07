package aigateway

import (
	"context"
	"encoding/json"
	"errors"
	"net"
	"net/http"
	"strings"
)

// ErrorKind is a coarse classification of a failure, used as a metric label.
//
// The set is deliberately small and closed: these values end up on a public metrics endpoint, so
// they must never carry anything derived from a message body or another unbounded source.
type ErrorKind string

const (
	ErrorKindTimeout       ErrorKind = "timeout"
	ErrorKindNetwork       ErrorKind = "network"
	ErrorKindCancelled     ErrorKind = "cancelled"
	ErrorKindRateLimited   ErrorKind = "rate_limited"
	ErrorKindAuth          ErrorKind = "auth"
	ErrorKindModelNotFound ErrorKind = "model_not_found"
	ErrorKindBadRequest    ErrorKind = "bad_request"
	ErrorKindProviderError ErrorKind = "provider_error"
	ErrorKindParseError    ErrorKind = "parse_error"
	ErrorKindUnknown       ErrorKind = "unknown"
)

// classifyTransportError classifies a failure that happened before a response was read
func classifyTransportError(err error) ErrorKind {
	switch {
	case errors.Is(err, context.DeadlineExceeded):
		return ErrorKindTimeout
	case errors.Is(err, context.Canceled):
		return ErrorKindCancelled
	}

	var netErr net.Error
	if errors.As(err, &netErr) && netErr.Timeout() {
		return ErrorKindTimeout
	}
	if errors.As(err, &netErr) {
		return ErrorKindNetwork
	}

	var opErr *net.OpError
	if errors.As(err, &opErr) {
		return ErrorKindNetwork
	}

	return ErrorKindUnknown
}

// classifyResponseError classifies a failure reported by a response, from its HTTP status and the
// error envelope the API returns.
//
// The status is the more reliable signal, so it is consulted first; the message is only inspected
// for cases the status cannot distinguish, such as a rate limit surfaced as a generic failure.
func classifyResponseError(statusCode int, errs []apiError) ErrorKind {
	_, message := firstError(errs)
	lowered := strings.ToLower(message)

	// Checked ahead of the status because the gateway reports its own throttling with a variety of
	// statuses, and being throttled is worth telling apart from an upstream failure
	if strings.Contains(lowered, "rate limit") {
		return ErrorKindRateLimited
	}

	switch statusCode {
	case http.StatusTooManyRequests:
		return ErrorKindRateLimited
	case http.StatusUnauthorized, http.StatusForbidden:
		return ErrorKindAuth
	case http.StatusNotFound:
		return ErrorKindModelNotFound
	case http.StatusRequestTimeout, http.StatusGatewayTimeout:
		return ErrorKindTimeout
	}

	if strings.Contains(lowered, "model not found") {
		return ErrorKindModelNotFound
	}

	switch {
	case statusCode >= 500:
		return ErrorKindProviderError
	case statusCode >= 400:
		return ErrorKindBadRequest
	}

	// A failure reported in the envelope while the status said success
	if message != "" {
		return ErrorKindProviderError
	}
	return ErrorKindUnknown
}

// ClassifyToolError classifies a failure returned by a tool handler.
//
// Tool handlers return opaque errors, but they wrap consistently with %w, so the underlying cause is
// still reachable. Anything unrecognised is reported as a plain execution failure rather than being
// guessed at from its message.
func ClassifyToolError(err error) ErrorKind {
	if err == nil {
		return ErrorKindUnknown
	}

	switch {
	case errors.Is(err, context.DeadlineExceeded):
		return ErrorKindTimeout
	case errors.Is(err, context.Canceled):
		return ErrorKindCancelled
	}

	var syntaxErr *json.SyntaxError
	var typeErr *json.UnmarshalTypeError
	if errors.As(err, &syntaxErr) || errors.As(err, &typeErr) {
		return ErrorKindParseError
	}

	var netErr net.Error
	if errors.As(err, &netErr) {
		if netErr.Timeout() {
			return ErrorKindTimeout
		}
		return ErrorKindNetwork
	}

	return ErrorKindProviderError
}
