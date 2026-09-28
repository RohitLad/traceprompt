#!/bin/bash
# Smoke test: verifies a running stack end-to-end.
# Usage: ./scripts/smoke.sh [API_URL]   (default http://localhost:3000)
set -euo pipefail

API="${1:-http://localhost:3000}"
echo "-> smoke against $API"

health=$(curl -sf "$API/api/health")
echo "   health: $health"

# Register a throwaway user (unique email per run).
EMAIL="smoke-$(date +%s)@example.com"
REG=$(curl -sf -X POST "$API/api/v1/auth/register" \
  -H 'Content-Type: application/json' \
  -d "{\"email\":\"$EMAIL\",\"password\":\"password-123\",\"name\":\"Smoke\"}")
TOKEN=$(echo "$REG" | python3 -c 'import json,sys; print(json.load(sys.stdin)["token"])')
ORG=$(echo "$REG" | python3 -c 'import json,sys; print(json.load(sys.stdin)["org"]["id"])')
echo "   registered $EMAIL"

PROJ=$(curl -sf -X POST "$API/api/v1/projects" \
  -H "Authorization: Bearer $TOKEN" -H 'Content-Type: application/json' \
  -d "{\"name\":\"smoke\",\"organizationId\":\"$ORG\"}")
PID=$(echo "$PROJ" | python3 -c 'import json,sys; print(json.load(sys.stdin)["id"])')
echo "   project $PID"

KEYS=$(curl -sf -X POST "$API/api/v1/projects/$PID/keys" \
  -H "Authorization: Bearer $TOKEN" -H 'Content-Type: application/json' \
  -d '{"name":"smoke"}')
PUB=$(echo "$KEYS" | python3 -c 'import json,sys; print(json.load(sys.stdin)["publicKey"])')
SEC=$(echo "$KEYS" | python3 -c 'import json,sys; print(json.load(sys.stdin)["secret"])')
BASIC=$(echo -n "$PUB:$SEC" | base64)

NOW=$(date -u +%Y-%m-%dT%H:%M:%SZ)
INGEST=$(curl -sf -X POST "$API/api/public/ingestion" \
  -H "Authorization: Basic $BASIC" -H 'Content-Type: application/json' \
  -d "{\"batch\":[{\"id\":\"smoke-e1\",\"type\":\"trace-create\",\"timestamp\":\"$NOW\",\"body\":{\"id\":\"smoke-t1\",\"name\":\"smoke\"}}]}")
echo "   ingest: $INGEST"

# Poll for the worker to persist (up to 30s).
for _ in $(seq 1 30); do
  TRACES=$(curl -sf -H "Authorization: Bearer $TOKEN" "$API/api/v1/projects/$PID/traces" || echo '{}')
  N=$(echo "$TRACES" | python3 -c 'import json,sys; print(len(json.load(sys.stdin).get("data", [])))' 2>/dev/null || echo 0)
  if [ "$N" -ge 1 ]; then
    echo "   traces visible: $N"
    echo "SMOKE PASSED"
    exit 0
  fi
  sleep 1
done
echo "SMOKE FAILED: trace never appeared" >&2
exit 1
