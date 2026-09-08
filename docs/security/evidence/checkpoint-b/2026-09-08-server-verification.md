# Checkpoint B server verification — 2026-09-08

Evidence directory convention: none existed under `docs/security/gates/` before
this batch (only `M0.md` itself), so this is a new directory,
`docs/security/evidence/checkpoint-b/`, created for this batch. All output
below is pasted verbatim from either a `gh` call against GitHub Actions or a
read-only SSH session against the deployed host
(`ubuntu@143.47.47.79`, Oracle Ampere/aarch64). No command that could mutate
the running stack was executed (no restart/redeploy/stop/remove/reconfigure,
nothing written outside `/tmp` on the host, and everything written there was
removed afterward).

Repository HEAD at the time of this evidence: `6c0cb8d9abe244cb59015db88b46b2e693358ebb`.

## 1. Task 13.2 — `ci.yml` + `security.yml` real run on GitHub Actions

```
$ git rev-parse HEAD
6c0cb8d9abe244cb59015db88b46b2e693358ebb

$ gh run list --limit 3
completed	success	docs: correct the firewall item, published ports are blocked by Oracl…	ci	main	push	34276511537	2m49s	2026-09-08T20:43:53Z
completed	success	docs: correct the firewall item, published ports are blocked by Oracl…	security	main	push	34276511534	2m8s	2026-09-08T20:43:53Z
completed	failure	docs: correct the firewall item, published ports are blocked by Oracl…	.github/workflows/deploy.yml	main	push	34276510378	0s	2026-09-08T20:43:53Z
```

Both `ci` (34276511537) and `security` (34276511534) ran against the exact
current `HEAD` commit and are green:

```
$ gh run view 34276511537

✓ main ci · 34276511537
Triggered via push about 6 minutes ago

JOBS
✓ changes (path-based job filters) in 4s (ID 102230869890)
✓ coverage (domain >= 80%, full suite incl. Testcontainers) in 1m15s (ID 102230907502)
✓ ts (turbo lint/typecheck/test) in 43s (ID 102230907588)
✓ scope (make lint-scope, tenant-column enforcement) in 13s (ID 102230907601)
✓ go (lint, build, fast unit -short, golden) in 1m43s (ID 102230907640)
✓ e2e (make test-e2e, Testcontainers Postgres 17) in 2m8s (ID 102230907671)
- gen (make gen dirty-diff gate) in 0s (ID 102230908744)   # skipped: path filter, no generator inputs changed in a docs-only push
✓ ci-required (aggregate gate) in 4s (ID 102231609648)

$ gh run view 34276511534

✓ main security · 34276511534
Triggered via push about 6 minutes ago

JOBS
✓ semgrep (OWASP Top 10, Go, TypeScript, project rules) in 30s (ID 102230746618)
✓ govulncheck (known Go vulnerabilities) in 2m5s (ID 102230746769)
✓ pnpm audit --audit-level=high (app dependencies) in 19s (ID 102230746900)
✓ gitleaks (secret scanning) in 13s (ID 102230747060)
✓ gosec (Go security linter, SARIF) in 48s (ID 102230747124)
✓ trivy (filesystem/dependency scan) in 21s (ID 102230747279)
```

`gen` shows `-` (skipped), not a failure: `ci.yml`'s `changes` job gates it on
a path filter, and this push touched only `docs/pendientes-despliegue.md`, so
the generator dirty-diff check correctly did not run. `ci-required` (the
aggregate gate job) is green, so the skip did not block the pipeline.

Six real scanner jobs ran in `security.yml` — not a linter scanning zero
files: `semgrep` (30s), `govulncheck` (2m5s), `pnpm audit` (19s), `gitleaks`
(13s), `gosec` (48s), `trivy` (21s) — all non-trivial durations, all green.

This closes the previously-open verify-report CRITICAL-6 gap for real: task
13.2 was unmarked in Phase 13 precisely because no GitHub Actions run had
ever executed against this repository. It now has, twice in a row across the
last several pushes, and the run cited above is against the exact commit
this batch leaves at `HEAD`.

### `deploy.yml` — observed, NOT part of 13.2's scope, NOT fixed here

`gh run list` also shows `.github/workflows/deploy.yml` failing on every
push, in 0 seconds:

