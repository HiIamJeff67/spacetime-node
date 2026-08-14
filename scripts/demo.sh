#!/usr/bin/env bash

set -euo pipefail

GATEWAY_URL="${GATEWAY_URL:-http://localhost:8000}"
REDEMPTION_URL="${REDEMPTION_URL:-http://localhost:8003}"
USER_ID_HASH="${USER_ID_HASH:-sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa}"
STATION_ID="${STATION_ID:-R04}"
LINE_ID="${LINE_ID:-R}"
POSITION_ID="${POSITION_ID:-exit-3}"
TRACE_ID="${TRACE_ID:-demo-$(date +%s)}"
MAX_ATTEMPTS="${MAX_ATTEMPTS:-30}"
SLEEP_SECONDS="${SLEEP_SECONDS:-1}"

command -v curl >/dev/null || { echo "curl is required" >&2; exit 1; }
command -v jq >/dev/null || { echo "jq is required" >&2; exit 1; }

echo "[1/5] create journey entry at ${STATION_ID}"
entry_response=$(curl -fsS -X POST "${GATEWAY_URL}/v1/entry-events" \
  -H 'Content-Type: application/json' \
  -d "$(jq -nc \
    --arg user_id_hash "$USER_ID_HASH" \
    --arg station_id "$STATION_ID" \
    --arg line_id "$LINE_ID" \
    --arg position_id "$POSITION_ID" \
    --arg trace_id "$TRACE_ID" \
    '{request_context:{trace_id:$trace_id},user_id_hash:$user_id_hash,station_id:$station_id,line_id:$line_id,position_id:$position_id}')")
journey_id=$(jq -er '.journey_id' <<<"$entry_response")
echo "journey_id=${journey_id}"

echo "[2/5] wait for recommendation"
recommendation_response=''
for attempt in $(seq 1 "$MAX_ATTEMPTS"); do
  recommendation_response=$(curl -sS -G "${GATEWAY_URL}/v1/recommendations/latest" \
    --data-urlencode "journey_id=${journey_id}") || true
  if jq -e '.recommendation_id and .offer_id' >/dev/null 2>&1 <<<"$recommendation_response"; then
    break
  fi
  if [ "$attempt" = "$MAX_ATTEMPTS" ]; then
    echo "recommendation did not become ready: ${recommendation_response}" >&2
    exit 1
  fi
  sleep "$SLEEP_SECONDS"
done
recommendation_id=$(jq -er '.recommendation_id' <<<"$recommendation_response")
offer_id=$(jq -er '.offer_id' <<<"$recommendation_response")
jq --arg selected_offer_id "$offer_id" '{recommendation_id,offer_id,title,copy_source,decision_latency_ms,reasons,
    vector_candidates: [.candidates[]? | {offer_id,vector_score,eligible,reasons}],
    final_rule_decision: {offer_id:$selected_offer_id,rule_score:(.candidates[]? | select(.offer_id == $selected_offer_id) | .rule_score),reasons},
    copy_boundary: {source: .copy_source, template_fallback: (.copy_source == "template")}}' <<<"$recommendation_response"

case "$offer_id" in
  offer-coffee-xinyi) merchant_id=merchant-coffee-demo ;;
  offer-lunch-xinyi) merchant_id=merchant-lunch-demo ;;
  offer-dessert-101) merchant_id=merchant-dessert-demo ;;
  *) echo "no demo merchant mapping for offer ${offer_id}" >&2; exit 1 ;;
esac

echo "[3/5] redeem offer ${offer_id}"
idempotency_key="demo-${journey_id}"
redemption_response=$(curl -fsS -X POST "${REDEMPTION_URL}/v1/redemptions" \
  -H 'Content-Type: application/json' \
  -H "Idempotency-Key: ${idempotency_key}" \
  -d "$(jq -nc \
    --arg user_id_hash "$USER_ID_HASH" \
    --arg offer_id "$offer_id" \
    --arg journey_id "$journey_id" \
    --arg trace_id "$TRACE_ID" \
    --arg idempotency_key "$idempotency_key" \
    '{request_context:{trace_id:$trace_id,journey_id:$journey_id},user_id_hash:$user_id_hash,offer_id:$offer_id,idempotency_key:$idempotency_key}')")
redemption_id=$(jq -er '.redemption.redemption_id' <<<"$redemption_response")
verification_code=$(jq -er '.redemption.merchant_verification_code' <<<"$redemption_response")
jq '.redemption | {redemption_id,journey_id,offer_id,status,points_cost}' <<<"$redemption_response"

echo "[4/5] verify redemption at merchant mock"
verification_response=$(curl -fsS -X POST "${REDEMPTION_URL}/v1/redemptions/${redemption_id}/verify" \
  -H 'Content-Type: application/json' \
  -d "$(jq -nc \
    --arg merchant_id "$merchant_id" \
    --arg verification_code "$verification_code" \
    --arg trace_id "$TRACE_ID" \
    '{request_context:{trace_id:$trace_id},merchant_id:$merchant_id,verification_code:$verification_code}')")
jq '.redemption | {redemption_id,journey_id,offer_id,status}' <<<"$verification_response"

echo "[5/5] demo complete"
echo "journey_id=${journey_id} recommendation_id=${recommendation_id} redemption_id=${redemption_id}"
echo "Grafana: ${GRAFANA_URL:-http://localhost:3000} (business dashboard), ${OTEL_URL:-http://localhost:3001} (traces/metrics)"
