#!/usr/bin/env python3
"""Kafka cluster contract lint.

Validates the rendered Strimzi Kafka manifest from
``infra/helm/infra/kafka-cluster`` against the durability and availability
invariants the data plane depends on. Fails (exit code 1) on any drift. The
intent is to make accidental regressions in PR review impossible: once the
manifest meets the contract, nobody can flip it back without the CI job going
red.

The contract checked here mirrors the four P0 requirements documented in the
ROADMAP:

    1. KRaft mode is enabled and ZooKeeper is gone.
    2. ``min.insync.replicas=2`` (with ``default.replication.factor=3``).
    3. The producer-side defaults the data plane relies on are not silently
       undermined by broker config (``acks=all`` is enforced server-side via
       ``min.insync.replicas`` + ``unclean.leader.election.enable=false``;
       ``enable.idempotence`` is a producer setting, validated separately by
       the unit tests in ``libs/event-bus-data``).
    4. Rack awareness by AZ (``broker.rack`` derived from
       ``topology.kubernetes.io/zone``).

Usage::

    python3 tools/kafka-lint/check_kraft.py
    python3 tools/kafka-lint/check_kraft.py path/to/rendered-cluster.yaml

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


DEFAULT_CHART = Path("infra/helm/infra/kafka-cluster")

# Broker config keys that MUST be present with these exact values.
REQUIRED_BROKER_CONFIG: dict[str, Any] = {
    "min.insync.replicas": 2,
    "default.replication.factor": 3,
    "offsets.topic.replication.factor": 3,
    "transaction.state.log.replication.factor": 3,
    "transaction.state.log.min.isr": 2,
    "unclean.leader.election.enable": False,
    "auto.create.topics.enable": False,
}

# Acceptable values for the rack topology key. We accept either the standard
# Kubernetes well-known label or the legacy ``failure-domain`` form, but it
# MUST be an AZ-level key — node-level rack awareness defeats the purpose.
ACCEPTABLE_RACK_KEYS = frozenset(
    {
        "topology.kubernetes.io/zone",
        "failure-domain.beta.kubernetes.io/zone",
    }
)

EXPECTED_REPLICA_SELECTOR = (
    "org.apache.kafka.common.replica.RackAwareReplicaSelector"
)


def repo_root() -> Path:
    # tools/kafka-lint/check_kraft.py -> repo root is two levels up.
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
        ["helm", "template", "kafka-cluster", str(chart)],
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


def find_kafka(docs: list[dict[str, Any]]) -> dict[str, Any]:
    for doc in docs:
        if doc.get("kind") == "Kafka":
            return doc
    raise ValueError("no `kind: Kafka` resource found in manifest")


def find_node_pools(docs: list[dict[str, Any]]) -> list[dict[str, Any]]:
    return [d for d in docs if d.get("kind") == "KafkaNodePool"]


def check_kraft(kafka: dict[str, Any], errors: list[str]) -> None:
    annotations = (kafka.get("metadata") or {}).get("annotations") or {}
    kraft = annotations.get("strimzi.io/kraft")
    if kraft != "enabled":
        errors.append(
            "Kafka.metadata.annotations['strimzi.io/kraft'] must be "
            f"'enabled' (found {kraft!r}). KRaft is required; ZooKeeper "
            "is forbidden."
        )
    node_pools_anno = annotations.get("strimzi.io/node-pools")
    if node_pools_anno != "enabled":
        errors.append(
            "Kafka.metadata.annotations['strimzi.io/node-pools'] must be "
            f"'enabled' (found {node_pools_anno!r}). Node pools are how "
            "controller/broker roles are declared in KRaft mode."
        )


def check_no_zookeeper(kafka: dict[str, Any], errors: list[str]) -> None:
    spec = kafka.get("spec") or {}
    if "zookeeper" in spec:
        errors.append(
            "Kafka.spec.zookeeper is present. ZooKeeper must be removed "
            "entirely; KRaft is the only supported mode."
        )


def check_node_pools(
    pools: list[dict[str, Any]], errors: list[str]
) -> None:
    if not pools:
        errors.append(
            "no KafkaNodePool resources found. KRaft mode requires at "
            "least one node pool with the 'controller' role."
        )
        return

    has_controller = False
    for pool in pools:
        name = (pool.get("metadata") or {}).get("name", "<unnamed>")
        roles = ((pool.get("spec") or {}).get("roles")) or []
        if not isinstance(roles, list):
            errors.append(
                f"KafkaNodePool/{name}: spec.roles must be a list, got "
                f"{type(roles).__name__}"
            )
            continue
        if "controller" in roles:
            has_controller = True
        for role in roles:
            if role not in ("controller", "broker"):
                errors.append(
                    f"KafkaNodePool/{name}: unknown role {role!r}; "
                    "expected 'controller' and/or 'broker'"
                )

    if not has_controller:
        errors.append(
            "no KafkaNodePool declares the 'controller' role. KRaft mode "
            "requires a quorum of controller nodes."
        )


def check_broker_config(kafka: dict[str, Any], errors: list[str]) -> None:
    config = ((kafka.get("spec") or {}).get("kafka") or {}).get("config") or {}
    for key, expected in REQUIRED_BROKER_CONFIG.items():
        if key not in config:
            errors.append(
                f"Kafka.spec.kafka.config['{key}'] is missing; expected "
                f"{expected!r}."
            )
            continue
        actual = config[key]
        if actual != expected:
            errors.append(
                f"Kafka.spec.kafka.config['{key}'] must be {expected!r}, "
                f"found {actual!r}."
            )

    selector = config.get("replica.selector.class")
    if selector != EXPECTED_REPLICA_SELECTOR:
        errors.append(
            "Kafka.spec.kafka.config['replica.selector.class'] must be "
            f"{EXPECTED_REPLICA_SELECTOR!r} so consumers can fetch from the "
            f"closest in-AZ replica (found {selector!r})."
        )


def check_rack(kafka: dict[str, Any], errors: list[str]) -> None:
    rack = ((kafka.get("spec") or {}).get("kafka") or {}).get("rack") or {}
    key = rack.get("topologyKey")
    if not key:
        errors.append(
            "Kafka.spec.kafka.rack.topologyKey is missing. Rack awareness "
            "by AZ is required so partition replicas survive a zone outage."
        )
        return
    if key not in ACCEPTABLE_RACK_KEYS:
        errors.append(
            f"Kafka.spec.kafka.rack.topologyKey must be one of "
            f"{sorted(ACCEPTABLE_RACK_KEYS)} (AZ-level), found {key!r}."
        )


def check_replicas(kafka: dict[str, Any], errors: list[str]) -> None:
    # Belt-and-braces: the manifest also pins `spec.kafka.replicas: 3` for
    # CRD-schema reasons even when node pools own the topology. If somebody
    # lowers it to <3 it's a strong signal the durability contract is being
    # broken intentionally.
    replicas = ((kafka.get("spec") or {}).get("kafka") or {}).get("replicas")
    if replicas is None:
        return  # not required by Strimzi when node pools are enabled
    if not isinstance(replicas, int) or replicas < 3:
        errors.append(
            "Kafka.spec.kafka.replicas must be >= 3 to satisfy "
            f"min.insync.replicas=2 + replication.factor=3 (found {replicas!r})."
        )


def lint(path: Path) -> list[str]:
    try:
        docs = load_documents(path)
    except (OSError, ValueError, yaml.YAMLError) as exc:
        return [f"failed to load {path}: {exc}"]
    return lint_documents(docs)


def lint_documents(docs: list[dict[str, Any]]) -> list[str]:
    errors: list[str] = []
    try:
        kafka = find_kafka(docs)
    except ValueError as exc:
        return [str(exc)]

    pools = find_node_pools(docs)

    check_kraft(kafka, errors)
    check_no_zookeeper(kafka, errors)
    check_node_pools(pools, errors)
    check_broker_config(kafka, errors)
    check_rack(kafka, errors)
    check_replicas(kafka, errors)

    return errors


def main(argv: list[str]) -> int:
    if len(argv) > 2:
        print(
            f"usage: {argv[0]} [path/to/kafka-cluster.yaml]", file=sys.stderr
        )
        return 2

    if len(argv) == 2:
        path = Path(argv[1]).resolve()
        source = str(path)
        if not path.is_file():
            print(f"error: manifest not found at {path}", file=sys.stderr)
            return 2
        errors = lint(path)
    else:
        source = f"helm template {repo_root() / DEFAULT_CHART}"
        try:
            errors = lint_documents(render_default_documents())
        except (OSError, ValueError) as exc:
            print(f"error: {exc}", file=sys.stderr)
            return 2

    if errors:
        print(f"Kafka KRaft contract lint FAILED for {source}:", file=sys.stderr)
        for err in errors:
            print(f"  - {err}", file=sys.stderr)
        return 1

    print(f"Kafka KRaft contract lint OK: {source}")
    return 0


if __name__ == "__main__":
    sys.exit(main(sys.argv))
