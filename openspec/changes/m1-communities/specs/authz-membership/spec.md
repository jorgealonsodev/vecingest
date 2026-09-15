# Authz Membership Specification

## Purpose

The tenant-isolation enforcement layer PRD §7.7 names (`RequireMembership`)
made structurally hard to omit, per proposal Decision 1's layered guarantee:
compile-time, registration-time fail-closed, test-time. Tenant column per
scoped table: `community_id` (via `unit_id`/`invitationId` resolution) or
`office_id`.

## Requirements

### Requirement: Membership Type Cannot Be Fabricated

The system MUST expose an `authz.Membership` type with no exported
constructor outside the `authz` package and no zero-value that passes
validation, so no handler can construct a membership itself.

#### Scenario: Zero-value membership rejected

- GIVEN a `Membership{}` zero value
- WHEN it is passed to any code path that checks membership validity
- THEN the check fails

#### Scenario: Membership only exists via resolution

- WHEN a scoped handler runs
- THEN its `Membership` argument was produced only by `authz`'s internal
  resolver, never by caller-constructed literal

### Requirement: Scoped Handlers Register Exclusively Through Typed scoped Constructors

Every operation under a route carrying a tenant-scope resource
(`{communityId}`, `{officeId}`, `{unitId}`, an invitation's owning
community) MUST be registered through one of three typed constructors —
`scoped.Community[I, O, PI](api, op, roles, handler)`,
`scoped.Office[I, O, PI](api, op, roles, handler)`, or, for a route with
no path resource (e.g. `GET /v1/communities`, `GET /v1/me`),
`scoped.Self[I, O](api, op, handler)`. `PI` is a pointer-receiver
constraint — `PI interface{ *I; authz.CommunityScoped }` for `Community`,
`PI interface{ *I; authz.OfficeScoped }` for `Office` — that forces the
input type itself to declare its tenant id, because huma parses path
parameters into the typed input `*I` and a single generic function cannot
express three different input constraints. A function with the resulting
signature (`func(ctx, PI, authz.Membership) (*O, error)` for `Community`/
`Office`; `func(ctx, *I, authz.Memberships) (*O, error)` for `Self`) MUST
NOT be assignable to plain `huma.Register`, whose signature takes only
`(context.Context, *I) (*O, error)`. An input type that does not
implement its scope's declared interface MUST fail to compile against the
corresponding constructor.

#### Scenario: Scoped handler signature does not compile against huma.Register

- GIVEN a handler with signature `func(ctx, PI, authz.Membership) (*O, error)`
- WHEN it is passed to plain `huma.Register`
- THEN compilation fails

#### Scenario: A scoped constructor resolves membership before invoking the handler

- GIVEN a scoped route registered via `scoped.Community` (or `scoped.Office`)
- WHEN a request with a valid membership arrives
- THEN the handler receives the resolved `Membership` as its final argument

#### Scenario: Input missing its scope interface fails to compile

- GIVEN an input struct that does not implement `authz.CommunityScoped`
- WHEN it is supplied as the type parameter to `scoped.Community`
- THEN compilation fails

### Requirement: Membership Resolved From Route Resource, Never From Header Or Body

