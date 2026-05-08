# OpenFoundry — `infra/`

Single-source-of-truth for everything that runs OpenFoundry: Kubernetes
via Helm, Docker Compose for local dev, runbooks, scripts, Terraform.

## Layout

```
infra/
├── argocd/           ← GitOps engine. ONE command bootstrap; ArgoCD reconciles everything below.
│   ├── argocd-helm-values.yaml   # values for the argo-cd Helm chart
│   ├── bootstrap/                # AppProject + root app-of-apps + self-managed ArgoCD
│   ├── apps/{dev,staging,prod}/  # ApplicationSets per environment
│   └── notifications/            # Slack notifications config + Secret template
│
├── helm/             ← Kubernetes via Helm — chart definitions + values
│   ├── helmfile.yaml.gotmpl   # legacy entrypoint, kept for break-glass
│   ├── apps/                  # OpenFoundry application charts
│   ├── operators/             # third-party operators (cnpg, strimzi, …)
│   ├── infra/                 # third-party infra clusters / CRs
│   ├── _shared/               # shared library chart (templates only)
│   ├── profiles/              # cross-release values overlays
│   └── docs/                  # DSN contract, migration notes
│
├── compose/          ← Docker Compose dev environment (alternative to k8s)
│
├── observability/    ← Prometheus rules + Grafana dashboards (consumed by helm/)
│
├── runbooks/         ← Operational playbooks (Markdown)
│
├── scripts/          ← One-off helper scripts (backups, dev-stack, smoke tests)
│   └── argocd-bootstrap.sh   # one-shot, unattended GitOps bootstrap
│
├── terraform/        ← Cloud infra (DNS, Ceph, CDN). Orthogonal to k8s.
│
└── test-tools/       ← Load benchmarks + chaos experiments
```

## Quick reference

| I want to … | Where to look |
| --- | --- |
| Bootstrap a fresh cluster (GitOps) | `make gitops-bootstrap` (or `GITOPS_ENV=prod make gitops-bootstrap`) |
| Watch ArgoCD reconcile | `make gitops-status` |
| Open the ArgoCD UI | `make gitops-ui` |
| Deploy everything via Helm directly (break-glass) | `cd infra/helm && helmfile -e dev apply` |
| Run with Docker Compose | `cd infra/compose && docker compose up` |
| Add a new OpenFoundry service | `infra/helm/apps/of-<release>/` |
| Add a new third-party operator | `infra/helm/operators/<name>/` + register in `infra/argocd/apps/<env>/00-upstream-charts.yaml` |
| Add a new third-party cluster CR | `infra/helm/infra/<name>/` + register in `infra/argocd/apps/<env>/10-intree-charts.yaml` |
| Find a Postgres DSN convention | `infra/helm/docs/DATABASE_URL.md` |
| Investigate a runtime incident | `infra/runbooks/` |
| Understand observability rules | `infra/observability/` |

## Helm: install order (enforced by `needs:` in the helmfile)

```
Layer 1 — operators/   (cert-manager, cnpg, k8ssandra, strimzi, rook, flink)
Layer 2 — infra/       (postgres clusters, cassandra cluster, kafka cluster,
                        ceph cluster, temporal, lakekeeper, debezium,
                        flink-jobs, vespa, trino, spark-{operator,jobs}, mimir,
                        observability, local-registry)
Layer 3 — apps/        (of-platform, then of-data-engine | of-ontology |
                        of-ml-aip | of-apps-ops, then of-web)
```

## How deployment works

**Default path (recommended): GitOps with ArgoCD.** One command bootstraps
ArgoCD; from then on every commit to `main` is reconciled into the cluster
automatically with auto-sync, prune and self-heal. See
[`argocd/README.md`](argocd/README.md) for the full guide.

```sh
make gitops-bootstrap                  # dev (default)
make gitops-bootstrap GITOPS_ENV=prod  # production
```

**Break-glass path: direct Helmfile.** The same chart tree is still
runnable by hand for cluster bring-up before ArgoCD itself is healthy
or for offline diff/template:

```sh
cd infra/helm
helmfile -e dev template   # render only
helmfile -e dev diff       # diff against current cluster
helmfile -e dev apply      # apply directly (no ArgoCD involvement)
```

`helmfile -e dev apply` runs every layer in order; profile gates skip
heavy releases on the dev profile (Vespa, Trino, Spark, Mimir, Rook,
Flink stay disabled).
