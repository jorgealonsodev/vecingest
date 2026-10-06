# M2 incident API — initial slice

## Objective and authorization
Implement incident creation, list and detail without photos, following PRD_go.md §5.3 and its permission matrix. User selected the no-attachments first slice. M1 external closure remains a separate pending follow-up.

## Scope
- Create incidents with initial open status; list and detail with community/household isolation.
- Tenant creation respects tenants_can_create_incidents.
- Common incidents visible to community members; unit incidents limited to creator, unit members and administrative roles as specified by the PRD. Presidency alone never grants private unit access.
- Preserve documented roles and statuses; do not invent moderation.
- Non-goals: photos/uploads, UI, comments, assignments/company inbox, state transitions, notifications, followers, duplicate workflow, ratings and Stitch mutations.

## Constraints and routing
- One delegated writer at a time; exploration, implementation and verification delegated.
- CodeGraph-first navigation where available; report child tool limitations honestly.
- Test-first for deterministic behavior: observed RED, GREEN and regression checks; documentation uses structural validation.
- No push, PR or merge in this scope.
- Feature base: 7ff0eca; source tree was clean before tracking.
- Delivery strategy: auto-chain; chain_strategy=feature-branch-chain, selected under user's explicit instruction to choose optimal ordinary options without further interviews. Provisional forecast: 900–1500 authored changed lines excluding generated outputs, across coherent units; no cosmetic shrinking or oversized blind write. Record reviewable unit boundaries; publication remains out of scope.

## Settled contract
- admin/admin_staff may create in their community, including any valid target unit. Owners/tenants require active unit membership for unit targets. Tenant creation checks the community setting.
- Body: title, description, documented category, common|unit scope, optional unit_id and location_text. Unit scope requires a same-community target; common scope forbids unit_id. Creator is derived from caller; status is open and initial priority normal. Priority suggestion is deferred; no client-controlled definitive priority.
- List: status/category/unit filters, bounded limit default 20/max 100, stable created_at/id ordering. Implement a next-page cursor following existing conventions if present, otherwise keyset cursor based on the same stable order, bound to filters/community and validated. No silently truncated unpageable list.
- Detail: 404 for foreign/private unauthorized resources; no comments, attachments, events or internal budget/company projections.
- Schema: community-leading keys/indexes and same-community unit FK, preserving current tenant conventions. Exact constraint mechanics verified by writer before edits.

## Tasks and chained slices
- [ ] M2-1 (in progress): Settle contract and tracking document; close with a passive documentation work-unit commit after structural readback.
- [ ] M2-2 (pending, slice A): Schema/query persistence and deterministic integrity tests, including generated sqlc outputs.
- [ ] M2-5 (pending, slice B): Typed incident authorization and household visibility with deterministic tests.
- [ ] M2-3 (pending, slice C): Create/list/detail transport, permission matrix, generated OpenAPI/shared outputs and behavior tests.
- [ ] M2-4 (pending): Independent functional regression and native review/assessment of coherent candidates, recording each commit and evidence.

## Acceptance criteria
- Cross-community and unauthorized household list/detail cannot disclose private incident data or existence.
- Caller identity and community/unit association are validated; client cannot forge creator identity.
- Tenant creation setting is enforced with documented default.
- Typed scoped registration, tenant-key SQL/FK/index conventions and lint-scope hold.
- Existing M1 routes, roles and matrix coverage remain passing.
- Reproducible generated Go/OpenAPI/shared artifacts; runnable API/database checks as applicable.

## Evidence and open points
- PRD_go.md:236–266 defines statuses, creation and visibility; :89–94 includes admin_staff permissions; :234 defines tenants_can_create_incidents default true.
- CodeGraph rebuilt recently; child explorer did not have callable CodeGraph or shell tools. Parent verified clean source tree on feature/m1-communities-pr7-app-portal.
- Explorer handoff mux2pksk-4-fzjq mapped migration 00012_incidents.sql, query incidents.sql, typed authz registration and DTO/handler/routes plus tests. No suites run; no source changes yet.
- Meaningful persistence RED: missing incident table and same-community unit integrity tests. HTTP RED later covers false tenant setting, invalid targets, common/unit visibility, creator forgery and foreign-resource 404.
- Per-slice checks: applicable focused DB/API tests, generation, lint-scope and short regressions; full database-backed API and e2e at closure when available. Documentation has no meaningful behavioral RED.
- Provisional surfaces and commands are explorer recommendations; writer must verify existing naming/conventions rather than trusting child tool availability.

## Next step
Commit the settled contract on an M2 feature branch, then delegate slice A with test-first checks. User requests autonomous optimal choices until completion and maximum useful delegation, subject to mandatory consent and safety boundaries.
