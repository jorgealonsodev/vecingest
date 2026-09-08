# API Contract Generation Specification

## Purpose

The generation chain from Go structs to the TypeScript client and Zod
schemas, and its CI enforcement, per PRD §7.2, §7.7, §9.

## Requirements

### Requirement: Generation Chain

`make gen` MUST regenerate `api/openapi/openapi.yaml` from the Go
input/output structs via `huma`, then regenerate the TypeScript client
(`openapi-typescript` + `openapi-fetch`) and the Zod schemas
(`openapi-zod-client`) into `packages/shared` from that `openapi.yaml`. No
type in `packages/shared` MAY be hand-written.

#### Scenario: Struct change propagates

- GIVEN a Go input struct gains a new required field
- WHEN `make gen` runs
- THEN `openapi.yaml`, the TS client, and the Zod schema for that struct
  all reflect the new field

### Requirement: CI Dirty-Diff Gate

CI MUST run `make gen` and fail the build if it produces any diff against
the committed `openapi.yaml`, generated TS client, or generated Zod
schemas.

#### Scenario: Uncommitted generated diff fails CI

- GIVEN a PR that changed a Go struct without running `make gen`
- WHEN CI runs the dirty-diff check
- THEN CI fails

#### Scenario: Clean generation passes CI

- GIVEN a PR where `make gen` was run and its output committed
- WHEN CI runs the dirty-diff check
- THEN CI passes with no diff
