#!/usr/bin/env python3
"""Rook-Ceph topology contract lint.

Validates the rendered Rook-Ceph manifest from
``infra/helm/infra/ceph-cluster`` against the quorum, high-availability and
rack/AZ-awareness invariants the lakehouse and Kafka tiered storage depend on.
Fails (exit code 1) on any drift. The intent is the same as
``tools/kafka-lint/check_kraft.py``: encode the contract once so accidental
regressions in PR review are impossible.

The contract checked here mirrors the P0 requirements documented in the
ROADMAP and in ``infra/helm/infra/ceph-cluster/README.md``:

    1. Mon quorum: ``mon.count`` is odd and ``>= 3``;
       ``allowMultiplePerNode`` is ``false``.
    2. Mgr HA: ``mgr.count >= 2``; ``allowMultiplePerNode`` is ``false``.
    3. Mon/Mgr placement: a ``topologySpreadConstraints`` entry with an
       AZ-level (or rack-level) ``topologyKey`` is declared so quorum
       survives a zone outage. A node-level key (``kubernetes.io/hostname``)
       does NOT satisfy the contract on its own — only as an additional
       constraint.
    4. Disruption management: ``managePodBudgets: true`` so that a node
       drain cannot evict more mon/OSD pods than the quorum / CRUSH map
       tolerates.
    5. Pool failure domain: every ``CephBlockPool``, direct-pool
       ``CephObjectStore`` and ``CephObjectZone`` declares
       ``failureDomain`` ∈ {``zone``, ``rack``, ``region``, ``datacenter``,
       ``room``}. ``host`` is rejected (silently degrades to "tolerate one
       node" rather than "tolerate one zone/rack") with a narrow allowlist for
       documented legacy resources kept for backwards-compatibility.

Usage::

    python3 tools/ceph-lint/check_topology.py
    python3 tools/ceph-lint/check_topology.py path/to/rendered-ceph.yaml

With no argument the script runs ``helm template`` against the current chart.
Requires Helm and PyYAML. The CI workflow installs both; locally run
``pip install pyyaml`` if PyYAML is missing.
"""

from __future__ import annotations

import subprocess
import sys
from pathlib import Path
from typing import Any

try:
    import yaml
except ImportError:  # pragma: no cover - exercised only when PyYAML missing
    print(
        "error: PyYAML is required. Install it with `pip install pyyaml`.",
        file=sys.stderr,
    )
    sys.exit(2)


DEFAULT_CHART = Path("infra/helm/infra/ceph-cluster")

# Topology keys that count as "AZ-aware or stronger". A pure host-level key
# (``kubernetes.io/hostname``) does not satisfy the spread contract on its
# own — the whole point is to survive losing a zone/rack, not just a node.
ACCEPTABLE_SPREAD_KEYS = frozenset(
    {
        "topology.kubernetes.io/zone",
        "topology.kubernetes.io/region",
        "topology.rook.io/rack",
        "topology.rook.io/datacenter",
        "topology.rook.io/room",
        "failure-domain.beta.kubernetes.io/zone",
    }
)

ACCEPTABLE_FAILURE_DOMAINS = frozenset(
    {"zone", "rack", "region", "datacenter", "room"}
)

# Pools knowingly accepted at ``failureDomain: host`` because they pre-date
# the contract and migrating them in-place forces a destructive bucket
# re-creation. Every entry MUST be documented in
# ``infra/helm/infra/ceph-cluster/README.md`` (see the "CRUSH layout and pool assignment"
# table) so the deuda técnica stays visible.
LEGACY_HOST_FAILURE_DOMAIN_ALLOWLIST: frozenset[tuple[str, str, str]] = (
    frozenset(
        {
            # (kind, name, pool_label)
            ("CephObjectStore", "openfoundry", "metadataPool"),
            ("CephObjectStore", "openfoundry", "dataPool"),
        }
    )
)


def repo_root() -> Path:
    # tools/ceph-lint/check_topology.py -> repo root is two levels up.
    return Path(__file__).resolve().parent.parent.parent


def parse_documents(text: str, source: str) -> list[dict[str, Any]]:
    docs = [d for d in yaml.safe_load_all(text) if isinstance(d, dict)]
    if not docs:
        raise ValueError(f"{source}: no YAML documents found")
    return docs


def load_documents(path: Path) -> list[dict[str, Any]]:
    return parse_documents(path.read_text(), str(path))


