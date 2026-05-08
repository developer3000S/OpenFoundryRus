# `infra/argocd/` — GitOps with Argo CD

Argo CD is the deployment engine for OpenFoundry. Every operator,
infrastructure cluster and application release is reconciled from this
Git repository on every commit. **You do not run `helm`, `helmfile` or
`kubectl apply` by hand any more** — you push to `main` and the cluster
catches up.

## Install — one command, fully unattended

Prerequisite: a Kubernetes context where you have `cluster-admin`, and
`kubectl` + `helm` on your `PATH`. That's it.

```sh
# default = dev environment, repo = github.com/DioCrafts/OpenFoundry, revision = main
make gitops-bootstrap

# or, for staging / prod:
make gitops-bootstrap GITOPS_ENV=staging
make gitops-bootstrap GITOPS_ENV=prod
```

That single command:

1. Installs the upstream `argo/argo-cd` Helm chart into the `argocd`
   namespace using [`argocd-helm-values.yaml`](argocd-helm-values.yaml).
2. Waits for the Argo CD CRDs and controllers to be ready.
3. Applies the `AppProject openfoundry`
   ([`bootstrap/appproject.yaml`](bootstrap/appproject.yaml)).
4. Applies the self-managed Application
   ([`bootstrap/argocd-self-managed.yaml`](bootstrap/argocd-self-managed.yaml))
   so subsequent edits to `argocd-helm-values.yaml` in Git roll out
   automatically — Argo CD upgrades itself.
5. Applies the root app-of-apps Application
   ([`bootstrap/root-app.yaml`](bootstrap/root-app.yaml)) pointed at
   `infra/argocd/apps/<env>/`. Argo CD then reconciles every release
   declared there.

The script is idempotent. Re-running it on a healthy cluster is a no-op.

### Customising the source repo

Default repo / revision are baked in (this file is the source of truth);
override them only if you fork or rename:

```sh
OPENFOUNDRY_GITOPS_REPO=https://github.com/your-org/your-fork.git \
OPENFOUNDRY_GITOPS_REVISION=release-2026-Q2 \
make gitops-bootstrap
```

Permanent fork? Update the `repoURL` value in two places:

* [`bootstrap/argocd-self-managed.yaml`](bootstrap/argocd-self-managed.yaml) — the self-managed Application
* `infra/argocd/apps/<env>/*-charts.yaml` — every ApplicationSet template

A single `sed -i 's|github.com/DioCrafts/OpenFoundry|github.com/your-org/your-fork|g' infra/argocd/**/*.yaml`
covers them all.

## What lives here

```
infra/argocd/
├── README.md                              # this file
├── argocd-helm-values.yaml                # values applied to the argo-cd chart
├── bootstrap/
│   ├── namespace.yaml                     # `argocd` ns (PSS-restricted)
│   ├── appproject.yaml                    # AppProject openfoundry — repo/dest/role whitelists
│   ├── argocd-self-managed.yaml           # Argo CD reconciles its own Helm release
│   └── root-app.yaml                      # app-of-apps pointing at apps/<env>/
├── apps/                                  # everything Argo CD generates from
│   ├── dev/
│   │   ├── 00-upstream-charts.yaml        # ApplicationSet — operators + kite (upstream Helm charts)
│   │   └── 10-intree-charts.yaml          # ApplicationSet — infra + apps (in-tree charts)
│   ├── staging/
│   │   ├── 00-upstream-charts.yaml
│   │   └── 10-intree-charts.yaml
│   └── prod/
│       ├── 00-upstream-charts.yaml
│       └── 10-intree-charts.yaml
└── notifications/
    ├── slack-secret.example.yaml          # template for the Slack token Secret
    └── (slack-secret.yaml — gitignored, you create this)
```

## How releases are organised

Two ApplicationSets per environment generate one Application per release:

* **`00-upstream-charts.yaml`** — releases that pull a remote chart
  (cert-manager, cnpg, k8ssandra-operator, strimzi, rook-ceph, flink,
  kite). Each entry pairs a chart with a values file in
  `infra/helm/operators/<name>/values.yaml` via the multi-source
  `$values` ref, so values stay in the existing tree — zero
  duplication.

* **`10-intree-charts.yaml`** — releases whose chart lives in this
  repo under `infra/helm/{infra,apps}/`. The Application's `path`
  points at the chart directory; `valueFiles` enumerates the per-env
  overlays (`values.yaml`, `values-<env>.yaml`, and the cross-cutting
  `infra/helm/profiles/values-<env>.yaml`).

### Sync waves

Argo CD reconciles in three waves to honour cross-release dependencies:

