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
- Private incident creator access is an independent PRD §5.3 grant, not contingent on ongoing membership in that unit. Other unit members require current membership. The verifier's stricter creator-membership wording came from the parent verification prompt, not the settled PRD contract; preserve the tested creator exception. Authentication, deleted-resource handling and scoped caller identity remain mandatory.
- Body: title, description, documented category, common|unit scope, optional unit_id and location_text. Unit scope requires a same-community target; common scope forbids unit_id. Creator is derived from caller; status is open and initial priority normal. Priority suggestion is deferred; no client-controlled definitive priority.
- List: status/category/unit filters, bounded limit default 20/max 100, stable created_at/id ordering. Implement a next-page cursor following existing conventions if present, otherwise keyset cursor based on the same stable order, bound to filters/community and validated. No silently truncated unpageable list.
- Detail: 404 for foreign/private unauthorized resources; no comments, attachments, events or internal budget/company projections.
- Schema: community-leading keys/indexes and same-community unit FK, preserving current tenant conventions. Exact constraint mechanics verified by writer before edits.

## Tasks and chained slices
- [x] M2-1: Settled contract and tracking document; passive documentation work-unit commit 082f84d. Structural readback and staged diff whitespace check passed; no behavioral RED applies.
- [x] M2-2 (slice A): Schema/query persistence, generated sqlc and integrity tests; commit 5634883. Independent functional verification and native review approved/acknowledged.
- [ ] M2-5 (in progress, slice B): Typed incident authorization implemented by mux3neoi-9-b5me; source review and commit pending. Independent IncidentAccess validates row visibility before handler; no synthetic role/membership.
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
- Explorer handoff mux2pksk-4-fzjq mapped migration 00012_incidents.sql, query incidents.sql, typed authz registration and DTO/handler/routes plus tests.
- Slice A writer mux2u8y7-5-ve7r returned completed: 6 source/generated/test files, 485 authored + 307 generated additions. Cohesive integrity tests retained despite 400-line advisory.
- Writer RED: focused test command failed compilation because incident sqlc types/methods were absent; no test bodies ran. GREEN: cd api && go test -race -count=1 ./test/... -run IncidentPersistence passed with PostgreSQL 17/Docker in 3.956s. Fixture isolation initially failed and was corrected; rerun passed. sqlc generation, lint-scope, short suite and diff-check passed; short deliberately skipped DB integration, separately exercised focused.
- Independent verifier mux3ccl9-6-ejiw: focused Docker PASS 4.574s; full api/test suite PASS 37.670s including existing Up/Down/Up coverage; lint-scope and short PASS, short cached and DB tests intentionally skip. Diff-check clean and no unintended mutations. No per-test counts reported. RED not independently replayed; generated files read for consistency, not regenerated; migration-12-only rollback not separately executed.
- Verifier reconciliation mux3fxug-7-q6qx withdrew creator-policy conflict; status verified for tested persistence scope, no other concrete unit issue. Accepted PRD creator grant remains independent of unit membership expiry. Initial ASSESS unassessable due to untracked declaration; conservative independent verification applied.
- Slice A commit 5634883 (802 insertions/4 deletions including tracker). Native target sha256:e158f68390d99194d8ad55e668435628ab752cdde8c8c86d2123e7dd7d2370cd, lineage review-f2cf665b0c59617e, medium tier, consolidated review-reliability approved and acknowledgement burned authority. One informational advisory R3-001 at api/test/incidents_persistence_test.go:150–159 is separate follow-up, not a correction or reopened review.
- Passive tracker delta target b7f38c7d was natively approved/acknowledged, lineage review-32792ed44949a3b2, authority burned. This does not review source.
- Meaningful persistence RED: missing incident table and same-community unit integrity tests. HTTP RED later covers false tenant setting, invalid targets, common/unit visibility, creator forgery and foreign-resource 404.
- Per-slice checks: applicable focused DB/API tests, generation, lint-scope and short regressions; full database-backed API and e2e at closure when available. Documentation has no meaningful behavioral RED.
- Provisional surfaces and commands are explorer recommendations; writer must verify existing naming/conventions rather than trusting child tool availability.

## Slice B evidence
- Worker mux3neoi-9-b5me completed 10 source/test/generated files: 443 authored changed lines (424 additions/19 removals), 25 generated additions. Cohesive security tests retained beyond advisory 400.
- RED: focused race authz tests failed due to missing scoped.Incident/IncidentAccess plus unmarked incident route incorrectly passing boot assertion. GREEN: same command passed, package times 1.430s/1.033s. Intermediate test-fixture binding/counter failures corrected, rerun passed.
- sqlc generation, lint-scope, short suite and diff-check PASS. Short deliberately skips DB integration; separate Docker IncidentPersistence run PASS 3.889s. No declared pre-existing failures. No transport routes added.
- Native ASSESS: medium risk, large runtime writer, current candidate unconsumed, reviewDue slice_budget_reached; self-verification stands, independentVerifier false. Follow native review instead of adding redundant separate verification. Exact continuation returned review.status with native target cwd/contract/next-transition.

## Next step
Complete native review and work-unit commit for slice B, then map/delegate slice C transport. IncidentAccess is minted only after lookup and caller visibility; deleted/absent/invisible rows map to 404, genuine DB failures remain internal errors. Transport remains pending. User requests autonomous optimal choices until completion and maximum useful delegation, subject to mandatory consent and safety boundaries.
