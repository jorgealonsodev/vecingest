# Platform Bootstrap Specification

## Purpose

The `vecingest` CLI, startup config validation, secret isolation,
migrations, superadmin provisioning, and M0 build/deploy hardening
evidence, per PRD §5.1, §7.7, §8, §9, §10.1.

## Requirements

### Requirement: CLI Subcommands

The `vecingest` binary MUST expose `serve`, `worker`, `migrate`, `seed`,
`bootstrap-superadmin`, and `health` subcommands from a single binary.
`health` MUST support both `--ready` and `--worker` modes, matching the
two compose healthcheck invocations at PRD §8.1 line 1187
(`["CMD", "/vecingest", "health", "--ready"]`, the `api` service) and
line 1206 (`["CMD", "/vecingest", "health", "--worker"]`, the `worker`
service).

#### Scenario: Subcommand dispatch

- WHEN `vecingest bootstrap-superadmin` is invoked
- THEN it runs the bootstrap flow and not any other subcommand's behavior

#### Scenario: health --ready invoked as the api container healthcheck

- GIVEN the `api` container's healthcheck configuration
- WHEN it runs `vecingest health --ready`
- THEN the command exits 0 or 1 based on `/v1/health/ready` and performs
  no other subcommand's behavior

#### Scenario: health --worker invoked as the worker container healthcheck

- GIVEN the `worker` container's healthcheck configuration
- WHEN it runs `vecingest health --worker`
- THEN the command exits 0 or 1 based on the River client's own heartbeat
  freshness and performs no other subcommand's behavior

### Requirement: seed Refuses To Run Outside Non-Production

`vecingest seed` MUST check `APP_ENV` before opening any database
connection and MUST refuse to run when `APP_ENV` equals `production`, is
unset, or is any value other than a recognised non-production value (PRD
§8 line 1106: "solo si `APP_ENV != production`"). An unset or
unrecognised `APP_ENV` MUST be treated as production and refused — a
deliberate fail-closed divergence from the PRD's literal `!=` check.

#### Scenario: Refused with APP_ENV=production

- GIVEN `APP_ENV=production`
- WHEN `vecingest seed` is invoked
- THEN it exits non-zero before opening a database connection and
  creates no fixture data

#### Scenario: Refused with APP_ENV unset

- GIVEN `APP_ENV` is not set in the environment
- WHEN `vecingest seed` is invoked
- THEN it exits non-zero before opening a database connection

#### Scenario: Runs with a valid non-production APP_ENV

- GIVEN `APP_ENV=development`
- WHEN `vecingest seed` is invoked against a reachable database
- THEN it proceeds to create the M0 fixture set

### Requirement: Config Startup Validation

`internal/config` MUST validate every required environment variable at
startup and MUST prevent the process from starting when one is missing,
reporting which variable (PRD 10.1 gate item 5).

#### Scenario: Missing required variable

- GIVEN a deployment missing `JWT_SECRET`
- WHEN the process starts
- THEN it exits without serving traffic and logs which variable is missing

### Requirement: Per-Role Database Write and Read Handles

The process MUST expose distinct write and read database handles for
each process role (`serve`'s `ServeDB`, `worker`'s `WorkerDB`), per PRD
§7.7 ("El código se escribe desde M0 con esa separación (`ServeDB` con
pooler, `WorkerDB` directo)") and PRD §7.10 point 2, which lists
`ServeDB`/`WorkerDB` and `ReadDB`/`WriteDB` under "Qué está preparado
desde M0". Each role's read handle MUST default to the same DSN as
`DATABASE_URL` when no `DATABASE_URL_READ`/`DATABASE_URL_WORKER`
override is configured, and no configured override MUST widen the
runtime role's (`app_rw`) privileges beyond what an unconfigured handle
would hold: a configured `DATABASE_URL_READ` or `DATABASE_URL_WORKER`
MUST authenticate as `app_rw` and no broader role, verifiable by running
`SELECT current_user` on that handle and asserting the result is
`app_rw`.

#### Scenario: Write and read handles are distinct pool objects

- GIVEN the `serve` process has started with no read-DSN override
- WHEN its database handles are inspected
- THEN the write handle and the read handle are distinct pool objects

#### Scenario: Unconfigured overrides default to DATABASE_URL

- GIVEN neither `DATABASE_URL_READ` nor `DATABASE_URL_WORKER` is set
- WHEN the process builds its per-role handles
- THEN both handles resolve to the same DSN as `DATABASE_URL`

#### Scenario: A configured read override cannot escalate the runtime role

- GIVEN a `DATABASE_URL_READ` override is configured for the `serve`
  process
- WHEN `SELECT current_user` is run on the read handle
- THEN it returns `app_rw`, never a broader role

