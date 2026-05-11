# `tools/ceph-lint`

Static contract lint for the rendered Rook-Ceph manifest. The live source is
the Helm chart under `infra/helm/infra/ceph-cluster/`; the linter renders that
chart before checking the contract.

## Why

Ceph is the sumidero of every persistent workload in OpenFoundry: Iceberg
parquet, Kafka tiered storage, Postgres WAL, model artefacts, dataset
buckets. Its quorum (mon) and rack/AZ-awareness (CRUSH `failureDomain`)
are *invisible by default* — when they regress, nothing breaks until the
day a zone or rack actually goes down, and at that point the whole
lakehouse goes with it.

This linter encodes the contract once so a regression cannot reach `main`:

- **Mon quorum**: `mon.count` is odd and `>= 3`; `allowMultiplePerNode`
  is `false`.
- **Mgr HA**: `mgr.count >= 2`; `allowMultiplePerNode` is `false`.
- **Mon/Mgr placement**: `placement.mon.topologySpreadConstraints` and
  `placement.mgr.topologySpreadConstraints` declare an AZ-level (or
  rack-level) `topologyKey` so the scheduler cannot land every mon (or
  every mgr) in the same zone. A node-level key
  (`kubernetes.io/hostname`) on its own does **not** satisfy the contract.
- **Disruption management**: `disruptionManagement.managePodBudgets:
  true` so a `kubectl drain` cannot evict more mons or OSDs than the
  quorum / CRUSH map tolerates.
- **Pool failure domain**: every `CephBlockPool`, direct-pool
  `CephObjectStore`, and `CephObjectZone` pool uses `failureDomain` ∈
  {`zone`, `rack`, `region`, `datacenter`, `room`}. `host` is rejected by
  default; documented legacy resources kept for backwards-compatibility live
  in an explicit allowlist (`LEGACY_HOST_FAILURE_DOMAIN_ALLOWLIST`) inside the
  script and are cross-referenced in
  `infra/helm/infra/ceph-cluster/README.md`.

The `CephFilesystem` data/metadata pools are checked too if any are added
in the future, even though none is shipped today.

## Run locally

```
pip install pyyaml
python3 tools/ceph-lint/check_topology.py
```

With no path argument, the script runs `helm template ceph-cluster
infra/helm/infra/ceph-cluster` internally. You can also pass an already
rendered manifest path:

```
helm template ceph-cluster infra/helm/infra/ceph-cluster > /tmp/ceph-cluster.yaml
python3 tools/ceph-lint/check_topology.py /tmp/ceph-cluster.yaml
```

The script exits 0 on success and 1 on any contract violation, printing
each violated invariant.

## CI

Runs on every push and pull request that touches `infra/helm/infra/ceph-cluster/**` or
this tool — see `.github/workflows/ceph-lint.yml`.
