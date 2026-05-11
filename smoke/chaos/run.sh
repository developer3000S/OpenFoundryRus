#!/usr/bin/env bash
#
# Suite de chaos del data plane.
#
# Para cada script de chaos en este directorio:
#   1. Lo ejecuta (mata 1 pod, espera health verde).
#   2. A continuación corre los scenarios críticos p2..p6 con el runner
#      Go `tools/of-cli`:
#      `go run ./tools/of-cli -- smoke run --scenario <s> --output <o>`.
#
# Falla si CUALQUIER scenario falla bajo CUALQUIER chaos: ese es el
# contrato no-SPOF del data plane.

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
ROOT_DIR="$(cd "$SCRIPT_DIR/../.." && pwd)"
SCENARIOS_DIR="$ROOT_DIR/smoke/scenarios"
RESULTS_DIR="${CHAOS_RESULTS_DIR:-$ROOT_DIR/smoke/results/chaos}"

# Permite override; por defecto usa el binario compilable in-repo.
: "${OF_CLI:=go run ./tools/of-cli --}"

CHAOS_SCRIPTS=(
  "kill-one-mon.sh"
  "kill-one-kafka-broker.sh"
  "kill-active-kafka-controller.sh"
  "kill-one-nats-node.sh"
  "kill-pg-primary.sh"
)

SCENARIOS=(
  "p2-runtime-critical-path.json"
  "p3-semantic-governance-critical-path.json"
  "p4-developer-platform-critical-path.json"
  "p5-ai-ml-critical-path.json"
  "p6-analytics-enterprise-critical-path.json"
)

mkdir -p "$RESULTS_DIR"

failures=()

run_scenarios() {
  local chaos_label="$1"
  local scenario_path output_path
  for scenario in "${SCENARIOS[@]}"; do
    scenario_path="$SCENARIOS_DIR/$scenario"
    output_path="$RESULTS_DIR/${chaos_label}__${scenario}"
    if [[ ! -f "$scenario_path" ]]; then
      echo "chaos: scenario inexistente: $scenario_path" >&2
      failures+=("$chaos_label/$scenario:missing")
      continue
    fi
    echo "::group::chaos=$chaos_label scenario=$scenario"
    # shellcheck disable=SC2086  # OF_CLI is intentionally word-split
    if ! ( cd "$ROOT_DIR" && $OF_CLI smoke run \
        --scenario "$scenario_path" \
        --output "$output_path" ); then
      failures+=("$chaos_label/$scenario")
    fi
    echo "::endgroup::"
  done
}

for chaos_script in "${CHAOS_SCRIPTS[@]}"; do
  chaos_path="$SCRIPT_DIR/$chaos_script"
  chaos_label="${chaos_script%.sh}"
  echo "==> chaos step: $chaos_label"
  if [[ ! -x "$chaos_path" ]]; then
    echo "chaos: script no ejecutable o ausente: $chaos_path" >&2
    failures+=("$chaos_label:missing-script")
    continue
  fi
  if ! "$chaos_path"; then
    echo "chaos: $chaos_label falló durante la inyección" >&2
    failures+=("$chaos_label:inject")
    continue
  fi
  run_scenarios "$chaos_label"
done

if (( ${#failures[@]} > 0 )); then
  echo
  echo "FAIL: chaos suite con ${#failures[@]} fallos:" >&2
  for f in "${failures[@]}"; do
    echo "  - $f" >&2
  done
  exit 1
fi

echo
echo "PASS: todos los scenarios pasaron bajo todos los chaos."
