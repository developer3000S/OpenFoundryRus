# syntax=docker/dockerfile:1.7
FROM node:24.15.0-alpine3.23 AS base
ENV PNPM_HOME=/pnpm
ENV PATH=$PNPM_HOME:$PATH
RUN corepack enable && pnpm config set store-dir /pnpm/store

FROM base AS deps
WORKDIR /workspace

COPY package.json pnpm-lock.yaml pnpm-workspace.yaml ./
COPY apps/web/package.json apps/web/package.json
RUN --mount=type=cache,id=openfoundry-pnpm-store,target=/pnpm/store,sharing=locked \
	pnpm install --frozen-lockfile

FROM deps AS build
COPY . .
RUN --mount=type=cache,id=openfoundry-pnpm-store,target=/pnpm/store,sharing=locked \
	pnpm --filter @open-foundry/web build

FROM nginx:1.27-alpine AS runtime
COPY --from=build /workspace/apps/web/dist /usr/share/nginx/html
COPY apps/web/nginx.conf /etc/nginx/conf.d/default.conf
EXPOSE 3000
CMD ["nginx", "-g", "daemon off;"]
