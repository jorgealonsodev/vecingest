# Office Management Specification

## Purpose

`offices` and `office_members`: superadmin-only office creation, first-admin
bootstrap, and `admin_staff` addition, per §3 and §7.4. Tenant column:
`office_id`.

## Requirements

### Requirement: Office Creation Restricted To Superadmin

Only `superadmin` MUST be able to create an office via
`POST /v1/admin/offices`; no self-registration path for offices exists
(§3: "es una relación comercial, no un registro abierto").

#### Scenario: Non-superadmin cannot create an office

- GIVEN an authenticated `admin` user
- WHEN they call `POST /v1/admin/offices`
- THEN the system returns 403

#### Scenario: Superadmin creates an office

- GIVEN an authenticated `superadmin`
- WHEN they call `POST /v1/admin/offices` with valid data
- THEN the office is created

### Requirement: First-Admin Bootstrap Without Invitation

When `superadmin` creates an office, the system MUST create the office's
first `admin` user and an `office_members` row with role `admin` directly,
without an invitation record of any kind. That user MUST obtain a password
only through M0's existing forgot-password flow.

#### Scenario: First admin has no usable password until reset

- GIVEN a newly created office with its bootstrap admin user
- WHEN that user attempts to log in before completing forgot-password
- THEN login fails because no password is set

#### Scenario: No invitation row created for the bootstrap admin

- WHEN `superadmin` creates an office
- THEN no row is inserted into `invitations` for the new admin user

### Requirement: Office Staff Addition Restricted To Existing Accounts

`POST /v1/offices/me/members` MUST require `admin` membership in the
caller's office, MUST add an existing user account as `admin_staff` to
that `office_id`, and MUST NOT create a new user account or send an
invitation email.

#### Scenario: Adding an unknown email fails

- GIVEN an admin calling `POST /v1/offices/me/members` with an email with
  no existing account
- WHEN the request is processed
- THEN it fails without creating a user

#### Scenario: Admin adds an existing user as admin_staff

- GIVEN an existing user account and an admin of office O
- WHEN the admin calls `POST /v1/offices/me/members` with that user's email
- THEN an `office_members` row is created with `office_id = O`,
  `role = admin_staff`

### Requirement: Admin Scope Derived From office_members, Never From Client Input

A user's `admin`/`admin_staff` access to a community MUST be derived from
`communities.office_id` matching one of the user's `office_members` rows,
resolved via `authz.Membership`, never from a client-supplied office id.

#### Scenario: Admin from another office cannot read a foreign community

- GIVEN an admin whose only `office_members` row is for office A, and a
  community owned by office B
- WHEN they call a scoped route for that community
- THEN the system returns 403 or 404

### Requirement: GET /v1/offices/me Returns Caller's Offices

`GET /v1/offices/me` MUST return the office(s) the caller belongs to via
`office_members`, scoped to that membership; `GET /v1/offices/me/members`
MUST list only members of the caller's own office(s).

#### Scenario: Admin lists own office

- GIVEN an admin with one `office_members` row
- WHEN they call `GET /v1/offices/me`
- THEN the response contains that office and no others
