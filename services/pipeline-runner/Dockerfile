# syntax=docker/dockerfile:1.10
#
# pipeline-runner — Go orchestrator that shells out to spark-submit.
#
# Unlike the rest of the OpenFoundry fleet this image is NOT distroless:
# the orchestrator embeds the Spark runtime so the resulting pod can drive
# `/opt/spark/bin/spark-submit` directly. Final size = Spark base
# (~600 MB) + Iceberg JARs + ~15 MB Go binary.
#
# Cache strategy is identical to the rest of the monorepo:
#   1. Module download isolated in a stage that only re-runs on go.mod /
#      go.sum changes.
#   2. Source bind-mounted (no fat COPY layer).
#   3. Shared BuildKit caches for /go/pkg/mod and /root/.cache/go-build.
#   4. Iceberg JAR downloads live in their own stage so a Go-only change
#      never re-fetches them from Maven Central.
#
# Build context MUST be the openfoundry-go monorepo root.

ARG GO_VERSION=1.26.3
ARG ALPINE_VERSION=3.22
ARG SPARK_IMAGE=apache/spark:3.5.4-scala2.12-java17-python3-ubuntu
ARG ICEBERG_VERSION=1.5.2
ARG SBT_IMAGE=sbtscala/scala-sbt:eclipse-temurin-21.0.8_9_1.12.11_2.12.21

############################################################
# Stage 1 — modules: pre-warmed Go module graph.
############################################################
FROM --platform=$BUILDPLATFORM golang:${GO_VERSION}-alpine${ALPINE_VERSION} AS modules

ENV GOTOOLCHAIN=local \
    CGO_ENABLED=0 \
    GOFLAGS="-mod=readonly -buildvcs=false" \
    GOPROXY=https://proxy.golang.org,direct \
    GOSUMDB=sum.golang.org

WORKDIR /workspace
COPY --link go.mod go.sum ./
RUN --mount=type=cache,id=of-go-mod,target=/go/pkg/mod,sharing=locked \
    go mod download -x

############################################################
# Stage 2 — builder: compiles the pipeline-runner binary.
############################################################
FROM modules AS builder

ARG VERSION=dev
ARG TARGETOS=linux
ARG TARGETARCH=amd64

RUN --mount=type=bind,source=.,target=/workspace,rw \
    --mount=type=cache,id=of-go-mod,target=/go/pkg/mod,sharing=locked \
    --mount=type=cache,id=of-go-build,target=/root/.cache/go-build,sharing=locked \
    set -eux; \
    GOOS=${TARGETOS} GOARCH=${TARGETARCH} \
        go build -trimpath -buildvcs=false \
            -ldflags "-s -w -buildid= -extldflags=-static -X main.version=${VERSION}" \
            -tags "osusergo,netgo,static_build" \
            -o /out/pipeline-runner \
            ./services/pipeline-runner/cmd/pipeline-runner

############################################################
# Stage 3 — jars: download Iceberg runtime JARs in an isolated
#           stage so they cache independently from the Go build.
############################################################
FROM --platform=$BUILDPLATFORM alpine:${ALPINE_VERSION} AS jars
ARG ICEBERG_VERSION
RUN --mount=type=cache,id=apk-jars,target=/var/cache/apk,sharing=locked \
    apk add --no-cache curl ca-certificates && \
    mkdir -p /jars && \
    curl -fsSL --retry 3 -o /jars/iceberg-spark-runtime-3.5_2.12-${ICEBERG_VERSION}.jar \
        "https://repo1.maven.org/maven2/org/apache/iceberg/iceberg-spark-runtime-3.5_2.12/${ICEBERG_VERSION}/iceberg-spark-runtime-3.5_2.12-${ICEBERG_VERSION}.jar" && \
    curl -fsSL --retry 3 -o /jars/iceberg-aws-bundle-${ICEBERG_VERSION}.jar \
        "https://repo1.maven.org/maven2/org/apache/iceberg/iceberg-aws-bundle/${ICEBERG_VERSION}/iceberg-aws-bundle-${ICEBERG_VERSION}.jar" && \
    chmod 0644 /jars/*.jar

############################################################
# Stage 3.5 — scala-jar: builds the OpenFoundry Spark transform
#             entrypoint JAR (services/pipeline-runner-spark) so the
#             runtime stage can drop it into /opt/spark/jars/.
############################################################
FROM ${SBT_IMAGE} AS scala-jar
ARG VERSION
WORKDIR /workspace

COPY services/pipeline-runner-spark/build.sbt          ./build.sbt
COPY services/pipeline-runner-spark/project/           ./project/
RUN --mount=type=cache,id=sbt-ivy,target=/root/.ivy2,sharing=locked \
    --mount=type=cache,id=sbt-cache,target=/root/.sbt,sharing=locked \
    --mount=type=cache,id=sbt-coursier,target=/root/.cache/coursier,sharing=locked \
    sbt -no-colors update

COPY services/pipeline-runner-spark/src/ ./src/

ENV VERSION=${VERSION}
RUN --mount=type=cache,id=sbt-ivy,target=/root/.ivy2,sharing=locked \
    --mount=type=cache,id=sbt-cache,target=/root/.sbt,sharing=locked \
    --mount=type=cache,id=sbt-coursier,target=/root/.cache/coursier,sharing=locked \
    sbt -no-colors "set assembly / assemblyJarName := \"pipeline-runner-spark.jar\"" assembly && \
    cp target/scala-2.12/pipeline-runner-spark.jar /workspace/pipeline-runner-spark.jar

############################################################
# Stage 4 — runtime: Apache Spark + Iceberg + Go orchestrator.
############################################################
FROM ${SPARK_IMAGE} AS runtime

ARG VERSION=dev
ARG VCS_REF=unknown
ARG BUILD_DATE=unknown
ARG ICEBERG_VERSION

LABEL org.opencontainers.image.title="openfoundry-pipeline-runner" \
      org.opencontainers.image.description="OpenFoundry pipeline orchestrator (spark-submit shim)" \
      org.opencontainers.image.version="${VERSION}" \
      org.opencontainers.image.revision="${VCS_REF}" \
      org.opencontainers.image.created="${BUILD_DATE}" \
      org.opencontainers.image.source="https://github.com/openfoundry/openfoundry-go" \
      org.opencontainers.image.vendor="OpenFoundry" \
      org.opencontainers.image.licenses="Apache-2.0" \
      org.opencontainers.image.base.name="apache/spark:3.5.4-scala2.12-java17-python3-ubuntu"

COPY --from=jars      /jars/                                /opt/spark/jars/
COPY --from=scala-jar /workspace/pipeline-runner-spark.jar  /opt/spark/jars/pipeline-runner-spark.jar
COPY --from=builder   /out/pipeline-runner                  /usr/local/bin/pipeline-runner

# Keep the apache/spark base entrypoint (`/opt/entrypoint.sh`) so the Spark
# Operator can drive the image as a SparkApplication driver/executor pod.
# The Go orchestrator (pipeline-runner) stays available at /usr/local/bin/
# for external callers that prefer the spark-submit-shim flow; explicit
# invocation is required (it is NOT the default ENTRYPOINT any more).
USER 185:185
STOPSIGNAL SIGTERM