```
$ gh run view 34276510378

X main .github/workflows/deploy.yml · 34276510378
X This run likely failed because of a workflow file issue.
```

Root cause, confirmed by inspection (not fixed, out of scope for 13.2 which
asks only about `ci.yml` + `security.yml`): `deploy.yml` calls
`security.yml` as a reusable workflow (`uses: ./.github/workflows/security.yml`),
but `security.yml`'s `on:` block has no `workflow_call:` trigger —

```
$ grep -n "^on:" -A5 .github/workflows/security.yml
on:
  pull_request:
  push:
    branches: [main]
  schedule:
    - cron: "17 4 * * 1"
```

(`ci.yml`'s `on:` block does have `workflow_call:` and that half of the
reusable-call works fine.) This is a real, reproducible defect independent
of anything in this batch, and it means the Portainer-redeploy webhook job
at the end of `deploy.yml` has never run from CI — consistent with the
stack having been deployed by hand rather than by the webhook so far (see
§4 below, deployed commit vs. current `HEAD`). Left unfixed and unmarked
anywhere as done; noted here so it is not mistaken for 13.2's scope and not
silently rediscovered later.

## 2. Task 14.1 — provisioning

```
$ ssh ubuntu@143.47.47.79 docker --version
Docker version 29.2.1, build a5c7197

$ ssh ubuntu@143.47.47.79 docker ps --format 'table {{.Names}}\t{{.Image}}\t{{.Status}}'
NAMES                     IMAGE                             STATUS
vecingest-worker-1        vecingest-api:local               Up 25 minutes (healthy)
vecingest-web-1           vecingest-web:local               Up 25 minutes (healthy)
vecingest-api-1           vecingest-api:local               Up 25 minutes (healthy)
vecingest-site-1          vecingest-site:local              Up 25 minutes (healthy)
vecingest-db-1            postgres:17-alpine                Up 48 minutes (healthy)
...
portainer                 portainer/portainer-ee:latest     Up 4 weeks
nginx-proxy-manager-app   jc21/nginx-proxy-manager:latest   Up 4 weeks
nginx-proxy-manager-db    mariadb:latest                    Up 4 weeks

$ dig +short vecingest.xdev.es A
104.21.16.220
172.67.216.64          # Cloudflare-proxied

$ dig +short app.vecingest.xdev.es A
143.47.47.79

$ dig +short api.vecingest.xdev.es A
143.47.47.79

$ curl -s -o /dev/null -w '%{http_code}\n' https://vecingest.xdev.es/
200
$ curl -s -o /dev/null -w '%{http_code}\n' https://app.vecingest.xdev.es/
200
$ curl -s https://api.vecingest.xdev.es/v1/health/live
{"$schema":"https://api.vecingest.xdev.es/schemas/LiveResponse.json","ok":true}
```

Confirmed: Docker (v29.2.1), Portainer (`portainer-ee:latest`, running),
Nginx Proxy Manager (`jc21/nginx-proxy-manager:latest`, running), and DNS
for all three hosts (`vecingest.xdev.es` fronted by Cloudflare, `app.` and
`api.` pointing directly at the host). All three public URLs return the
expected HTTP status with a valid TLS handshake (curl would fail the
handshake otherwise).

**GHCR pull access — not applicable.** Per PRD_go.md §8.1.1, Portainer no
longer pulls images from a registry; it builds `api`, `web` and `site`
itself on the server from this repository (`build:` in `docker-compose.yml`).
There is no GHCR credential to provision or verify for this deployment path.

## 3. Task 14.6 — container hardening (`docker inspect`)

