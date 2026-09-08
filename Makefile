.PHONY: dev gen test test-short test-golden-update test-e2e lint lint-scope lint-compose

# Single workspace entrypoint (PRD §9, §7.7). Every CI job and every
# developer command routes through one of these targets.

dev:
	@echo "dev: not yet implemented (wired incrementally as api/ and app/ gain runnable entrypoints)"

## gen: regenerates sqlc code, api/openapi/openapi.yaml, the TS client and
## the Zod schemas. CI fails if `git diff` is dirty after this target runs.
gen:
	cd api && go run ./cmd/openapi-gen
	cd api && go tool sqlc generate
	pnpm --filter @vecingest/shared build

## test: full Go + TS suite (unit + integration, requires Docker for
## Testcontainers-backed tests).
test:
	cd api && go test -race ./...
	pnpm --filter app test

## test-short: the Docker-free fast suite. Every test that acquires a
## container or execs a built binary MUST carry a testing.Short() skip
## guard so this target never depends on a running Docker daemon.
test-short:
	cd api && go test -race -short ./...

## test-golden-update: the only sanctioned path to regenerate golden files
## (e.g. api/internal/domain/audit/testdata/*.golden). Always inspect the
## diff and re-run `make test` without -update afterward.
test-golden-update:
	cd api && go test -race ./internal/domain/audit/... -update

## test-e2e: full integration suite against a real Postgres 17 via
## Testcontainers.
test-e2e:
	cd api && go test -race -count=1 ./test/...   # whole integration package; do NOT add a -run filter, a stale one silently runs zero tests

lint:
	cd api && gofumpt -l -d .
	# No `2>/dev/null ||` fallback here: run from api/ or not at all. Running
	# golangci-lint from the repo root reports "0 issues" after a typechecking
	# error, because go.work makes the root contain no module — a silent false green.
	cd api && golangci-lint run ./...
	pnpm run lint
	$(MAKE) lint-compose

## lint-scope: fails if any sqlc query against a scoped table omits its
## tenant column (community_id/office_id/company_id) filter.
lint-scope:
	cd api && go run ./cmd/lintscope ./internal/db/queries

## lint-compose: fails if any docker-compose.yml service is missing
## a required hardening key (read_only, cap_drop, no-new-privileges,
## tmpfs, non-root user, mem_limit, restart, networks) -- Compose has no
## admission controller, so a missing key is otherwise a silent gap.
## deploy/docker-compose.dev.yml is a local-only single-service (db)
## convenience file and is intentionally not linted here.
lint-compose:
	cd api && go run ./cmd/lintcompose ../docker-compose.yml
