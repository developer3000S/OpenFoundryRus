# syntax=docker/dockerfile:1.10
#
# OpenFoundry Web (React + Vite) — multi-stage SPA bundle behind nginx.
#
# Cache strategy
#   1. Lockfile-only stage isolates `pnpm install` so node_modules is
#      only refetched when package.json / pnpm-lock.yaml change.
#   2. pnpm content-addressable store mounted as a BuildKit cache
#      (sharing=locked) — shared across all CI runs and local builds.
#   3. App source COPY-ed with `--link` so source-only changes don't
#      bust the deps stage; Vite's own cache lives in a sticky cache
#      mount for incremental rebuilds.
#
# Final image
#   * Base: nginx:1.27-alpine (~25 MB) running as the unprivileged
#     `nginx` user. No shell access to the SPA bundle from outside,
#     STOPSIGNAL SIGQUIT for nginx graceful shutdown.
#   * Asset bundle is delivered immutable + long-cache via the existing
#     apps/web/nginx.conf (assets/* gets `Cache-Control: immutable`).

ARG NODE_VERSION=24.15.0
ARG ALPINE_VERSION=3.22
ARG NGINX_VERSION=1.27
ARG PNPM_VERSION=9.15.4

############################################################
# Stage 1 — base: pnpm + corepack on Node Alpine.
############################################################
FROM --platform=$BUILDPLATFORM node:${NODE_VERSION}-alpine${ALPINE_VERSION} AS base

ARG PNPM_VERSION
ENV PNPM_HOME=/pnpm \
    PATH=/pnpm:$PATH \
    COREPACK_DEFAULT_TO_LATEST=0 \
    NODE_ENV=production

RUN --mount=type=cache,id=apk-web-base,target=/var/cache/apk,sharing=locked \
    apk add --no-cache libc6-compat tini && \
    corepack enable && \
    corepack prepare pnpm@${PNPM_VERSION} --activate && \
    pnpm config set store-dir /pnpm/store && \
    pnpm config set fund false && \
    pnpm config set update-notifier false

############################################################
# Stage 2 — deps: pnpm install (lockfile-driven, fully cached).
############################################################
FROM base AS deps
WORKDIR /workspace

COPY --link package.json pnpm-lock.yaml pnpm-workspace.yaml ./
COPY --link apps/web/package.json apps/web/package.json

RUN --mount=type=cache,id=of-pnpm-store,target=/pnpm/store,sharing=locked \
    NODE_ENV=development \
    pnpm install --frozen-lockfile --prefer-offline

############################################################
# Stage 3 — build: compile React + Vite assets.
############################################################
FROM deps AS build

ARG VERSION=dev
ENV NODE_ENV=production \
    VITE_APP_VERSION=$VERSION

COPY --link apps/web/ apps/web/

RUN --mount=type=cache,id=of-pnpm-store,target=/pnpm/store,sharing=locked \
    --mount=type=cache,id=of-vite-cache,target=/workspace/apps/web/node_modules/.vite,sharing=locked \
    pnpm --filter @open-foundry/web build

############################################################
# Stage 4 — runtime: nginx-alpine, hardened, unprivileged.
############################################################
FROM nginx:${NGINX_VERSION}-alpine AS runtime

ARG VERSION=dev
ARG VCS_REF=unknown
ARG BUILD_DATE=unknown

LABEL org.opencontainers.image.title="openfoundry-web" \
      org.opencontainers.image.description="OpenFoundry web SPA (React + Vite) served by nginx" \
      org.opencontainers.image.version="${VERSION}" \
      org.opencontainers.image.revision="${VCS_REF}" \
      org.opencontainers.image.created="${BUILD_DATE}" \
      org.opencontainers.image.source="https://github.com/openfoundry/openfoundry-go" \
      org.opencontainers.image.vendor="OpenFoundry" \
      org.opencontainers.image.licenses="Apache-2.0" \
      org.opencontainers.image.base.name="docker.io/library/nginx:1.27-alpine"

# Drop the default nginx site + replace with the SPA-aware one. Tighten
# permissions on the runtime dirs so nginx can run as `nginx` (uid 101)
# without root.
RUN set -eux; \
    rm -f /etc/nginx/conf.d/default.conf; \
    install -d -o nginx -g nginx /var/cache/nginx /var/log/nginx; \
    install -m 0644 /dev/null /var/run/nginx.pid; \
    chown nginx:nginx /var/run/nginx.pid

COPY --link apps/web/nginx.conf /etc/nginx/conf.d/default.conf
COPY --from=build --chown=nginx:nginx /workspace/apps/web/dist /usr/share/nginx/html

USER nginx
STOPSIGNAL SIGQUIT
EXPOSE 3000

HEALTHCHECK --interval=30s --timeout=3s --start-period=5s --retries=3 \
    CMD wget -qO- http://127.0.0.1:3000/ >/dev/null || exit 1

CMD ["nginx", "-g", "daemon off;"]
