# Community Management Specification

## Purpose

Communities CRUD scoped by office/unit membership, legal and descriptive
fields, `parent_community_id` groundwork, per §5.2 and §7.3. Tenant column:
`office_id` (creation) and `community_id` (read/update/child resources).

## Requirements

### Requirement: Community Creation Restricted To Admin, Scoped To Office

`POST /v1/communities` MUST require an `admin` membership (not
`admin_staff`, per §3's permission matrix: `admin` has full CRUD on
Comunidad, `admin_staff` has only RU) and MUST set the new community's
`office_id` from the resolved membership, never trusting a client-supplied
`office_id` without validating it matches that membership.

#### Scenario: Admin creates a community in their own office

- GIVEN an admin with an `office_members` row for office O
- WHEN they call `POST /v1/communities` with valid data
- THEN a community is created with `office_id = O`

#### Scenario: admin_staff cannot create a community

- GIVEN an `admin_staff` user of office O
- WHEN they call `POST /v1/communities`
- THEN the system returns 403

### Requirement: Community Read And List Scoped By Membership

`GET /v1/communities` MUST return only communities reachable through the
caller's office membership (`admin`/`admin_staff`) or unit membership
(`owner`/`tenant`). `GET /v1/communities/:id` MUST return 403 or 404 for a
community the caller has no membership tied to.

#### Scenario: Owner sees only their own community

- GIVEN an owner with a unit membership in community C
- WHEN they call `GET /v1/communities`
- THEN the response contains C and no community they have no membership in

#### Scenario: Foreign community detail access denied

- GIVEN an owner in community C and a different community D
- WHEN they call `GET /v1/communities/D`
- THEN the system returns 403 or 404

### Requirement: Community Update Restricted To Office Roles

`PATCH /v1/communities/:id` MUST be permitted for `admin` and
`admin_staff` scoped to the community's `office_id`, and MUST be rejected
for `owner` and `tenant`.

#### Scenario: Owner cannot update community fields

- GIVEN an owner in community C
- WHEN they call `PATCH /v1/communities/C`
- THEN the system returns 403

### Requirement: Legal And Descriptive Fields Persisted Per §7.3

A community MUST record `name`, `cif`, `address`, `city`, `province`,
`postal_code`, `office_id`, optional `parent_community_id`, `settings`
(jsonb), `annual_budget`, `reserve_fund`, `secretary_is_office`,
`last_ordinary_meeting_at`, `dpa_signed_at`, and optional
`transferred_from_office_id`/`transferred_at`. `parent_community_id` MUST
be persisted from M1 even though sub-community vote aggregation is out of
scope, to avoid a later migration.

#### Scenario: Community created without a parent

- WHEN a community is created without `parent_community_id`
- THEN it is stored as null

#### Scenario: Community linked to a parent community

- GIVEN an existing community P
- WHEN a new community is created with `parent_community_id = P`
- THEN the link is persisted and readable on the child's detail response

### Requirement: Community Detail Excludes Cross-Milestone Aggregates

`GET /v1/communities/:id` MUST return the community's own fields (name,
CIF, address, unit and office-member counts) and MUST NOT compute or
return reserve-fund compliance, meeting quorum summaries, or account
balance, since those depend on M5/M7 data not present at M1.

#### Scenario: Community detail omits M5/M7 aggregate fields

- GIVEN a community with no receipts or meetings recorded
- WHEN its detail is requested
- THEN the response contains no reserve-fund-compliance, quorum, or
  balance fields
