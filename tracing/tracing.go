// Package tracing wires up OpenTelemetry tracing and exports spans to Tempo.
//
// Tracing is always on: the endpoint and credentials are required configuration, so there is no
// unconfigured state to fall back from. A Tempo outage is a different matter and is deliberately
// harmless — the batch processor drops spans and the bot carries on.
package tracing

import (
	"context"
	"encoding/base64"
	"fmt"
	"runtime/debug"
	"strings"
	"time"

	"github.com/Scrin/siikabot/config"
	"github.com/rs/zerolog/log"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.26.0"
	"go.opentelemetry.io/otel/trace"
	"google.golang.org/grpc/credentials"
)

// serviceName identifies the bot in Tempo. Not deployment-specific, so it needs no configuration.
const serviceName = "siikabot"

// exportTimeout bounds how long a shutdown flush may take before the process gives up on it
const exportTimeout = 10 * time.Second

// Init configures the global tracer provider and returns a shutdown function that flushes any
// pending spans. Call the shutdown before exiting, or the last spans of a run are lost.
func Init(ctx context.Context) (func(context.Context) error, error) {
	opts := []otlptracegrpc.Option{
		// WithEndpointURL rather than WithEndpoint: the endpoint is configured as a URL, matching
		// SIIKABOT_HOMESERVER_URL and the rest of the project, and this strips the scheme itself.
		// Passing a URL to WithEndpoint instead makes gRPC treat "https" as the hostname and fail
		// with a DNS lookup error that gives no hint as to the real cause.
		otlptracegrpc.WithEndpointURL(config.TempoEndpoint),
		// The credential is applied here rather than through OTEL_EXPORTER_OTLP_HEADERS, whose
		// values are specified as percent-encoded W3C Baggage but are decoded inconsistently across
		// SDKs — and a base64 credential contains exactly the characters that expose that
		otlptracegrpc.WithHeaders(map[string]string{
			"Authorization": basicAuth(config.TempoUser, config.TempoPassword),
		}),
	}

	// WithEndpointURL infers transport security from the scheme, but leaves the TLS config empty.
	// Setting it explicitly pins verification to the system roots, which is what Tempo behind nginx
	// with a real certificate needs.
	if strings.HasPrefix(config.TempoEndpoint, "https://") {
		opts = append(opts, otlptracegrpc.WithTLSCredentials(credentials.NewClientTLSFromCert(nil, "")))
	}

	exporter, err := otlptracegrpc.New(ctx, opts...)
	if err != nil {
		log.Error().Ctx(ctx).Err(err).
			Str("endpoint", config.TempoEndpoint).
			Msg("Failed to create the trace exporter")
		return nil, fmt.Errorf("failed to create trace exporter: %w", err)
	}

	provider := sdktrace.NewTracerProvider(
		sdktrace.WithBatcher(exporter),
		sdktrace.WithResource(newResource()),
		// Everything is sampled. At this bot's volume the cost is immaterial, and partial sampling
		// would mean occasionally holding a trace id that no recorded trace corresponds to.
		sdktrace.WithSampler(sdktrace.AlwaysSample()),
	)

	otel.SetTracerProvider(provider)

	// W3C trace context, so an incoming traceparent from nginx is understood and continued
	otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(
		propagation.TraceContext{},
		propagation.Baggage{},
	))

	// Exporter errors are logged rather than surfaced, so a Tempo problem never reaches a user
	otel.SetErrorHandler(otel.ErrorHandlerFunc(func(err error) {
		log.Warn().Err(err).Msg("OpenTelemetry error")
	}))

	log.Info().Ctx(ctx).
		Str("endpoint", config.TempoEndpoint).
		Str("service_name", serviceName).
		Msg("Tracing initialized")

	return func(shutdownCtx context.Context) error {
		shutdownCtx, cancel := context.WithTimeout(shutdownCtx, exportTimeout)
		defer cancel()
		return provider.Shutdown(shutdownCtx)
	}, nil
}

// newResource describes this process to the tracing backend
func newResource() *resource.Resource {
	attrs := []attribute.KeyValue{
		semconv.ServiceName(serviceName),
	}

	// The version comes from the build itself, so it is right without anything having to set it
	if info, ok := debug.ReadBuildInfo(); ok {
		for _, setting := range info.Settings {
			if setting.Key == "vcs.revision" && setting.Value != "" {
				attrs = append(attrs, semconv.ServiceVersion(setting.Value))
				break
			}
		}
	}

	return resource.NewWithAttributes(semconv.SchemaURL, attrs...)
}

// basicAuth builds an HTTP basic authorization header value
func basicAuth(user, password string) string {
	return "Basic " + base64.StdEncoding.EncodeToString([]byte(user+":"+password))
}

// Tracer returns a tracer for the given instrumentation scope, conventionally the package path
func Tracer(scope string) trace.Tracer {
	return otel.Tracer(scope)
}

// TraceID returns the trace id of the span in this context, or an empty string when there is none.
//
// Used where a trace id has to travel outside the tracing system entirely: into a Matrix reply's
// debug data, or into Cloudflare's request metadata.
func TraceID(ctx context.Context) string {
	if sc := trace.SpanContextFromContext(ctx); sc.IsValid() {
		return sc.TraceID().String()
	}
	return ""
}

// SpanID returns the span id of the span in this context, or an empty string when there is none
func SpanID(ctx context.Context) string {
	if sc := trace.SpanContextFromContext(ctx); sc.IsValid() {
		return sc.SpanID().String()
	}
	return ""
}
