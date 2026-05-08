#!/usr/bin/env bash
# OpenFoundry — Argo CD GitOps bootstrap.
#
# One-shot, idempotent, unattended. Run this once on any cluster where
# you have cluster-admin and a working `kubectl` context. After it
# finishes, Argo CD reconciles the cluster from this Git repository on
# every commit; you never run helm/helmfile by hand again.
#
# The script is safe to re-run: every step is `apply --server-side`
# (Helm `upgrade --install` for the chart). Re-running on a healthy
# cluster is a no-op modulo the wait/banner output.
#
# Usage:
#   ./infra/scripts/argocd-bootstrap.sh                # dev (default)
#   ./infra/scripts/argocd-bootstrap.sh staging
#   ./infra/scripts/argocd-bootstrap.sh prod
#
# Environment overrides (all optional, sane defaults):
#   OPENFOUNDRY_GITOPS_REPO       Git URL Argo CD reads manifests from.
#                                 Default: https://github.com/DioCrafts/OpenFoundry.git
#   OPENFOUNDRY_GITOPS_REVISION   Branch / tag / SHA. Default: main
#   ARGOCD_CHART_VERSION          argo-cd Helm chart pin. Default: 7.7.10
#   ARGOCD_TIMEOUT                Wait timeout for ArgoCD to come up. Default: 600s
#   SKIP_WAIT                     Set to 1 to skip the readiness wait.
#
# Required tools (the script verifies them up-front):
#   - kubectl
#   - helm
#
# This script does not install or pin those tools; install them via
# Homebrew, asdf, mise, or your distro package manager once.
set -euo pipefail

# ─── Inputs ────────────────────────────────────────────────────────
OPENFOUNDRY_ENV="${1:-dev}"
case "$OPENFOUNDRY_ENV" in
  dev|staging|prod) ;;
  *)
    echo "error: environment must be one of dev|staging|prod (got: $OPENFOUNDRY_ENV)" >&2
    exit 2
    ;;
esac

OPENFOUNDRY_GITOPS_REPO="${OPENFOUNDRY_GITOPS_REPO:-https://github.com/DioCrafts/OpenFoundry.git}"
OPENFOUNDRY_GITOPS_REVISION="${OPENFOUNDRY_GITOPS_REVISION:-main}"
ARGOCD_CHART_VERSION="${ARGOCD_CHART_VERSION:-7.7.10}"
ARGOCD_TIMEOUT="${ARGOCD_TIMEOUT:-600s}"
SKIP_WAIT="${SKIP_WAIT:-0}"

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
ARGOCD_DIR="$ROOT_DIR/infra/argocd"

# ─── Helpers ───────────────────────────────────────────────────────
log()   { printf '\033[1;36m[argocd-bootstrap]\033[0m %s\n' "$*"; }
fatal() { printf '\033[1;31m[argocd-bootstrap]\033[0m %s\n' "$*" >&2; exit 1; }

require_tool() {
  command -v "$1" >/dev/null 2>&1 || fatal "$1 not found in PATH. Install it and re-run."
}

# ─── Pre-flight ────────────────────────────────────────────────────
require_tool kubectl
require_tool helm

CTX="$(kubectl config current-context 2>/dev/null || true)"
if [[ -z "$CTX" ]]; then
  fatal "no current kubectl context. Set one with 'kubectl config use-context …' and re-run."
fi

log "OpenFoundry GitOps bootstrap"
log "  environment      : $OPENFOUNDRY_ENV"
log "  kube context     : $CTX"
log "  git repository   : $OPENFOUNDRY_GITOPS_REPO"
log "  git revision     : $OPENFOUNDRY_GITOPS_REVISION"
log "  argo-cd chart    : $ARGOCD_CHART_VERSION"

# ─── 1. Argo CD Helm chart ────────────────────────────────────────
log "[1/5] installing/upgrading argo-cd Helm chart"
helm repo add argo https://argoproj.github.io/argo-helm >/dev/null 2>&1 || true
helm repo update argo >/dev/null

