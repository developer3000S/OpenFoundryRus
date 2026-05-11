# OpenFoundry — Smoke & Chaos

Esta carpeta agrupa los **scenarios de smoke** del data plane y la **suite
de chaos** que valida sus propiedades no-SPOF.

## Estructura

```
smoke/
├── scenarios/              # JSON consumidos por `of-cli smoke run`
│   ├── p0-critical-path.json
│   ├── p2-runtime-critical-path.json
│   ├── p3-semantic-governance-critical-path.json
│   ├── p4-developer-platform-critical-path.json
│   ├── p5-ai-ml-critical-path.json
│   └── p6-analytics-enterprise-critical-path.json
├── results/                # Salida (uno por scenario, sobreescrita)
│   └── chaos/              # Salida de la suite de chaos (chaos__scenario.json)
├── fixtures/
└── chaos/                  # Suite de chaos del data plane
    ├── lib/common.sh
    ├── kill-one-mon.sh                 # Rook-Ceph mon
    ├── kill-one-kafka-broker.sh        # Strimzi Kafka
    ├── kill-one-nats-node.sh           # NATS
    ├── kill-pg-primary.sh              # CNPG failover
    └── run.sh                          # Orquestador
```

## Ejecutar un scenario individual

El runner actual es el CLI Go en `tools/of-cli`. Desde la raíz del repo:

```bash
go run ./tools/of-cli -- smoke run \
  --scenario smoke/scenarios/p2-runtime-critical-path.json \
  --output   smoke/results/p2-runtime-critical-path.json
```

Para acelerar ejecuciones repetidas, compila el binario una vez:

```bash
mkdir -p bin
go build -trimpath -o bin/of ./tools/of-cli
./bin/of smoke run \
  --scenario smoke/scenarios/<file>.json \
  --output   smoke/results/<file>.json
```

El `justfile` de la raíz es solo un shim sobre `make`; no contiene recipes
`just smoke-*` actuales. Si una página las menciona, trátala como stale.

## Suite de chaos

La suite valida las propiedades no-SPOF del data plane: por cada capa
(Ceph mon, Kafka, NATS, Postgres primary) mata 1 pod, espera a que el
cluster vuelva a verde, y luego
ejecuta los scenarios `p2..p6`. Falla si **cualquier** scenario falla
bajo **cualquier** chaos.

### CI

Está atada a `.github/workflows/chaos-smoke.yml`, que se ejecuta:

- En `workflow_dispatch` (manual; opcionalmente con el secret de
  repo/org `CHAOS_KUBECONFIG` para inyectar el kubeconfig del cluster
  destino — si no está, se asume que el runner ya lo tiene configurado).
- Nightly (`cron: "17 4 * * *"`).

**No** corre en cada PR (es cara).

### Local — kind

```bash
# 1. Cluster local
kind create cluster --name openfoundry-chaos

# 2. Instala los operadores y CRs del DP que se vayan a probar.
#    Mínimo:
#      - Strimzi  + Kafka  (infra/k8s/platform/manifests/strimzi/)
#      - Rook     + Ceph   (infra/k8s/platform/manifests/rook/)
#      - CloudNativePG + Cluster (infra/k8s/platform/manifests/cnpg/)
#      - NATS Helm chart en ns `nats`
#    Ver READMEs en cada subcarpeta de infra/k8s/.

# 3. Lanza el edge gateway / servicios del CP en otra terminal (o port-forwards
#    contra el cluster) de forma que `http://127.0.0.1:8080` sirva el
#    `edge-gateway-service` esperado por los scenarios (ver smoke/scenarios/*.json).

# 4. Compila el CLI una vez.
mkdir -p bin
go build -trimpath -o bin/of ./tools/of-cli
export OF_CLI="$PWD/bin/of"

# 5. Corre la suite completa.
./smoke/chaos/run.sh
```

### Local — k3d

```bash
k3d cluster create openfoundry-chaos --agents 3
# resto idéntico al flujo de kind.
```

### Variables de entorno útiles

| Variable                  | Default                | Descripción                                                    |
|---------------------------|------------------------|----------------------------------------------------------------|
| `OF_CLI`                  | `go run ./tools/of-cli --` | Cómo invocar el runner. Pon una ruta a binario para acelerar.  |
| `CHAOS_RESULTS_DIR`       | `smoke/results/chaos`  | Dónde escribir los JSON de salida de cada combinación.         |
| `CHAOS_WAIT_TIMEOUT`      | `600s`                 | Timeout máximo de `kubectl wait` tras matar un pod.            |
| `CHAOS_DRY_RUN`           | `0`                    | `1` ⇒ no toca el cluster (para validar la lógica del script).  |
| `ROOK_CEPH_NAMESPACE`     | `rook-ceph`            | NS del CephCluster.                                            |
| `KAFKA_NAMESPACE`         | `kafka`                | NS del Kafka Strimzi.                                          |
| `KAFKA_CLUSTER`           | `openfoundry`          | Nombre del CR `Kafka`.                                         |
| `KAFKA_POOL`              | `kafka`                | Nombre del `KafkaNodePool`.                                    |
| `NATS_NAMESPACE`          | `nats`                 | NS del cluster NATS.                                           |
| `NATS_SELECTOR`           | `app.kubernetes.io/name=nats` | Selector de pods NATS.                                  |
| `PG_NAMESPACE`            | `default`              | NS del CNPG `Cluster`.                                         |
| `PG_CLUSTER`              | `openfoundry-pg`       | Nombre del `Cluster` CNPG.                                     |

### Lanzar un único experimento

Cualquier `kill-*.sh` se puede ejecutar de forma aislada:

```bash
./smoke/chaos/kill-one-kafka-broker.sh
```

…y luego ejecuta un scenario con `go run ./tools/of-cli -- smoke run
--scenario <file> --output <file>` para validar manualmente.

## Validación de los scripts

```bash
shellcheck smoke/chaos/*.sh smoke/chaos/lib/*.sh
# Si tienes actionlint instalado:
actionlint .github/workflows/chaos-smoke.yml
```
