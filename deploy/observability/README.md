# Local OpenTelemetry / LGTM

Compose starts `grafana/otel-lgtm` and points every API service and worker at
`otel-lgtm:4317`. The image provides the development collector plus Grafana,
Loki, Tempo and Prometheus-compatible metrics storage.

- Business dashboard: `http://localhost:3000`
- System observability Grafana: `http://localhost:3001`
- OTLP gRPC: `localhost:4317`
- OTLP HTTP: `localhost:4318`

The application keeps telemetry as a no-op when
`OTEL_EXPORTER_OTLP_ENDPOINT` is unset, so individual services remain easy to
run without the Compose observability stack.
