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
- [x] M2-5 (slice B): Typed incident authorization, visibility-before-handler and regression tests; commit 34d3971. Native consolidated review approved and exact acknowledgement burned authority.
- [x] M2-3 (slice C1): Authorized create API, settings/current-role checks, matrix/tests/generated contracts; commit 3bd48ae. Independent focused verification and committed-slice native review approved/acknowledged.
- [x] M2-6 (slice C2): List/detail transport, safe projection, bounded filter-bound keyset pagination, typed scopes, permission matrix and generated contracts/tests. Independently verified and natively reviewed after one bounded correction.
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
- Native ASSESS: medium risk, large runtime writer, reviewDue slice_budget_reached; self-verification stands, independentVerifier false. No additional independent verifier required for slice B. Exact continuation returned review.status with native target cwd/contract/next-transition.
- Native target sha256:e4c76329d9bf422286eb5298b18d13974eef4424bf940b9fcea15b2145552238, lineage review-8adca0eefaa3e5d1, medium, 11 paths/483 diff lines, reliability lens approved; acknowledgement burned authority. First consent binding expired without lineage/mutation; provider-directed fresh START succeeded through host UI. No correction required. Work-unit commit 34d3971.
- Separate passive tracker delta 52bd14de approved/acknowledged in review-1208b2d9c5204c52; not additional source review.

## Slice C1 evidence
- Worker mux4t4i7-b-ku94 completed create only, 11 files, 557 authored +310 generated additions, preserving full coverage beyond advisory. Missing tenant setting defaults true, explicit false remains false; exact active unit membership enforced and safe projection used.
- Meaningful RED: missing route404 (3.606s), then explicit unit_id:null incorrectly succeeded201 before decoder correction. GREEN Docker-focused Incident command PASS4.284s, 1 integration test/zero skips; focused final rerun passed. No separate cleanup refactor claimed.
- PermissionMatrix PASS3 tests,35/35operations,192assertions. sqlc/makegen reproducible, lintscope/short/diffcheck PASS; short deliberately skips DB tests. Custom DTO decoding rejects explicitnull; publicnoise maps to storednoise_and_coexistence.
- ASSESS unassessable due to untracked declaration, conservative independentVerifier true. Independent verifier mux5g579-c-x3zg: focused1test PASS4.367s; matrix3tests35/35ops192assertions PASS4.863s; full DockerAPI93top-level+4subtests zeroobservedskip/fail PASS278.531s; fullDB PASS36.143s; lintscope/short PASS(short cached, DB deliberately skipped). No unintended mutations, diffcheck clean; untracked sources excluded from tracked stat.
- High candidate-exposed gap reproduced by fix writer: expired/future/deleted-unit common creators and staleowner+activetenant with tenant setting false returned201 instead of403. Incident-local current owner/tenant evidence query and all-scope creation gate now enforce active nondeleted same-community membership; exactunit gate retained. No shared M1 resolver change, admin/adminstaff or creatorREAD alteration. Focused Incident GREEN, matrix35/35ops192assertions, sqlc/lintscope/short/diffcheck PASS; no command times reported by followup. Prior fullAPI/DB PASS predates fix, not post-fix evidence.
- Post-fix verifier mux61aes-e-k7sp: focusedIncident+matrix4tests PASS7.843s35/35ops192assertions; fullAPI93top-level+4subtests zeroobservedskip/fail PASS280.471s; DBPASS36.724s; lintscope/diffcheck PASS. Inactive-only and staleowner/activetenant403 directly observed.
- Mixed-role correction mux6ac8l-f-3hty: RED activeowner+activetenant common/ownedunit false-setting returned403 (4.285s). Minimal2filefix applies setting only when HasActiveTenant && !HasActiveOwner. GREEN focused1Dockerintegration PASS4.614s; matrix35/35ops192assertions PASS4.969s; lintscope/short/diffcheck PASS(shortDBskips). Inactiveowner/staleowner tests remain denied, activeowner grant preserved. No SQL/sharedresolver/admin/READ changes.
- Finalfocused independent verifier mux6dyz9-g-a4ma PASS4tests/zero skips/fails8.014s; matrix35/35ops192assertions; lintscope/diffcheck PASS/no unintended mutations. Observed activeowner+tenant201, staleowner/tenantonlyfalse403, inactiveonlycommon403. Prior mixedrole blocker resolved, no remaining causal blocker in focused scope. RED not independently replayed; previous fullresults predate2filefix; finalfullafterC2 pending.
- Medium schema limitation: OpenAPI description documents scope/unit rule, generated Zod unit_id remains optional without conditional validation. Server enforces required unit target/forbidden common target. Track honestly; do not silently broaden shared generator or claim client validation enforces that rule. Writer-reported RED/gen reproducibility not independently replayed. Source native review and commit pending.