helm upgrade --install argocd argo/argo-cd \
  --namespace argocd \
  --create-namespace \
  --version "$ARGOCD_CHART_VERSION" \
  --values "$ARGOCD_DIR/argocd-helm-values.yaml" \
  --wait \
  --timeout "$ARGOCD_TIMEOUT"

# ─── 2. Wait for the Argo CD CRDs ─────────────────────────────────
if [[ "$SKIP_WAIT" != "1" ]]; then
  log "[2/5] waiting for argocd CRDs to register"
  for crd in applications.argoproj.io appprojects.argoproj.io applicationsets.argoproj.io; do
    kubectl wait --for=condition=Established crd/"$crd" --timeout="$ARGOCD_TIMEOUT"
  done

  log "      waiting for argocd controllers to be Available"
  kubectl -n argocd rollout status deploy/argocd-server --timeout="$ARGOCD_TIMEOUT"
  kubectl -n argocd rollout status deploy/argocd-repo-server --timeout="$ARGOCD_TIMEOUT"
  kubectl -n argocd rollout status statefulset/argocd-application-controller --timeout="$ARGOCD_TIMEOUT" 2>/dev/null \
    || kubectl -n argocd rollout status deploy/argocd-application-controller --timeout="$ARGOCD_TIMEOUT" 2>/dev/null \
    || true
fi

# ─── 3. AppProject + self-managed Application ─────────────────────
log "[3/5] applying AppProject 'openfoundry' + self-managed argocd Application"
kubectl apply --server-side --force-conflicts -f "$ARGOCD_DIR/bootstrap/appproject.yaml"
kubectl apply --server-side --force-conflicts -f "$ARGOCD_DIR/bootstrap/argocd-self-managed.yaml"

# ─── 4. Root app-of-apps ──────────────────────────────────────────
log "[4/5] applying root app-of-apps Application (env=$OPENFOUNDRY_ENV)"
ROOT_APP_RENDERED="$(mktemp -t of-root-app.XXXXXX.yaml)"
trap 'rm -f "$ROOT_APP_RENDERED"' EXIT

# Substitute the three placeholders. We avoid envsubst (not always
# installed) and limit substitution to OPENFOUNDRY_* variables — anything
# else passes through verbatim.
sed \
  -e "s|\${OPENFOUNDRY_ENV}|$OPENFOUNDRY_ENV|g" \
  -e "s|\${OPENFOUNDRY_GITOPS_REPO}|$OPENFOUNDRY_GITOPS_REPO|g" \
  -e "s|\${OPENFOUNDRY_GITOPS_REVISION}|$OPENFOUNDRY_GITOPS_REVISION|g" \
  "$ARGOCD_DIR/bootstrap/root-app.yaml" > "$ROOT_APP_RENDERED"

kubectl apply --server-side --force-conflicts -f "$ROOT_APP_RENDERED"

# ─── 5. Done ──────────────────────────────────────────────────────
log "[5/5] bootstrap complete"
cat <<EOF

  ┌─ Next steps ──────────────────────────────────────────────────┐
  │                                                               │
  │  Argo CD is now reconciling the cluster from:                 │
  │    $OPENFOUNDRY_GITOPS_REPO@$OPENFOUNDRY_GITOPS_REVISION
  │                                                               │
  │  Watch progress:                                              │
  │    make gitops-status                                         │
  │    kubectl -n argocd get applications,applicationsets         │
  │                                                               │
  │  Open the Argo CD UI (port-forward, no ingress yet):          │
  │    kubectl -n argocd port-forward svc/argocd-server 8080:443  │
  │    # then visit https://localhost:8080                        │
  │                                                               │
  │  Initial admin password:                                      │
  │    kubectl -n argocd get secret argocd-initial-admin-secret \\
  │      -o jsonpath='{.data.password}' | base64 -d ; echo        │
  │                                                               │
  │  Optional — wire Slack notifications:                         │
  │    cp infra/argocd/notifications/slack-secret.example.yaml \\
  │       infra/argocd/notifications/slack-secret.yaml            │
  │    \$EDITOR infra/argocd/notifications/slack-secret.yaml       │
  │    kubectl apply -f infra/argocd/notifications/slack-secret.yaml
  │                                                               │
  └───────────────────────────────────────────────────────────────┘
EOF
