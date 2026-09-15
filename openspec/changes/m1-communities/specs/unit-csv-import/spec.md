# Unit CSV Import Specification

## Purpose

Bulk unit/owner import scoped to
`POST /v1/communities/:id/units/import`, with formula-injection and
path-traversal hardening (§5.2, PRD §10.1 M1 gate item 7). Tenant column:
`community_id`, taken from the route.

## Requirements

### Requirement: Downloadable Import Template

`GET /v1/communities/:id/units/import/template` MUST return a CSV template
with the columns required for unit and owner import (portal, floor, door,
type, coefficient, owner name, DNI/CIF), scoped to the caller's community
membership.

#### Scenario: Admin downloads the template

- GIVEN an admin scoped to community C
- WHEN they call `GET /v1/communities/C/units/import/template`
- THEN they receive a CSV file with the documented columns

### Requirement: Dry-Run Validates Without Writing

`POST /v1/communities/:id/units/import?dry_run=true` MUST validate every
row and return per-row validation results without writing any unit or
member row.

#### Scenario: Dry run reports invalid rows without persisting

- GIVEN a CSV with 2 invalid rows and 8 valid rows
- WHEN it is imported with `dry_run=true`
- THEN the response lists errors for the 2 invalid rows and no unit or
  member row is written

#### Scenario: Dry run with all valid rows writes nothing

- GIVEN a fully valid CSV
- WHEN it is imported with `dry_run=true`
- THEN the response reports success and no row is written

### Requirement: Row-By-Row Validation Before Any Write

A non-dry-run import MUST validate every row before writing any row, and
MUST NOT partially commit when any row fails.

#### Scenario: One invalid row aborts the whole import

- GIVEN a CSV with 9 valid rows and 1 invalid row
- WHEN it is imported without `dry_run`
- THEN no row is written and the invalid row is reported

### Requirement: Formula-Injection Hardening

The system MUST escape any cell value beginning with `=`, `+`, `-`, or `@`
when generating a CSV (template or export), and MUST neutralize such a
prefix on values ingested from an import before they are stored or ever
re-exported.

#### Scenario: Exported CSV escapes a formula-prefixed value

- GIVEN a unit or owner field stored with a leading `=`
- WHEN a CSV export is generated
- THEN the cell is escaped so a spreadsheet application does not evaluate
  it as a formula

#### Scenario: Imported row with a formula-prefixed value is neutralized

- GIVEN an import row whose owner-name cell begins with `=`
- WHEN the row is imported
- THEN the stored/re-exportable value is neutralized, not evaluated

### Requirement: Path-Traversal-Safe Filenames

The system MUST reject or sanitize an uploaded import filename containing
path-traversal sequences before deriving any storage key from it.

#### Scenario: Filename with traversal sequence rejected

- GIVEN an uploaded file named `../../etc/passwd.csv`
- WHEN the import request is processed
- THEN the system rejects the filename or sanitizes it before use
