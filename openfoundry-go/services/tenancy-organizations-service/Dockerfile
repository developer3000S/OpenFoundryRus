# syntax=docker/dockerfile:1.7
FROM golang:1.23 AS builder
WORKDIR /workspace
COPY go.mod go.sum ./
RUN --mount=type=cache,target=/go/pkg/mod \
    go mod download
COPY . .
ARG VERSION=dev
RUN --mount=type=cache,target=/root/.cache/go-build \
    --mount=type=cache,target=/go/pkg/mod \
    CGO_ENABLED=0 GOOS=linux \
    go build -trimpath \
      -ldflags "-s -w -X main.version=${VERSION}" \
      -o /out/tenancy-organizations-service \
      ./services/tenancy-organizations-service/cmd/tenancy-organizations-service

FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=builder /out/tenancy-organizations-service /usr/local/bin/service
USER nonroot:nonroot
EXPOSE 50113
ENTRYPOINT ["/usr/local/bin/service"]