```
=== vecingest-api-1 ===
{
  "Image": "vecingest-api:local",
  "User": "65532:65532",
  "ReadonlyRootfs": true,
  "CapDrop": ["ALL"],
  "CapAdd": null,
  "SecurityOpt": ["no-new-privileges:true"],
  "Memory_bytes": 268435456,
  "Tmpfs": {"/tmp": ""},
  "Env_GOMEMLIMIT": "GOMEMLIMIT=200MiB"
}

=== vecingest-worker-1 ===
{
  "Image": "vecingest-api:local",
  "User": "65532:65532",
  "ReadonlyRootfs": true,
  "CapDrop": ["ALL"],
  "CapAdd": null,
  "SecurityOpt": ["no-new-privileges:true"],
  "Memory_bytes": 268435456,
  "Tmpfs": {"/tmp": ""},
  "Env_GOMEMLIMIT": "GOMEMLIMIT=200MiB"
}

=== vecingest-db-1 ===
{
  "Image": "postgres:17-alpine",
  "User": "",
  "ReadonlyRootfs": false,
  "CapDrop": null,
  "CapAdd": null,
  "SecurityOpt": ["no-new-privileges:true"],
  "Memory_bytes": 805306368,
  "Tmpfs": null,
  "Env_GOMEMLIMIT": "none"
}

=== vecingest-web-1 ===
{
  "Image": "vecingest-web:local",
  "User": "1000:1000",
  "ReadonlyRootfs": true,
  "CapDrop": ["ALL"],
  "CapAdd": null,
  "SecurityOpt": ["no-new-privileges:true"],
  "Memory_bytes": 134217728,
  "Tmpfs": {"/tmp": "", "/var/cache/nginx": "", "/var/run": ""},
  "Env_GOMEMLIMIT": "none"
}

=== vecingest-site-1 ===
{
  "Image": "vecingest-site:local",
  "User": "1000:1000",
  "ReadonlyRootfs": true,
  "CapDrop": ["ALL"],
  "CapAdd": null,
  "SecurityOpt": ["no-new-privileges:true"],
  "Memory_bytes": 134217728,
  "Tmpfs": {"/tmp": "", "/var/cache/nginx": "", "/var/run": ""},
  "Env_GOMEMLIMIT": "none"
}
```

Interpretation, service by service:

| Service | `read_only` | non-root user | `cap_drop: ALL` | `no-new-privileges` | `mem_limit` | `GOMEMLIMIT` |
|---|---|---|---|---|---|---|
| `api` | true | `65532:65532` (distroless nonroot) | yes | yes | 256 MiB (268435456 B) | `200MiB` |
| `worker` | true | `65532:65532` | yes | yes | 256 MiB | `200MiB` |
| `web` | true | `1000:1000` | yes | yes | 128 MiB (134217728 B) | n/a — nginx, not Go; expected absent |
| `site` | true | `1000:1000` | yes | yes | 128 MiB | n/a — nginx, not Go; expected absent |
| `db` | **false** | **root (empty `User`)** | **null** | yes | 768 MiB (805306368 B) | n/a — Postgres, not Go |

`db`'s missing `read_only`/`cap_drop`/non-root is **not a failure** — it is
the documented exception in `docker-compose.yml` itself (the `db` service
comment) and in `docs/security/gates/M0.md`'s own Checkpoint B row 8:
`postgres:17-alpine`'s official image needs a writable data directory, WAL
and temp files, and its own entrypoint privilege drop, so the shared
`*hardening` anchor (`read_only`, `cap_drop: ALL`, `tmpfs`) would break
`initdb` and startup. `db` still gets `no-new-privileges:true` and a
`mem_limit`. This matches the PRD's own reference compose verbatim and
`lintcompose`'s policy table.

`GOMEMLIMIT=200MiB` on `api`/`worker` sits under their 256 MiB `mem_limit`
with headroom for non-Go-heap memory, matching the PRD's stated intent.

## 4. Task 14.7 — `docker.sock` exposure audit (every container on the host)

```
$ for c in $(docker ps -a --format '{{.Names}}'); do
    m=$(docker inspect "$c" | jq -r '[.[0].Mounts[]? | select(.Source=="/var/run/docker.sock")] | length')
    echo "$c: docker.sock mounts=$m"
  done
vecingest-worker-1: docker.sock mounts=0
vecingest-web-1: docker.sock mounts=0
vecingest-api-1: docker.sock mounts=0
vecingest-site-1: docker.sock mounts=0
vecingest-db-1: docker.sock mounts=0
engram-cloud: docker.sock mounts=0
engram-cloud-postgres: docker.sock mounts=0
dufs_preview_server: docker.sock mounts=0
nginx-proxy-manager-app: docker.sock mounts=0
nginx-proxy-manager-db: docker.sock mounts=0
randkeykit: docker.sock mounts=0
clipbin_app_dev: docker.sock mounts=0
clipbin_postgres_dev: docker.sock mounts=0
portainer: docker.sock mounts=1
backrest: docker.sock mounts=0
perreo-ipsum: docker.sock mounts=0
```