## C1 closure
- Work-unit commit 3bd48ae, 12paths1006insertions4deletions. Native committed-only range from 34d3971229539e329efc13bf9a1ad2ba76a9aeb9 isolates C1; target sha256:a36bfb34c166f2185c08ef21815f6057183bf0ed50b740f3c380e081364747a0, lineage review-d56b8c67aa337d4d, medium/reliability approved and exact acknowledgement burned authority.
- One informational R3-ConditionalClientSchema at packages/shared/src/schemas/index.ts:184 is separate later work, not a correction/reopened review. Server cross-field validation remains authoritative; client schema limitation disclosed.
- Initial workspace START failed stale_target_identity when unrelated env.example changed during consent. Fresh target STATUS showed no created candidate; preserved env.example without staging/reading credential values and reviewed committed C1 instead. env.example remains unrelated human work, never edit/stage it in C2.

## Slice C2 evidence
- Writer mux6q5zo-h-2cj9 returned status partial: 832 authored (+820/-12) and 330 generated lines across dto, handlers, incident tests, permission matrix, OpenAPI and shared client/schemas.
- Writer checks: focused Incident PASS 4 top-level + 10 subtests, no skips, 11.687s; PermissionMatrix PASS 37/37 operations and 216 assertions; lint-scope and short PASS with DB tests skipped; scoped generators run twice with matching hashes.
- RED EVIDENCE GAP: the pre-implementation focused failure output was truncated, so test-first ordering for C2 is unverified. Recorded honestly rather than assumed.
- Parent ran `make gen`: exit 0, no additional changed paths, and the bound candidate snapshot identity stayed identical, so the writer's generation was already complete and reproducible. The writer could not run it because sqlc writes outside its allowed surfaces.
- Independent verifier mux7o6t6-i-no10 verified with gaps, no blocking or candidate-caused defect: full Docker API 96 top-level / 110 counting subtests PASS, 0 skip, 0 fail, 288.5s; `./test/...` ok 40.287s (no `-v`, so no per-test counts); focused Incident+Matrix 7 top-level + 10 subtests PASS 15.137s; lint-scope OK. `make test-short` lacks `-count=1`, so 30 of 31 results came from the Go test cache and are not fresh or full-suite evidence.
- Verified per `path:line`: list and detail rerun the visibility SQL per request (handlers/incidents.go:320,346,477); the cursor is bound to version, community, caller and filters (:397) and its anchor is rechecked through GetVisibleIncidentByID (:294-307), so a cursor is pagination state and never widens access. President-alone, other-household, cross-community, deleted and absent all return 404; creator detail access survives membership expiry. A 105-row traversal sharing one created_at preserved exact id DESC order with no duplicates or skips; default 20, cap 100, empty final cursor; invalid filters, limits, cursors and unknown parameters are 400/422; 11 internal keys absent from responses; `noise` round-trips against stored `noise_and_coexistence`.
- Generated `items` permitting `null` is a Huma generator shape limitation, not a contract defect: the same shape already covers 14 HEAD array fields and the server always builds a non-nil slice (handlers/incidents.go:328).
- Native lineage review-9a636b69c68dfbca, medium, 1176 changed lines. First consolidated reliability run returned correction_required with BLOCKER R3-MailFromSplit at env.example:109-110: the unrelated dirty human edit had split the quoted MAIL_FROM value mid-word. Submitted a 3-line correction plan, rejoined the line so env.example matches HEAD exactly with zero diff and no content lost, then the env.example-scoped targeted validator approved. Acknowledgement burned authority for corrected target sha256:4e582a12848870ad9b883dfdd642fe69f898e8f20c99e1f10f0b82948c55f4d3.

## Open follow-ups (separate work, never corrections)
- L1 test gap: the cross-community cursor case (incident_test.go:597) also swaps the caller token, so the caller check rejects it before the community binding is exercised on its own. The handler does check community binding; only the test lacks isolation.
- L2 note: LIST requires active community membership, so a creator with expired membership gets 404 on LIST while DETAIL still works, matching the settled creator policy for detail.
- Six informational advisories from the approved review: R3-FilteredPaginationUnproved (handlers/incidents.go:312-319), R3-MatrixIncidentFixtureScope (permission_matrix_test.go:595-605), R3-UnboundedCursorLoop (incident_test.go:554-567), R3-VisibilityRecheckNegativeOnly (incident_test.go:647-651), R3-PanickingTestAssertions (incident_test.go:636), R3-UndocumentedQueryStrictness (dto/incidents.go:64-76).
- R3-ConditionalClientSchema from C1 (packages/shared/src/schemas/index.ts): the scope/unit_id cross-field rule stays server-enforced only.
- `make test-short` has no `-count=1`, so its output can come from the Go test cache; never cite it as fresh or full-suite evidence.

## Next step
M2 incident API create/list/detail is implemented, verified and reviewed. Remaining M2 scope (photos, comments, state transitions, assignment, notifications, followers, duplicates, company inbox, UI) is untouched and unauthorized. M1 external CI, deployment and functional closure remain separate pending work. No push, PR or merge performed. User requests autonomous optimal choices until completion and maximum useful delegation, subject to mandatory consent and safety boundaries.
