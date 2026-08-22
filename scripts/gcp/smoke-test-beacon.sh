#!/usr/bin/env bash
set -euo pipefail

APP_DIR=${APP_DIR:-/opt/spacetime-node}
BEACON_UUID=${1:-demo-beacon}
BEACON_MAJOR=${2:-1}
BEACON_MINOR=${3:-4}
BEACON_POWER=${4:-0}

cd "$APP_DIR"

payload=$(printf '{"uuid":"%s","major":%s,"minor":%s,"power":%s}' \
  "$BEACON_UUID" "$BEACON_MAJOR" "$BEACON_MINOR" "$BEACON_POWER")

printf 'Checking mobility Beacon resolution...\n'
mobility_response=$(curl -fsS http://127.0.0.1:8001/v1/mobility/beacon/resolve \
  -H 'Content-Type: application/json' \
  --data "$payload")
printf '%s\n' "$mobility_response"

printf '\nChecking Gateway Beacon entry path...\n'
curl -fsS http://127.0.0.1:8000/v1/entry-events \
  -H 'Content-Type: application/json' \
  --data "{\"user_id_hash\":\"sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa\",\"beacon\":$payload}"
printf '\n'

printf '\nBeacon smoke test complete.\n'
