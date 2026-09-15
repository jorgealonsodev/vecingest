# Unit Management Specification

## Purpose

`units` and `unit_members`: creation, roles, coefficient validation,
consent capture, per §5.2 and §7.3. Tenant column: `community_id` (units)
and, via `unit_id`, the same `community_id` for `unit_members`.

## Requirements

### Requirement: Unit Creation Scoped To Community

`POST /v1/communities/:id/units` MUST require `admin` or `admin_staff`
membership scoped to the community's `office_id`, and MUST set the new
unit's `community_id` from the route, never from the request body.

#### Scenario: Admin creates a unit

- GIVEN an admin scoped to community C
- WHEN they call `POST /v1/communities/C/units` with valid data
- THEN a unit is created with `community_id = C`

#### Scenario: Owner cannot create a unit

- GIVEN an owner in community C
- WHEN they call `POST /v1/communities/C/units`
- THEN the system returns 403

### Requirement: Unit Uniqueness Per Community

The system MUST enforce uniqueness of `(community_id, block, floor, door)`.

#### Scenario: Duplicate block/floor/door rejected

- GIVEN an existing unit with `block=A, floor=1, door=A` in community C
- WHEN a new unit is created with the same combination in the same
  community
- THEN the system rejects it

### Requirement: Participation Coefficient Sum Is A Warning, Not A Block

The system MUST accept a unit's `participation_coefficient` regardless of
the community-level sum, and MUST surface a warning (not an error) when a
community's coefficients sum outside 100 ± 0.01 (§5.2).

#### Scenario: Coefficients summing to 97 saved with a warning

- GIVEN a community whose units currently sum to 97
- WHEN the coefficients are read or a unit is saved
- THEN the response includes a warning and no write is blocked

#### Scenario: Coefficients summing to 100 produce no warning

- GIVEN a community whose units sum to exactly 100
- WHEN the coefficients are read
- THEN no warning is present

### Requirement: Unit Member Roles And Deferred board_role

`unit_members.role` MUST be `owner` or `tenant`; a unit MAY have zero to
many members recorded independently (co-owners). The `board_role` column
MUST exist on `unit_members` per §7.3 but MUST NOT be settable through any
M1 endpoint or UI; its assignment is deferred to the milestone that builds
the "Junta directiva" tab (M5/M7 data).

#### Scenario: Co-owners recorded independently

- GIVEN a unit with two owners
- WHEN both are added as `unit_members` with role `owner`
- THEN both rows exist and voting counts the unit once elsewhere (not
  this capability's concern)

#### Scenario: No endpoint sets board_role in M1

- WHEN the M1 API surface is enumerated
- THEN no operation accepts a `board_role` value for write

### Requirement: Consent And Notification Fields Captured Per Member

`unit_members` MUST capture `notification_address`,
`electronic_notifications_consent_at` (nullable), and
`consent_text_version` at creation or update (§5.2, §4.1).

#### Scenario: Member created without consent

- WHEN a unit member is created without explicit consent
- THEN `electronic_notifications_consent_at` is stored as null

#### Scenario: Member consent recorded with version

- WHEN a unit member accepts electronic-notification consent
- THEN `electronic_notifications_consent_at` and `consent_text_version`
  are both persisted

### Requirement: Unit Member Management Scoped To Community

`GET /v1/units/:id/members`, `PATCH /v1/units/:id/members/:memberId`, and
`DELETE /v1/units/:id/members/:memberId` MUST resolve membership from the
unit's `community_id` and MUST return 403 or 404 for a caller without a
membership tied to that community.

#### Scenario: Member of another community cannot list members

- GIVEN a tenant in community D and a unit belonging to community C
- WHEN they call `GET /v1/units/:unitId/members` for that unit
- THEN the system returns 403 or 404
