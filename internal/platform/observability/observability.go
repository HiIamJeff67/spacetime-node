package observability

import (
	"context"
	"errors"
	"fmt"
	"log"
	"log/slog"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/go-kratos/kratos/v3/middleware"
	"github.com/go-kratos/kratos/v3/transport"
	"github.com/segmentio/kafka-go"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/exporters/otlp/otlpmetric/otlpmetricgrpc"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc"
	otelmetric "go.opentelemetry.io/otel/metric"
	"go.opentelemetry.io/otel/propagation"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/semconv/v1.37.0"
	oteltrace "go.opentelemetry.io/otel/trace"
)

var ErrInvalidEndpoint = errors.New("invalid OTLP endpoint")

func validateEndpoint(endpoint string) error {
	if strings.Contains(endpoint, "://") {
		parsed, err := url.Parse(endpoint)
		if err != nil || parsed.Host == "" || (parsed.Scheme != "http" && parsed.Scheme != "https") {
			return ErrInvalidEndpoint
		}
	}
	return nil
}

func Setup(ctx context.Context, serviceName, version string) (func(context.Context) error, error) {
	otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(propagation.TraceContext{}, propagation.Baggage{}))
	endpoint := strings.TrimSpace(os.Getenv("OTEL_EXPORTER_OTLP_ENDPOINT"))
	if endpoint == "" {
		return func(context.Context) error { return nil }, nil
	}
	traceOptions := []otlptracegrpc.Option{}
	metricOptions := []otlpmetricgrpc.Option{}
	if err := validateEndpoint(endpoint); err != nil {
		return nil, err
	}
	if strings.Contains(endpoint, "://") {
		traceOptions = append(traceOptions, otlptracegrpc.WithEndpointURL(endpoint))
		metricOptions = append(metricOptions, otlpmetricgrpc.WithEndpointURL(endpoint))
	} else {
		traceOptions = append(traceOptions, otlptracegrpc.WithEndpoint(endpoint), otlptracegrpc.WithInsecure())
		metricOptions = append(metricOptions, otlpmetricgrpc.WithEndpoint(endpoint), otlpmetricgrpc.WithInsecure())
	}
	res, err := resource.New(ctx,
		resource.WithAttributes(semconv.ServiceName(serviceName), semconv.ServiceVersion(version)),
	)
	if err != nil {
		return nil, err
	}
	traceExporter, err := otlptracegrpc.New(ctx, traceOptions...)
	if err != nil {
		return nil, err
	}
	metricExporter, err := otlpmetricgrpc.New(ctx, metricOptions...)
	if err != nil {
		return nil, err
	}
	tracerProvider := sdktrace.NewTracerProvider(sdktrace.WithBatcher(traceExporter), sdktrace.WithResource(res))
	metricProvider := sdkmetric.NewMeterProvider(
		sdkmetric.WithReader(sdkmetric.NewPeriodicReader(metricExporter, sdkmetric.WithInterval(10*time.Second))),
		sdkmetric.WithResource(res),
	)
	otel.SetTracerProvider(tracerProvider)
	otel.SetMeterProvider(metricProvider)
	return func(shutdownCtx context.Context) error {
		traceErr := tracerProvider.Shutdown(shutdownCtx)
		metricErr := metricProvider.Shutdown(shutdownCtx)
		return errors.Join(traceErr, metricErr)
	}, nil
}

func Middleware(serviceName string) middleware.Middleware {
	tracer := otel.Tracer("spacetime-node/" + serviceName)
	meter := otel.Meter("spacetime-node/" + serviceName)
	requestCounter, _ := meter.Int64Counter("http_server_requests_total")
	errorCounter, _ := meter.Int64Counter("http_server_errors_total")
	requestDuration, _ := meter.Float64Histogram("http_server_request_duration_ms")
	return func(next middleware.Handler) middleware.Handler {
		return func(ctx context.Context, req interface{}) (interface{}, error) {
			serverTransport, _ := transport.FromServerContext(ctx)
			if serverTransport != nil {
				ctx = otel.GetTextMapPropagator().Extract(ctx, serverTransport.RequestHeader())
			}
			operation := serviceName
			if serverTransport != nil && serverTransport.Operation() != "" {
				operation = serverTransport.Operation()
			}
			ctx, span := tracer.Start(ctx, operation, oteltrace.WithSpanKind(oteltrace.SpanKindServer))
			if serverTransport != nil {
				otel.GetTextMapPropagator().Inject(ctx, serverTransport.ReplyHeader())
			}
			started := time.Now()
			response, err := next(ctx, req)
			attrs := otelmetric.WithAttributes(attribute.String("service.name", serviceName), attribute.String("rpc.method", operation))
			requestCounter.Add(ctx, 1, attrs)
			requestDuration.Record(ctx, float64(time.Since(started).Microseconds())/1000, attrs)
			if err != nil {
				errorCounter.Add(ctx, 1, attrs)
				span.RecordError(err)
			}
			span.End()
			return response, err
		}
	}
}

