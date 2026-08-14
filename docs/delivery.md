# SCRUM-13 delivery runbook

This is the reproducible local handoff for the demo. It deliberately uses only the Compose stack and seeded data: no real MRT/POS, push provider, or external LLM is required.

## One-command demo

Prerequisites: Docker Compose, `curl`, and `jq`.

```bash
cp .env.example .env
docker compose --env-file .env -f deploy/compose/compose.yaml up --build -d
./scripts/demo.sh
```

The script exercises entry event → asynchronous recommendation → vector candidate evidence → PostgreSQL rule decision → template/controlled-LLM copy boundary → redemption → merchant verification. It prints the journey, recommendation, and redemption IDs plus Grafana URLs. The recommendation response includes `candidates[]` so the demo visibly separates Qdrant recall from the final eligible offer.

Useful checks:

```bash
curl -fsS http://localhost:8000/healthz
curl -fsS http://localhost:8000/readyz
go test ./...
python3 -m py_compile scripts/load-test.py
python3 scripts/load-test.py --help
```

## Evidence and operating boundary

- `docs/performance-scrum36.json` records the warm-stack load run: 10/10 successful E2E requests at concurrency 2, 12.3 req/s, E2E P50 116.35 ms/P95 334.8 ms, decision P50 2 ms/P95 58 ms, and Kafka lag 0.
- The load test reports Qdrant latency and template fallback rate. A fallback rate of 1.0 is expected when no `OPENAI_API_KEY` is configured; copy generation remains within the configured timeout and uses the deterministic template.
- Grafana is exposed at `http://localhost:3000`; the OpenTelemetry/Grafana LGTM surface is exposed at `http://localhost:3001`.
- Analytics consumes Kafka events into ClickHouse. The `cmd/analytics-demo` helper emits engagement events for local dashboard verification; production MRT/POS/FCM/APNs integrations remain out of scope for this demo.

## Failure and retry behavior

The gateway and workers use an outbox, Kafka consumers commit offsets after successful handling, and notification/redemption operations are idempotent. Recommendation copy generation validates the structured output and falls back to the template on timeout, provider error, invalid schema, or fact mismatch. The relevant unit/integration tests run with `go test ./...`.
