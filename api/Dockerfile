# syntax=docker/dockerfile:1.7
#
# Multi-stage build for the vecingest API/worker binary (D-K).
#
# Builder: golang:1.27-bookworm, CGO_ENABLED=0, statically linked, single
# linux/amd64 platform (the phase-A server is amd64; arm64 is a later,
# cost-free addition per D-K).
#
# Final stage (default build target): gcr.io/distroless/static-debian12:nonroot
# — no shell, no package manager, no `wget`/`curl`. Runs as the image's
# built-in "nonroot" account, USER 65532:65532. Migrations are
# //go:embed-ed and run from the binary itself (no shell needed), and the
# healthcheck below is the binary probing itself:
# ["CMD","/vecingest","health","--ready"], exactly as PRD §8.1 specifies.
#
# `time/tzdata` is imported by cmd/vecingest/main.go so the IANA zone
# database travels inside the binary regardless of base image (PRD §9);
# distroless/static already ships tzdata, but the embed removes that
# dependency entirely (design.md cross-cutting rule 3).
#
# Build (from the repository root):
#   docker build -f api/Dockerfile -t vecingest-api .
#
# Focused verification of the tzdata embed (task 8.4), never the default
# target:
#   docker build --target tzcheck -f api/Dockerfile -t vecingest-tzcheck .
#   docker run --rm vecingest-tzcheck

FROM golang:1.27-bookworm AS builder
WORKDIR /src/api
ENV CGO_ENABLED=0 GOOS=linux GOARCH=amd64

COPY api/go.mod api/go.sum ./
RUN --mount=type=cache,target=/go/pkg/mod \
    go mod download

COPY api/ ./

ARG VERSION=dev

RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    go build -trimpath -ldflags "-s -w -X main.version=${VERSION}" -o /out/vecingest ./cmd/vecingest

# tzcheck is a build-only probe binary (task 8.4/8.5) — never part of the
# default (last) build stage below, and never shipped in the production
# image unless a caller explicitly requests --target tzcheck.
RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    go build -trimpath -o /out/tzcheck ./cmd/tzcheck

FROM gcr.io/distroless/static-debian12:nonroot AS tzcheck
COPY --from=builder /out/tzcheck /tzcheck
USER 65532:65532
ENTRYPOINT ["/tzcheck"]

# Default target: the production vecingest image.
FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=builder /out/vecingest /vecingest
USER 65532:65532
HEALTHCHECK --interval=30s --timeout=5s --retries=3 --start-period=10s \
    CMD ["/vecingest", "health", "--ready"]
ENTRYPOINT ["/vecingest"]
