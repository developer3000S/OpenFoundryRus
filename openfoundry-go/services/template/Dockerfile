# syntax=docker/dockerfile:1.7
#
# Multi-stage build for OpenFoundry Go services.
# Final image is distroless static (~25 MB), runs as nonroot, no shell.

FROM golang:1.23 AS builder
WORKDIR /workspace

# Pre-warm the module cache. Build context is the openfoundry-go root,
# so go.mod / go.sum live next to this Dockerfile when invoked from
# `docker build -f services/<name>/Dockerfile .`.
COPY go.mod go.sum ./
RUN --mount=type=cache,target=/go/pkg/mod \
    go mod download

COPY . .

ARG SERVICE_NAME=template
ARG VERSION=dev

RUN --mount=type=cache,target=/root/.cache/go-build \
    --mount=type=cache,target=/go/pkg/mod \
    CGO_ENABLED=0 GOOS=linux \
    go build -trimpath \
      -ldflags "-s -w -X main.version=${VERSION}" \
      -o /out/${SERVICE_NAME} ./services/${SERVICE_NAME}/cmd/${SERVICE_NAME}

FROM gcr.io/distroless/static-debian12:nonroot
ARG SERVICE_NAME=template
COPY --from=builder /out/${SERVICE_NAME} /usr/local/bin/service
COPY --from=builder /workspace/services/${SERVICE_NAME}/config.yaml /etc/openfoundry/config.yaml
USER nonroot:nonroot
EXPOSE 8080
ENTRYPOINT ["/usr/local/bin/service", "-config", "/etc/openfoundry/config.yaml"]