func NewLogger(serviceName string) *log.Logger {
	var handler slog.Handler = slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo})
	handler = handler.WithAttrs([]slog.Attr{slog.String("service.name", serviceName)})
	return slog.NewLogLogger(handler, slog.LevelInfo)
}

// Logf keeps the stdlib logger call sites while adding safe trace and business
// correlation fields to the JSON log message. Callers should pass hashes and
// stable IDs only; raw user content must never be logged.
func Logf(ctx context.Context, logger *log.Logger, message string, attrs ...attribute.KeyValue) {
	if logger == nil {
		return
	}
	fields := make([]string, 0, len(attrs)+2)
	if spanContext := oteltrace.SpanContextFromContext(ctx); spanContext.IsValid() {
		fields = append(fields, "trace_id="+spanContext.TraceID().String(), "span_id="+spanContext.SpanID().String())
	}
	for _, attr := range attrs {
		fields = append(fields, fmt.Sprintf("%s=%s", attr.Key, attr.Value.AsString()))
	}
	if len(fields) > 0 {
		message += " " + strings.Join(fields, " ")
	}
	logger.Print(message)
}

type kafkaCarrier map[string]string

func (c kafkaCarrier) Get(key string) string { return c[key] }

func (c kafkaCarrier) Set(key, value string) { c[key] = value }

func (c kafkaCarrier) Keys() []string {
	keys := make([]string, 0, len(c))
	for key := range c {
		keys = append(keys, key)
	}
	return keys
}

func InjectKafka(ctx context.Context) []kafka.Header {
	carrier := kafkaCarrier{}
	otel.GetTextMapPropagator().Inject(ctx, carrier)
	headers := make([]kafka.Header, 0, len(carrier))
	for key, value := range carrier {
		headers = append(headers, kafka.Header{Key: key, Value: []byte(value)})
	}
	return headers
}

func ExtractKafka(ctx context.Context, headers []kafka.Header) context.Context {
	carrier := kafkaCarrier{}
	for _, header := range headers {
		carrier[header.Key] = string(header.Value)
	}
	return otel.GetTextMapPropagator().Extract(ctx, carrier)
}

func StartKafkaSpan(ctx context.Context, topic, eventID string, consumer bool) (context.Context, oteltrace.Span) {
	kind := oteltrace.SpanKindProducer
	if consumer {
		kind = oteltrace.SpanKindConsumer
	}
	return otel.Tracer("spacetime-node/kafka").Start(ctx, "kafka."+topic, oteltrace.WithSpanKind(kind), oteltrace.WithAttributes(attribute.String("messaging.destination.name", topic), attribute.String("messaging.message.id", eventID)))
}

func AddCounter(ctx context.Context, name string, value int64, attributes ...attribute.KeyValue) {
	counter, _ := otel.Meter("spacetime-node").Int64Counter(name)
	counter.Add(ctx, value, otelmetric.WithAttributes(attributes...))
}

func RecordValue(ctx context.Context, name string, value int64, attributes ...attribute.KeyValue) {
	gauge, _ := otel.Meter("spacetime-node").Int64Gauge(name)
	gauge.Record(ctx, value, otelmetric.WithAttributes(attributes...))
}

func RecordDuration(ctx context.Context, name string, started time.Time, attributes ...attribute.KeyValue) {
	histogram, _ := otel.Meter("spacetime-node").Float64Histogram(name)
	histogram.Record(ctx, float64(time.Since(started).Microseconds())/1000, otelmetric.WithAttributes(attributes...))
}
