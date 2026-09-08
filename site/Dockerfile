# syntax=docker/dockerfile:1.7
#
# Builds the Astro public marketing site (site/) and serves the static
# output through nginx-unprivileged on port 8080 (D-K): the official nginx
# image starts as root and fails PRD §10.1's non-root container gate, so
# Nginx Proxy Manager must target 8080, not 80.
#
# m0-foundation's tasks.md has no dedicated phase for the site's real
# content; `pnpm --filter site build` writes a placeholder dist/index.html
# for now so this Dockerfile is buildable and verifiable today (task 8.3).
#
# Build (from the repository root):
#   docker build -f site/Dockerfile -t vecingest-site .

FROM node:22-slim AS deps
RUN corepack enable && corepack prepare pnpm@10.33.0 --activate
WORKDIR /workspace
COPY pnpm-workspace.yaml turbo.json package.json pnpm-lock.yaml ./
COPY site/package.json ./site/package.json
COPY app/package.json ./app/package.json
COPY packages/shared/package.json ./packages/shared/package.json
RUN pnpm install --frozen-lockfile

FROM deps AS build
COPY site ./site
RUN pnpm --filter site build

FROM nginxinc/nginx-unprivileged:1.27-alpine
COPY --from=build /workspace/site/dist /usr/share/nginx/html
COPY site/nginx.conf /etc/nginx/conf.d/default.conf
USER 1000:1000
EXPOSE 8080
HEALTHCHECK --interval=30s --timeout=5s --retries=3 \
    CMD ["wget", "-qO-", "http://127.0.0.1:8080/"]