def render_default_documents() -> list[dict[str, Any]]:
    chart = repo_root() / DEFAULT_CHART
    if not chart.is_dir():
        raise ValueError(f"default chart not found at {chart}")

    result = subprocess.run(
        ["helm", "template", "ceph-cluster", str(chart)],
        check=False,
        cwd=repo_root(),
        stdout=subprocess.PIPE,
        stderr=subprocess.PIPE,
        text=True,
    )
    if result.returncode != 0:
        raise ValueError(
            "helm template failed for "
            f"{chart}: {result.stderr.strip() or result.stdout.strip()}"
        )
    return parse_documents(result.stdout, f"helm template {chart}")


def find_by_kind(
    docs: list[dict[str, Any]], kind: str
) -> list[dict[str, Any]]:
    return [d for d in docs if d.get("kind") == kind]


def _name(doc: dict[str, Any]) -> str:
    return (doc.get("metadata") or {}).get("name", "<unnamed>")


def check_mon(cluster: dict[str, Any], errors: list[str]) -> None:
    spec = cluster.get("spec") or {}
    mon = spec.get("mon") or {}

    count = mon.get("count")
    if not isinstance(count, int) or count < 3:
        errors.append(
            "CephCluster.spec.mon.count must be an integer >= 3 "
            f"(found {count!r}). Quorum requires a strict majority; with "
            "<3 mons there is no fault tolerance."
        )
    elif count % 2 == 0:
        errors.append(
            f"CephCluster.spec.mon.count must be odd (found {count}). "
            "Even mon counts waste a node — they tolerate the same number "
            "of failures as count-1 (odd) and double the risk of split "
            "quorum on partitions."
        )

    if mon.get("allowMultiplePerNode") is True:
        errors.append(
            "CephCluster.spec.mon.allowMultiplePerNode must be false; "
            "co-locating mons on the same node defeats quorum redundancy."
        )


def check_mgr(cluster: dict[str, Any], errors: list[str]) -> None:
    spec = cluster.get("spec") or {}
    mgr = spec.get("mgr") or {}

    count = mgr.get("count")
    if not isinstance(count, int) or count < 2:
        errors.append(
            "CephCluster.spec.mgr.count must be an integer >= 2 "
            f"(found {count!r}). A single mgr is a SPOF for the dashboard, "
            "PG balancer, and the rook module."
        )

    if mgr.get("allowMultiplePerNode") is True:
        errors.append(
            "CephCluster.spec.mgr.allowMultiplePerNode must be false; "
            "active and standby mgr must not share a node."
        )


def _has_az_spread(constraints: Any) -> bool:
    if not isinstance(constraints, list):
        return False
    for entry in constraints:
        if not isinstance(entry, dict):
            continue
        key = entry.get("topologyKey")
        if key in ACCEPTABLE_SPREAD_KEYS:
            return True
    return False


def check_placement_spread(
    cluster: dict[str, Any], daemon: str, errors: list[str]
) -> None:
    spec = cluster.get("spec") or {}
    placement = (spec.get("placement") or {}).get(daemon) or {}
    constraints = placement.get("topologySpreadConstraints")
    if not _has_az_spread(constraints):
        errors.append(
            f"CephCluster.spec.placement.{daemon}.topologySpreadConstraints "
            "must declare at least one entry with topologyKey in "
            f"{sorted(ACCEPTABLE_SPREAD_KEYS)} so {daemon} pods spread "
            "across availability zones / racks. Without this, the scheduler "
            f"is free to land every {daemon} in the same zone and a single "
            "zone outage breaks the cluster."
        )


def check_disruption_management(
    cluster: dict[str, Any], errors: list[str]
) -> None:
    spec = cluster.get("spec") or {}
    dm = spec.get("disruptionManagement") or {}
    if dm.get("managePodBudgets") is not True:
        errors.append(
            "CephCluster.spec.disruptionManagement.managePodBudgets must "
            "be true. Without managed PDBs, a `kubectl drain` of two nodes "
            "during an upgrade can evict more mons than quorum tolerates."
        )


def _check_failure_domain(
    kind: str,
    name: str,
    pool_label: str,
    pool: dict[str, Any],
    errors: list[str],
) -> None:
    if not isinstance(pool, dict):
        return
    fd = pool.get("failureDomain")
    if fd is None:
        errors.append(
            f"{kind}/{name}: {pool_label}.failureDomain is missing; "
            f"expected one of {sorted(ACCEPTABLE_FAILURE_DOMAINS)}."
        )
        return
    if fd in ACCEPTABLE_FAILURE_DOMAINS:
        return
    if (
        fd == "host"
        and (kind, name, pool_label) in LEGACY_HOST_FAILURE_DOMAIN_ALLOWLIST
    ):
        return
    errors.append(
        f"{kind}/{name}: {pool_label}.failureDomain={fd!r} is too narrow. "
        f"Use one of {sorted(ACCEPTABLE_FAILURE_DOMAINS)} so a single "
        "zone/rack failure does not take the pool offline. If this is a "
        "documented legacy pool, add it to "
        "LEGACY_HOST_FAILURE_DOMAIN_ALLOWLIST in this script and to the "
        "README."
    )


