#!/usr/bin/env bash
# tools/online-retail/bootstrap_ontology.sh
#
# Bootstraps the Online Retail PoC ontology, link types and actions on top of
# the four Iceberg tables produced by the Spark pipeline (lakekeeper.default.*).
# Idempotent: if an entity already exists by name, the script reuses it.
#
# Pre-reqs:
#   - The 4 derived tables exist (apply infra/dev/poc-pipeline-nodes.yaml first)
#   - The ontology-* services are reachable via edge-gateway-service
#
# Usage:
#   tools/online-retail/bootstrap_ontology.sh \
#       --gateway-url http://localhost:18080 \
#       --token "$JWT"
set -euo pipefail

GATEWAY_URL="${GATEWAY_URL:-http://localhost:18080}"
TOKEN="${TOKEN:-}"
WAREHOUSE="${WAREHOUSE:-openfoundry}"
NAMESPACE="${NAMESPACE:-default}"

while [[ $# -gt 0 ]]; do
  case "$1" in
    --gateway-url) GATEWAY_URL="$2"; shift 2 ;;
    --token)       TOKEN="$2"; shift 2 ;;
    *) echo "unknown arg: $1"; exit 2 ;;
  esac
done

if [[ -z "$TOKEN" ]]; then
  echo "ERROR: pass --token <jwt> or set TOKEN env var (Foundry JWT for the ontology API)"
  exit 1
fi

api() {
  local method="$1" path="$2" body="${3:-}"
  if [[ -n "$body" ]]; then
    curl -sw "\nHTTP=%{http_code}\n" -X "$method" "$GATEWAY_URL$path" \
      -H "Authorization: Bearer $TOKEN" \
      -H "Content-Type: application/json" \
      -d "$body"
  else
    curl -sw "\nHTTP=%{http_code}\n" -X "$method" "$GATEWAY_URL$path" \
      -H "Authorization: Bearer $TOKEN"
  fi
}

# ── Object types ─────────────────────────────────────────────────────────
# Each type points at one Iceberg table that the Spark pipeline already
# materialised. The backing-dataset wiring is stored as an annotation on
# the type so the dashboard can resolve it later without re-querying
# Lakekeeper.
echo "[1/3] creating object types"
for t in customer transaction product; do
  case $t in
    customer)
      payload='{"name":"customer","display_name":"Customer","primary_key_property":"customer_id","description":"Aggregated metrics per customer","icon":"users","color":"#2d72d2","backing_table":"lakekeeper.default.customer_metrics"}'
      ;;
    transaction)
      payload='{"name":"transaction","display_name":"Transaction","primary_key_property":"transaction_id","description":"Order line with anomaly flag and editable review_status","icon":"object","color":"#cf923f","backing_table":"lakekeeper.default.transactions_anomalies"}'
      ;;
    product)
      payload='{"name":"product","display_name":"Product","primary_key_property":"stockcode","description":"Stock-keeping unit","icon":"cube","color":"#15803d","backing_table":"lakekeeper.default.transactions_clean"}'
      ;;
  esac
  api POST /api/v1/ontology/types "$payload" | tail -3
done

# ── Properties ──────────────────────────────────────────────────────────
# `transaction.review_status` is the writeback target for actions —
# stored editable enum with default 'pending'.
echo "[2/3] creating editable review_status property on transaction"
TX_ID=$(curl -s "$GATEWAY_URL/api/v1/ontology/types?search=transaction" -H "Authorization: Bearer $TOKEN" | sed -n 's/.*"id":"\([^"]*\)".*/\1/p' | head -1)
echo "transaction type id=$TX_ID"
api POST "/api/v1/ontology/types/$TX_ID/properties" \
  '{"name":"review_status","display_name":"Review status","property_type":"enum","required":false,"default_value":"pending","validation_rules":{"enum_values":["pending","reviewed","escalated"]},"inline_edit_config":{"enabled":true}}' | tail -3

# ── Actions ─────────────────────────────────────────────────────────────
echo "[3/3] creating MarkAsReviewed + EscalateAnomaly actions"
api POST /api/v1/ontology/actions \
  "{\"name\":\"mark_as_reviewed\",\"display_name\":\"MarkAsReviewed\",\"object_type_id\":\"$TX_ID\",\"operation_kind\":\"update_object\",\"input_schema\":[],\"config\":{\"property_mappings\":[{\"property_name\":\"review_status\",\"kind\":\"static\",\"static_value\":\"reviewed\"}]}}" | tail -3
api POST /api/v1/ontology/actions \
  "{\"name\":\"escalate_anomaly\",\"display_name\":\"EscalateAnomaly\",\"object_type_id\":\"$TX_ID\",\"operation_kind\":\"update_object\",\"input_schema\":[],\"config\":{\"property_mappings\":[{\"property_name\":\"review_status\",\"kind\":\"static\",\"static_value\":\"escalated\"}]}}" | tail -3

echo "OK"