For each scoped route, membership MUST be resolved from the route's path
resource id via an indexed query against `office_members`/`unit_members`
(or the resource's owning `community_id`/`office_id`), and MUST NOT be
taken from a header or request body.

#### Scenario: Foreign resource id resolves to no membership

- GIVEN a caller with no membership row for the community in the path
- WHEN they call a scoped route for that community
- THEN resolution fails and the request is rejected

#### Scenario: Header-supplied tenant id is ignored

- GIVEN a request carrying a valid path resource but a different
  `X-Community-Id`-style header
- WHEN the handler resolves membership
- THEN only the path resource is used

### Requirement: Fail-Closed Boot Assertion Over Three Independent Checks

On `serve` startup, the system MUST run a boot assertion comprising three
independently testable checks and MUST refuse to start if any of them
fails:

- **A1** — every scoped operation documented in `api.OpenAPI()` MUST carry
  a registration marker (`op.Metadata[authz.MetadataKey]`), found by
  walking `Paths` across its eight per-method `*Operation` fields (`Get`,
  `Put`, `Post`, `Delete`, `Head`, `Options`, `Patch`, `Trace`); `*OpenAPI`
  has no `Operations()` method, so this MUST be a walk, not a lookup.
- **A2** — no marked operation MUST exist without a registered resolver
  and a non-empty role set for the scope kind its marker declares.
- **A3** — the route set `chi.Walk` reports as actually served, minus the
  operation set `api.OpenAPI()` documents, MUST be a subset of a
  reviewed, per-entry-justified public-route allowlist. This is the check
  that closes the gap A1 structurally cannot: an operation registered
  with `Hidden: true` is routed unconditionally but is never added to
  `Paths`, so it is invisible to A1 and MUST instead surface as an
  undocumented entry in A3's difference set.

A3 asks `chi.Walk` only whether a route exists, not whether it enforces
authorization; it does not reintroduce the enforcement-visibility gap
that rules out inspecting huma's `Security` field or per-operation
middleware through `chi.Walk`, since humachi hides both inside one opaque
per-operation closure that `chi.Walk` cannot see. Route existence and
authorization enforcement are different questions, and A3 answers only
the first.

#### Scenario: Unregistered scoped operation blocks boot (A1)

- GIVEN an operation under a scoped prefix registered via plain
  `huma.Register` instead of a `scoped.*` constructor, so it carries no
  marker
- WHEN `serve` starts
- THEN A1 fails, the boot assertion refuses to start, and the error names
  the offending operation id

#### Scenario: Marker without a registered resolver blocks boot (A2)

- GIVEN a marked operation whose declared scope kind has no registered
  resolver
- WHEN `serve` starts
- THEN A2 fails and startup is refused

#### Scenario: Hidden scoped operation blocks boot (A3)

- GIVEN an operation registered under a scoped prefix with `Hidden: true`,
  so it is routed by chi but never added to `api.OpenAPI()`'s `Paths`
- WHEN `serve` starts
- THEN A3 finds it in the difference between the chi-served route set and
  the OpenAPI-documented set, it is absent from the allowlist, and
  startup is refused

#### Scenario: Allowlisted public route boots normally (A3)

- GIVEN `/v1/auth/login` registered via plain `huma.Register` and present
  in the public-route allowlist with a stated reason
- WHEN `serve` starts
- THEN A3's difference set contains that route, the allowlist covers it,
  and startup succeeds

### Requirement: Role Authorization Within A Resolved Scope

Given a resolved membership, the `scoped.Community`/`scoped.Office`
constructor MUST check the membership's role against the operation's
declared allowed roles and MUST reject with 403 when the role is not
permitted, even for the caller's own resource.

#### Scenario: Wrong role on own resource rejected

- GIVEN a caller with a `tenant` membership on their own unit
- WHEN they call an operation restricted to `owner`
- THEN the system returns 403

### Requirement: Permission-Matrix Test At 100% Route Coverage

The system MUST maintain a permission-matrix test generated from
`openapi.yaml` exercising every operation × every role × own/foreign
resource, asserting 403 or 404 for foreign-resource access, at 100% of
registered routes (PRD §10.1 M1 gate item 1). `openapi.yaml` documents
only the operations `api.OpenAPI()` knows about, which excludes any
`Hidden: true` operation; generating the matrix from it is sound only
because the Fail-Closed Boot Assertion's A3 check independently
guarantees that the chi-served route set and the OpenAPI-documented set
agree (any served-but-undocumented route either sits in the reviewed
allowlist or blocks boot). The matrix's coverage claim therefore depends
on A3 having passed, not on `openapi.yaml` being complete on its own.

#### Scenario: Foreign community access denied

- GIVEN an `admin` of office A and a community owned by office B
- WHEN they call a scoped route against the community owned by office B
- THEN the response is 403 or 404

#### Scenario: A route missing from the generated matrix fails the check

- GIVEN a newly registered scoped route not yet reflected in the generated
  matrix
- WHEN the matrix-coverage check runs
- THEN it fails until the route is covered