#### Scenario: A configured worker override cannot escalate the runtime role

- GIVEN a `DATABASE_URL_WORKER` override is configured for the `worker`
  process
- WHEN `SELECT current_user` is run on the worker handle
- THEN it returns `app_rw`, never a broader role

### Requirement: ENCRYPTION_KEY Isolation

After startup validation, `ENCRYPTION_KEY` MUST be removed from the process
environment and MUST NOT be reachable from the exported config object; it
MUST live in a separate secrets holder, so serializing a config value
cannot leak it.

#### Scenario: Config object excludes the key

- GIVEN startup validation has completed
- WHEN the exported config object is serialized (e.g., for logging)
- THEN it contains no `ENCRYPTION_KEY` field or value

#### Scenario: Environment scrubbed

- WHEN startup validation completes
- THEN `os.Getenv("ENCRYPTION_KEY")` returns empty thereafter

### Requirement: Embedded Migration Runner With Advisory Lock

Migrations MUST run embedded (`//go:embed`) at startup with no external
`goose` binary, guarded by a PostgreSQL session-level advisory lock so
concurrent `api`/`worker` boots do not race.

#### Scenario: Concurrent boot does not race

- GIVEN `api` and `worker` starting simultaneously against an unmigrated
  database
- WHEN both attempt migrations
- THEN one holds the advisory lock and migrates while the other blocks,
  then finds the schema already at target version

### Requirement: Idempotent Superadmin Bootstrap

`vecingest bootstrap-superadmin` MUST take the superadmin email as an
invocation argument and the password via standard input
(`--password-stdin`), MUST NOT accept the password as a command-line
argument, MUST NOT read either credential from environment variables, and
MUST be idempotent: running it twice against the same database MUST NOT
duplicate the user and MUST NOT error.

#### Scenario: First run creates the superadmin

- WHEN an operator pipes the password to
  `vecingest bootstrap-superadmin --email=admin@example.com --password-stdin`
  against a fresh database
- THEN exactly one superadmin user exists afterward and the password never
  appears in shell history or the process argument list

#### Scenario: Second run is a no-op

- GIVEN a database already bootstrapped with a superadmin
- WHEN the password is piped again to
  `bootstrap-superadmin --email=admin@example.com --password-stdin`
- THEN no duplicate user is created and the command exits successfully

### Requirement: Resource and Container Hardening Budget

`api`/`worker` MUST stay under 128 MB memory at rest, the image MUST stay
under 30 MB, boot MUST complete under 1 second, and containers MUST run
`read_only`, non-root, with `cap_drop: ALL` and `GOMEMLIMIT` set
(PRD 10.1 gate items 6, 8). Portainer access restricted to 2FA-enabled
operators and `docker.sock` not exposed to other containers are also
part of gate item 8, but have no automatable test; they MUST be verified
by archived manual evidence on the deployed server (Checkpoint B).

#### Scenario: Measured on the deployed server

- GIVEN the stack deployed on the target server
- WHEN `docker stats` and `docker inspect` are captured
- THEN budget and hardening flags hold, archived in
  `docs/security/gates/M0.md`

#### Scenario: Portainer and docker.sock verified manually

- GIVEN the stack deployed on the target server under Portainer
- WHEN an operator audits Portainer's 2FA requirement and `docker.sock`
  exposure
- THEN the result is archived in `docs/security/gates/M0.md` as manual
  evidence, not an automated test

### Requirement: CI Security Scanning Gate

CI MUST run `gitleaks`, `govulncheck`, `gosec`, `pnpm audit
--audit-level=high`, Trivy, and Semgrep as blocking checks
(PRD 10.1 gate item 7).

#### Scenario: Vulnerable dependency blocks merge

- GIVEN a PR introducing a dependency with a known high-severity CVE
- WHEN CI runs
- THEN the security scan step fails and the PR cannot merge

### Requirement: Immutable and Moving Image Tags

Images tagged `sha-<short>` or `v<semver>` MUST be immutable and MUST NOT
ever be overwritten. `latest` MUST be a moving pointer that always
resolves to the most recently published build.

#### Scenario: Version tag never overwritten

- GIVEN an image already published as `v0.3.0`
- WHEN a new build completes
- THEN `v0.3.0` still resolves to its original digest

#### Scenario: latest always moves

- WHEN a new build is published to `main`
- THEN `latest` is repointed to that build's digest

### Requirement: Initial Threat Model Document

The repository MUST contain `docs/security/threat-model.md` derived from
PRD §6.1's threat table before M0 closes (PRD 10.1 gate item 12).

#### Scenario: Document present and populated

- WHEN `docs/security/threat-model.md` is reviewed
- THEN it lists the threats and controls from PRD §6.1
