#!/usr/bin/env bash
# Smoke-tests the /api/v1/geospatial endpoints exposed by
# ontology-exploratory-analysis-service. Hits every route, reports a
# PASS/FAIL table. Re-runnable — deletes nothing.
#
# Layers are seeded from the JSON files next to this script the first
# run; subsequent runs reuse them (POST /layers returns 201 with a
# fresh ID — duplicate names are allowed by the service).
#
# Usage:  ./tools/dev-smoke/geospatial-smoke.sh [BASE_URL]
# Default BASE_URL is http://localhost:50131 (direct, no Vite proxy).
# Pass http://localhost:5174 to exercise through the dev server proxy.

set -u

BASE="${1:-http://localhost:50131}"
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PAYLOAD_CAPITALS="${SCRIPT_DIR}/payloads/capitals.json"
PAYLOAD_CONTINENTS="${SCRIPT_DIR}/payloads/continents.json"

PASS=0
FAIL=0
results=()

probe() {
  # probe NAME METHOD PATH [BODY_FILE_OR_DASH] [EXPECTED_HTTP_CODE]
  local name="$1" method="$2" path="$3" body="${4:-}" expected="${5:-200}"
  local args=( -sS -o /tmp/geo-smoke-resp.json -w "%{http_code}" -X "$method" "$BASE$path" )
  if [ -n "$body" ] && [ "$body" != "-" ]; then
    args+=( -H "Content-Type: application/json" --data-binary "@$body" )
  fi
  local code
  code=$(curl "${args[@]}" 2>&1) || code="curl-fail"
  local snippet
  snippet=$(head -c 80 /tmp/geo-smoke-resp.json 2>/dev/null | tr -d '\n')
  if [ "$code" = "$expected" ]; then
    results+=("PASS  $name → $code")
    PASS=$((PASS+1))
  else
    results+=("FAIL  $name → got $code, expected $expected — $snippet")
    FAIL=$((FAIL+1))
  fi
}

echo "==> Smoking ${BASE}/api/v1/geospatial/*"
echo

# 1. Read-only baseline
probe "GET /overview"        GET  "/api/v1/geospatial/overview"
probe "GET /layers (initial)" GET "/api/v1/geospatial/layers"
probe "GET /templates"        GET "/api/v1/geospatial/templates"

# 2. Seed two layers
echo
echo "==> Seeding layers"
LAYER_A_ID=$(curl -sS -X POST "$BASE/api/v1/geospatial/layers" \
  -H 'Content-Type: application/json' --data-binary "@${PAYLOAD_CAPITALS}" | tee /tmp/geo-smoke-A.json | \
  python3 -c "import json,sys; print(json.load(sys.stdin).get('id',''))" 2>/dev/null)
LAYER_B_ID=$(curl -sS -X POST "$BASE/api/v1/geospatial/layers" \
  -H 'Content-Type: application/json' --data-binary "@${PAYLOAD_CONTINENTS}" | tee /tmp/geo-smoke-B.json | \
  python3 -c "import json,sys; print(json.load(sys.stdin).get('id',''))" 2>/dev/null)
if [ -n "$LAYER_A_ID" ]; then results+=("PASS  POST /layers (capitals) → id=$LAYER_A_ID"); PASS=$((PASS+1)); else
  results+=("FAIL  POST /layers (capitals) → $(head -c 120 /tmp/geo-smoke-A.json)"); FAIL=$((FAIL+1)); fi
if [ -n "$LAYER_B_ID" ]; then results+=("PASS  POST /layers (continents) → id=$LAYER_B_ID"); PASS=$((PASS+1)); else
  results+=("FAIL  POST /layers (continents) → $(head -c 120 /tmp/geo-smoke-B.json)"); FAIL=$((FAIL+1)); fi

# 3. Read after write
probe "GET /overview (after seed)" GET "/api/v1/geospatial/overview"
probe "GET /layers (after seed)"   GET "/api/v1/geospatial/layers"

# 4. Update layer (rename)
if [ -n "$LAYER_A_ID" ]; then
  cat > /tmp/geo-smoke-update.json <<EOF
{"description":"Smoke-updated description on $(date -u +%Y-%m-%dT%H:%M:%SZ)"}
EOF
  probe "PUT /layers/{id} (update description)" PUT "/api/v1/geospatial/layers/${LAYER_A_ID}" /tmp/geo-smoke-update.json
fi

# 5. Vector tile fetch (z=0, x=0, y=0 — the world tile)
if [ -n "$LAYER_A_ID" ]; then
  probe "GET /tiles/{id} (z=0/x=0/y=0)"          GET "/api/v1/geospatial/tiles/${LAYER_A_ID}?z=0&x=0&y=0"
  probe "GET /tiles/{id}/features (viewport)"    GET "/api/v1/geospatial/tiles/${LAYER_A_ID}/features?min_lat=-90&min_lon=-180&max_lat=90&max_lon=180"
fi

# 6. Spatial query — "within" a global bbox should match every capital
cat > /tmp/geo-smoke-query.json <<EOF
{"layer_id":"${LAYER_A_ID:-00000000-0000-0000-0000-000000000000}","operation":"within","bounds":{"min_lat":-90,"min_lon":-180,"max_lat":90,"max_lon":180}}
EOF
probe "POST /query (within global bbox)" POST "/api/v1/geospatial/query" /tmp/geo-smoke-query.json

# 7. Clusters
cat > /tmp/geo-smoke-cluster.json <<EOF
{"layer_id":"${LAYER_A_ID:-00000000-0000-0000-0000-000000000000}","algorithm":"dbscan","epsilon_km":1500,"min_points":2}
EOF
probe "POST /clusters (dbscan ε=1500km)" POST "/api/v1/geospatial/clusters" /tmp/geo-smoke-cluster.json

# 8. Forward geocode
cat > /tmp/geo-smoke-geocode.json <<EOF
{"address":"Reykjavík"}
EOF
probe "POST /geocode (forward, query='Reykjavík')" POST "/api/v1/geospatial/geocode" /tmp/geo-smoke-geocode.json

# 9. Reverse geocode
cat > /tmp/geo-smoke-revgeocode.json <<EOF
{"coordinate":{"lat":64.1466,"lon":-21.9426}}
EOF
probe "POST /reverse-geocode (Reykjavík coords)" POST "/api/v1/geospatial/reverse-geocode" /tmp/geo-smoke-revgeocode.json

# Summary
echo
echo "==> Results"
printf '%s\n' "${results[@]}"
echo
echo "==> $PASS passed, $FAIL failed"
exit $FAIL