def check_block_pools(
    pools: list[dict[str, Any]], errors: list[str]
) -> None:
    for pool in pools:
        name = _name(pool)
        spec = pool.get("spec") or {}
        _check_failure_domain("CephBlockPool", name, "spec", spec, errors)


def check_object_stores(
    stores: list[dict[str, Any]], errors: list[str]
) -> None:
    for store in stores:
        name = _name(store)
        spec = store.get("spec") or {}
        if spec.get("zone"):
            # Multisite secondary object stores reference a CephObjectZone.
            # The actual pool topology lives on that zone and is checked
            # separately below.
            continue
        for pool_label in ("metadataPool", "dataPool"):
            pool = spec.get(pool_label)
            if pool is None:
                errors.append(
                    f"CephObjectStore/{name}: spec.{pool_label} is missing."
                )
                continue
            _check_failure_domain(
                "CephObjectStore", name, pool_label, pool, errors
            )


def check_object_zones(
    zones: list[dict[str, Any]], errors: list[str]
) -> None:
    for zone in zones:
        name = _name(zone)
        spec = zone.get("spec") or {}
        for pool_label in ("metadataPool", "dataPool"):
            pool = spec.get(pool_label)
            if pool is None:
                errors.append(
                    f"CephObjectZone/{name}: spec.{pool_label} is missing."
                )
                continue
            _check_failure_domain(
                "CephObjectZone", name, pool_label, pool, errors
            )


def check_filesystems(
    filesystems: list[dict[str, Any]], errors: list[str]
) -> None:
    for fs in filesystems:
        name = _name(fs)
        spec = fs.get("spec") or {}
        meta = spec.get("metadataPool")
        if meta is not None:
            _check_failure_domain(
                "CephFilesystem", name, "metadataPool", meta, errors
            )
        for idx, dp in enumerate(spec.get("dataPools") or []):
            _check_failure_domain(
                "CephFilesystem", name, f"dataPools[{idx}]", dp, errors
            )


def lint(paths: list[Path]) -> list[str]:
    errors: list[str] = []
    all_docs: list[dict[str, Any]] = []
    for path in paths:
        try:
            all_docs.extend(load_documents(path))
        except (OSError, ValueError, yaml.YAMLError) as exc:
            errors.append(f"failed to load {path}: {exc}")

    if errors:
        return errors

    return lint_documents(all_docs)


def lint_documents(all_docs: list[dict[str, Any]]) -> list[str]:
    errors: list[str] = []
    clusters = find_by_kind(all_docs, "CephCluster")
    if not clusters:
        return ["no `kind: CephCluster` resource found in the manifests"]
    if len(clusters) > 1:
        errors.append(
            f"expected exactly one CephCluster, found {len(clusters)}: "
            + ", ".join(_name(c) for c in clusters)
        )

    cluster = clusters[0]
    check_mon(cluster, errors)
    check_mgr(cluster, errors)
    check_placement_spread(cluster, "mon", errors)
    check_placement_spread(cluster, "mgr", errors)
    check_disruption_management(cluster, errors)

    check_block_pools(find_by_kind(all_docs, "CephBlockPool"), errors)
    check_object_stores(find_by_kind(all_docs, "CephObjectStore"), errors)
    check_object_zones(find_by_kind(all_docs, "CephObjectZone"), errors)
    check_filesystems(find_by_kind(all_docs, "CephFilesystem"), errors)

    return errors


def main(argv: list[str]) -> int:
    if len(argv) >= 2:
        paths = [Path(p).resolve() for p in argv[1:]]
        source = ", ".join(str(p) for p in paths)
        missing = [p for p in paths if not p.is_file()]
        if missing:
            for p in missing:
                print(f"error: manifest not found at {p}", file=sys.stderr)
            return 2
        errors = lint(paths)
    else:
        source = f"helm template {repo_root() / DEFAULT_CHART}"
        try:
            errors = lint_documents(render_default_documents())
        except (OSError, ValueError) as exc:
            print(f"error: {exc}", file=sys.stderr)
            return 2

    if errors:
        print(
            f"Rook-Ceph topology contract lint FAILED for {source}:",
            file=sys.stderr,
        )
        for err in errors:
            print(f"  - {err}", file=sys.stderr)
        return 1

    print(f"Rook-Ceph topology contract lint OK: {source}")
    return 0


if __name__ == "__main__":
    sys.exit(main(sys.argv))