Confirmed across **every** container on the host (16 total, not just the 5
vecingest ones): only `portainer` itself mounts `/var/run/docker.sock`. No
other container, vecingest or otherwise, has access to it.

**Portainer 2FA is NOT verified here and is explicitly left open.** There is
no CLI/API surface available in this read-only session to read Portainer's
authentication settings without either logging in through its UI/API with
credentials this session does not have, or querying its internal database —
both out of scope for a read-only infrastructure check. This must be
confirmed by a human logging into the Portainer UI (`https://<host>:9443`,
not reachable from outside during this check — see below) and checking
Settings → Authentication → 2FA. Not guessed, not assumed.

```
$ curl -sk -o /dev/null -w '%{http_code}\n' https://143.47.47.79:9443/ --max-time 5
(timed out, exit 28)
```

Portainer's HTTPS port is not reachable from outside the host during this
check (likely the same Oracle VCN security-list behavior documented in
`docs/pendientes-despliegue.md` §1 for the other published ports) — this is
about reachability, not about the 2FA setting itself, and is included only
so a future check knows why the CLI probe above did not attempt an
unauthenticated API call.

## 5. Supplementary evidence (not required by any task, gathered because it was free and read-only)

Deployed commit vs. current repository `HEAD`, to establish that the
hardening/inspection evidence above reflects a build free of undocumented
drift:

```
$ docker inspect vecingest-api-1 --format '{{.Config.Labels}}' | tr ',' '\n' | grep project.working_dir
com.docker.compose.project.working_dir:/data/compose/25/9a8422ff0d36e5cec8774d54c95988c313fe71c5

$ git log --oneline -1 9a8422ff0d36e5cec8774d54c95988c313fe71c5
9a8422f fix(deploy): derive both migration DSNs in compose instead of hand-typing them

$ git diff --stat 9a8422ff0d36e5cec8774d54c95988c313fe71c5..6c0cb8d9abe244cb59015db88b46b2e693358ebb
docs/pendientes-despliegue.md | 220 ++++++++++++++++++++++++++++++++++++++++++
1 file changed, 220 insertions(+)
```

The deployed stack is running exactly commit `9a8422f`; the four commits
between it and current `HEAD` touch only `docs/pendientes-despliegue.md`
(pure documentation, zero compose/code changes). The `docker inspect`
evidence above is therefore current, not stale.

Resource usage at rest (informal — this is NOT the k6 p95 measurement task
14.5 requires, and does not close it):

```
$ docker stats --no-stream vecingest-api-1 vecingest-worker-1 vecingest-db-1 vecingest-web-1 vecingest-site-1
NAME                 CPU %     MEM USAGE / LIMIT   MEM %
vecingest-api-1      0.00%     4.531MiB / 256MiB   1.77%
vecingest-worker-1   0.19%     6.023MiB / 256MiB   2.35%
vecingest-db-1       0.10%     46.36MiB / 768MiB   6.04%
vecingest-web-1      0.00%     3.191MiB / 128MiB   2.49%
vecingest-site-1     0.00%     3.184MiB / 128MiB   2.49%
```

Image sizes (informal — supports, but does not replace, the PRD's <30 MiB
budget claim, which the CI `advisory image-size report` step already
checks on the CI-built copy, not this server-built one):

```
$ docker image inspect vecingest-api:local --format '{{.Size}}'
7333399   # ~7.0 MiB
$ docker image inspect vecingest-web:local --format '{{.Size}}'
28492407  # ~27.2 MiB
$ docker image inspect vecingest-site:local --format '{{.Size}}'
25911675  # ~24.7 MiB
```

`api`/`worker` memory at rest (4.5 MiB / 6.0 MiB) is far under the PRD's
128 MiB target; this is encouraging but is explicitly **not** the p95
latency measurement gate item 6 requires, which needs an authenticated
`GET /v1/me` request under `k6` (task 14.5, blocked — see the report).
