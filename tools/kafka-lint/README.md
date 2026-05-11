# `tools/kafka-lint`

Static contract lint for the rendered Strimzi Kafka cluster manifest used by
the data plane. The live source is the Helm chart under
`infra/helm/infra/kafka-cluster/`; the linter renders that chart before
checking the contract.

## Why

Kafka's durability and availability properties are entirely controlled by a
small handful of broker configuration knobs. Lowering any of them — even
accidentally, in an unrelated PR — silently turns the data bus into a
lossy system. This linter encodes the contract so a regression cannot reach
`main`:

- KRaft mode is on (`strimzi.io/kraft: enabled`).
- ZooKeeper has been removed (`spec.zookeeper` absent, no SPOF).
- A `KafkaNodePool` declares the `controller` role.
- `min.insync.replicas=2`, `default.replication.factor=3`,
  `unclean.leader.election.enable=false`, `auto.create.topics.enable=false`,
  `transaction.state.log.{replication.factor=3,min.isr=2}`,
  `offsets.topic.replication.factor=3`.
- Rack-awareness uses an AZ-level Kubernetes topology key
  (`topology.kubernetes.io/zone`) and the consumer-side rack-aware replica
  selector is configured.

Producer-side guarantees (`acks=all`, `enable.idempotence=true`) are enforced
in the shared `event-bus-data` library and covered by its unit tests; this
linter focuses on what the broker manifest controls.

## Run locally

```
pip install pyyaml
python3 tools/kafka-lint/check_kraft.py
```

With no path argument, the script runs `helm template kafka-cluster
infra/helm/infra/kafka-cluster` internally. You can also pass an already
rendered manifest path:

```
helm template kafka-cluster infra/helm/infra/kafka-cluster > /tmp/kafka-cluster.yaml
python3 tools/kafka-lint/check_kraft.py /tmp/kafka-cluster.yaml
```

The script exits 0 on success and 1 on any contract violation, printing each
violated invariant.

## CI

Runs on every push and pull request that touches `infra/helm/infra/kafka-cluster/**` or
this tool — see `.github/workflows/kafka-lint.yml`.
