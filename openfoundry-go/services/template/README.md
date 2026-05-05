# Service template

Reference layout for every Go service in `openfoundry-go/services/`.
Copy the directory to `services/<your-service>/`, replace
`{{SERVICE_NAME}}` everywhere, register the service in
`Makefile` (already automatic via `wildcard`) and you have a buildable
binary that exposes:

- `GET /healthz` — liveness payload (matches Rust `core_models::health`)
- `GET /metrics` — Prometheus scrape endpoint
- bearer-token authentication via `auth-middleware` on the `/api/*` mount

## Layout

```
services/<name>/
├── cmd/<name>/main.go         # entrypoint
├── internal/
│   ├── config/config.go        # koanf-backed config
│   ├── server/server.go        # router + server lifecycle
│   ├── handler/                # HTTP handlers
│   ├── service/                # business logic
│   └── repo/                   # data access (sqlc-generated)
├── api/openapi.yaml            # OpenAPI contract (optional)
├── config.yaml                 # default config baked into the image
├── Dockerfile
└── README.md
```

## Build

```sh
go build -o bin/<name> ./services/<name>/cmd/<name>
```

Or via the root Makefile:

```sh
make build-services
```

## Image

```sh
docker build -t openfoundry/<name>:dev -f services/<name>/Dockerfile .
```

The Dockerfile is a multi-stage build that produces a distroless static
image (~25 MB) — no shell, no package manager, runs as nonroot.
