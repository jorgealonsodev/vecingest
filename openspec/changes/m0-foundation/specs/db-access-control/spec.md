# DB Access Control Specification

## Purpose

Three-role PostgreSQL provisioning, the runtime role's restricted
privileges, the `SECURITY DEFINER` bypass invariant, and the audit log hash
chain, per PRD §6.1, §7.7, §10.1, and research finding 7.

## Requirements

### Requirement: Three-Role Provisioning

The system MUST provision exactly three PostgreSQL roles: the image
superuser (one-shot bootstrap), `vecingest_owner` (schema owner, runs
migrations), and `app_rw` (runtime), via a Go migration since only Go code
can read the runtime password from the environment and open a second
connection under another role.

#### Scenario: Roles exist after bootstrap

- WHEN the bootstrap migration completes
- THEN `vecingest_owner` and `app_rw` both exist as distinct roles from the
  superuser

### Requirement: Runtime Role Append-Only Restriction

`app_rw` MUST hold only `SELECT` and `INSERT` on append-only tables
(`audit_log`, and `votes`/`time_entries` once they exist), MUST NOT own
those tables, and MUST NOT be a member of the owner role
(PRD 10.1 gate item 9).

#### Scenario: UPDATE rejected

- GIVEN a connection authenticated as `app_rw`
- WHEN it attempts `UPDATE audit_log SET ...`
- THEN PostgreSQL rejects it with an insufficient-privilege error

#### Scenario: DELETE and TRUNCATE rejected

- GIVEN a connection authenticated as `app_rw`
- WHEN it attempts `DELETE FROM audit_log` and separately
  `TRUNCATE audit_log`
- THEN PostgreSQL rejects both

### Requirement: Append-Only Restriction Extends To Every Partition

Because PostgreSQL checks privileges on the relation named in the query,
revoking `UPDATE`/`DELETE`/`TRUNCATE` on a partitioned parent table does
NOT restrict its child partitions. `app_rw` MUST hold no `UPDATE`,
`DELETE`, or `TRUNCATE` on `audit_log`'s parent table or on any of its
child partitions, present and future (extends PRD 10.1 gate item 9 to
partitioned children).

#### Scenario: Direct write to a child partition rejected

- GIVEN `audit_log` partitioned by month with `app_rw` granted only
  `SELECT`/`INSERT` on the parent
- WHEN a connection authenticated as `app_rw` attempts
  `UPDATE audit_log_2026_09 SET ...` directly against the child partition
- THEN PostgreSQL rejects it with an insufficient-privilege error

### Requirement: SECURITY DEFINER EXECUTE Invariant

No `SECURITY DEFINER` function or trigger owned by the owner role may hold
`EXECUTE` for `app_rw` or for `PUBLIC` if that function or trigger can
write to an append-only table; the enforcing migration MUST assert this
invariant, not merely document it.

#### Scenario: Migration-time assertion catches a violation

- GIVEN a migration that creates a `SECURITY DEFINER` function writing to
  `audit_log` and leaves `EXECUTE` granted to `PUBLIC`
- WHEN the migration-time assertion runs
- THEN it fails the migration

#### Scenario: Correctly revoked function passes

- GIVEN a `SECURITY DEFINER` function writing to an append-only table with
  `EXECUTE` revoked from `PUBLIC` and from `app_rw`
- WHEN the migration-time assertion runs
- THEN it passes

### Requirement: audit_log Hash Chain

Every `audit_log` row MUST include `prev_hash` and `hash` fields forming a
hash chain, where each row's `hash` is derived from its own content plus
the previous row's `hash` (PRD §6.1).

#### Scenario: Chain links consecutive rows

- GIVEN an existing `audit_log` row with hash H1
- WHEN a new row is inserted
- THEN its `prev_hash` equals H1 and its `hash` is derived from H1 and its
  own content

#### Scenario: Tampering breaks the chain

- GIVEN an `audit_log` row whose content is altered after insertion
- WHEN the chain is verified from that point forward
- THEN the recomputed hash no longer matches the stored `hash`
