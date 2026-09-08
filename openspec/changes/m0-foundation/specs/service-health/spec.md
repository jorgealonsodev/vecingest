# Service Health Specification

## Purpose

Liveness and readiness endpoints for container orchestration, per PRD
§7.7.

## Requirements

### Requirement: Liveness Endpoint

`GET /v1/health/live` MUST return `200 {"ok": true}` whenever the process
is running, with no dependency checks and no detail about internal
dependencies.

#### Scenario: Process running

- GIVEN the `api` process is running
- WHEN `GET /v1/health/live` is called
- THEN it responds 200 with `{"ok": true}` and no dependency detail

### Requirement: Readiness Endpoint

`GET /v1/health/ready` MUST return success only when every dependency
registered in the readiness check registry is reachable, and MUST return
a non-2xx status otherwise. At M0 the registry holds exactly one check —
`postgres` — because `proposal.md` puts object storage out of M0 scope
beyond configuration placeholders: there is no R2 client to probe, and
probing a dependency that does not exist would mean either fabricating a
check or failing readiness outright. PRD §8.1 gates `worker` on
`api: service_healthy`, so a readiness check that fails on an absent
dependency would take the whole stack down on boot. Object storage joins
the registry at M2, when an R2 client first exists.

#### Scenario: Database unreachable

- GIVEN the database connection is down
- WHEN `GET /v1/health/ready` is called
- THEN it responds with a non-2xx status

#### Scenario: Registered M0 dependency reachable

- GIVEN the M0 readiness registry holds only `postgres` and the database
  is reachable
- WHEN `GET /v1/health/ready` is called
- THEN it responds 200
