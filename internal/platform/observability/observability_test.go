package observability

import (
	"context"
	"testing"

	"go.opentelemetry.io/otel/trace"
)

func TestSetupWithoutEndpointIsNoop(t *testing.T) {
	t.Setenv("OTEL_EXPORTER_OTLP_ENDPOINT", "")
	shutdown, err := Setup(context.Background(), "test-service", "test")
	if err != nil {
		t.Fatal(err)
	}
	if err := shutdown(context.Background()); err != nil {
		t.Fatal(err)
	}
}

func TestSetupAcceptsURLEndpoint(t *testing.T) {
	if err := validateEndpoint("http://collector:4317"); err != nil {
		t.Fatal(err)
	}
	if err := validateEndpoint("grpc://collector:4317"); err != ErrInvalidEndpoint {
		t.Fatalf("got %v, want %v", err, ErrInvalidEndpoint)
	}
}

func TestKafkaTraceContextRoundTrip(t *testing.T) {
	traceID, err := trace.TraceIDFromHex("0102030405060708090a0b0c0d0e0f10")
	if err != nil {
		t.Fatal(err)
	}
	spanID, err := trace.SpanIDFromHex("0102030405060708")
	if err != nil {
		t.Fatal(err)
	}
	ctx := trace.ContextWithSpanContext(context.Background(), trace.NewSpanContext(trace.SpanContextConfig{TraceID: traceID, SpanID: spanID, TraceFlags: trace.FlagsSampled}))
	ctx = ExtractKafka(context.Background(), InjectKafka(ctx))
	if got := trace.SpanContextFromContext(ctx).TraceID(); got != traceID {
		t.Fatalf("got trace id %s, want %s", got, traceID)
	}
}