| Wave | Releases                                                                               |
|------|----------------------------------------------------------------------------------------|
| -100 | `argocd` (self-managed)                                                                |
| -50  | `openfoundry-root` (app-of-apps)                                                        |
|  0   | Operators: cert-manager, cnpg, k8ssandra-operator, strimzi, rook-ceph (prod), flink (prod) |
|  5   | kite                                                                                    |
| 10   | Infra: postgres-clusters, cassandra-cluster, kafka-cluster, ceph-cluster (prod), lakekeeper, debezium, vespa (≥staging), trino (prod), spark-operator (prod), mimir (prod), observability (≥staging), local-registry (dev) |
| 15   | flink-jobs (prod), spark-jobs (prod)                                                    |
| 20   | of-platform (gateway, identity, authz, tenancy)                                         |
| 30   | of-data-engine, of-ontology, of-ml-aip, of-apps-ops (parallel)                         |
| 40   | of-web                                                                                  |

### Per-environment release matrix

Mirrors the existing helmfile gates:

| Release            | dev | staging | prod |
|--------------------|:---:|:-------:|:----:|
| cert-manager       | ✅  | ✅      | ✅   |
| cnpg               | ✅  | ✅      | ✅   |
| k8ssandra-operator | ✅  | ✅      | ✅   |
| strimzi-operator   | ✅  | ✅      | ✅   |
| rook-ceph-operator | ❌  | ❌      | ✅   |
| flink-operator     | ❌  | ❌      | ✅   |
| kite               | ✅  | ✅      | ✅   |
| postgres-clusters  | ✅  | ✅      | ✅   |
| cassandra-cluster  | ✅  | ✅      | ✅   |
| kafka-cluster      | ✅  | ✅      | ✅   |
| ceph-cluster       | ❌  | ❌      | ✅   |
| lakekeeper         | ✅  | ✅      | ✅   |
| debezium           | ✅  | ✅      | ✅   |
| flink-jobs         | ❌  | ❌      | ✅   |
| vespa              | ❌  | ✅      | ✅   |
| trino              | ❌  | ❌      | ✅   |
| spark-operator     | ❌  | ❌      | ✅   |
| spark-jobs         | ❌  | ❌      | ✅   |
| mimir              | ❌  | ❌      | ✅   |
| observability      | ❌  | ✅      | ✅   |
| local-registry     | ✅  | ❌      | ❌   |
| of-platform        | ✅  | ✅      | ✅   |
| of-data-engine     | ✅  | ✅      | ✅   |
| of-ontology        | ✅  | ✅      | ✅   |
| of-ml-aip          | ✅  | ✅      | ✅   |
| of-apps-ops        | ✅  | ✅      | ✅   |
| of-web             | ✅  | ✅      | ✅   |

### Sync policy

Every Application opts into auto-sync, prune and self-heal:

```yaml
syncPolicy:
  automated: { prune: true, selfHeal: true }
  syncOptions:
    - CreateNamespace=true
    - ServerSideApply=true
    - ApplyOutOfSyncOnly=true
  retry: { limit: 5, backoff: { duration: 30s, factor: 2, maxDuration: 10m } }
```

`prune: true` means resources removed from Git are deleted from the
cluster. `selfHeal: true` means manual `kubectl edit` on a managed
resource is reverted on the next reconciliation. The setup is fully
declarative — the Git repo is the only source of truth.

## Day-2 operations

```sh
make gitops-status       # list every Application + ApplicationSet
make gitops-sync         # request a hard refresh on every Application
make gitops-ui           # port-forward the Argo CD UI to localhost:8080
make gitops-uninstall    # remove ArgoCD; managed workloads stay
```

To update a release: edit the relevant chart / values file in
`infra/helm/...`, commit, push. Argo CD picks it up within ~3 minutes
(the default poll interval) or instantly if you wire a webhook.

To pin a different revision:

```sh
OPENFOUNDRY_GITOPS_REVISION=v0.5.2 make gitops-bootstrap GITOPS_ENV=prod
```

## Slack notifications

Sync failures, health regressions and successful syncs are wired
through Argo CD Notifications to Slack. The webhook lives in a Secret
the user provides. Until that Secret exists, notifications no-op
silently — bootstrap is safe without it.

```sh
cp infra/argocd/notifications/slack-secret.example.yaml \
   infra/argocd/notifications/slack-secret.yaml
$EDITOR infra/argocd/notifications/slack-secret.yaml
kubectl apply -f infra/argocd/notifications/slack-secret.yaml
```

The real `slack-secret.yaml` is gitignored.

## Hardening checklist (before exposing to the open internet)

The default install is intentionally pragmatic — auth via the
auto-generated admin password, no ingress, plaintext server (TLS
terminated by ingress). Before exposing externally:

1. Wire SSO (Dex / OIDC). Toggle `dex.enabled: true` in
   [`argocd-helm-values.yaml`](argocd-helm-values.yaml) and configure
   your IdP.
2. Disable the bootstrap admin user
   (`configs.cm.admin.enabled: "false"`).
3. Enable the ingress (`server.ingress.enabled: true`) once you have
   a `cert-manager` `ClusterIssuer` ready.
4. Add a PreSync hook that runs validation jobs (`kubeval`, `polaris`,
   `kyverno`) — scaffolded but opt-in; not enabled by default to keep
   the install dependency-free.
5. Move `slack-token` (and any other secret) into Vault or
   External-Secrets-Operator instead of plain Kubernetes Secrets.
