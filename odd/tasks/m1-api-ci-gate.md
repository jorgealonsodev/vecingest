# M1 API database CI gate

## Objective
Run database-backed API authorization tests in blocking CI without weakening the Docker-free fast lane. Local PASS does not close the security gate: applicable remote CI evidence is required.

## Scope and constraints
- Add a verbose, race-enabled API test step to the existing Docker-enabled e2e job, already required by ci-required.
- Update docs/security/gates/M1.md truthfully; retain OPEN pending remote evidence.
- No API behavior changes, unrelated advisory fixes, push, PR or merge.
- One writer; delegated route triggered by multi-file edits and user preference.
- Delivery strategy: ask-on-risk; forecast below 150 authored changed lines; work-unit base a0486e29b0f18c5006226b8821f1bd17bd949bd9.

## Tasks
- [x] CI-1: Implement the blocking API test step and gate documentation; verify locally and commit as one coherent work unit. Commit: d7340c87fad9372bdaf84c77ecdb3adc82e8e04a.
- [x] CI-2: Assess and complete native review and independent verification for the work unit. High risk; approved acknowledgement completed for review-0172ed21b7658013; authority burned.
- [ ] CI-3 (external pending): Obtain an applicable remote CI run proving TestPermissionMatrix_ForeignResourceDenied PASS, not SKIP, before closing Checkpoint A. Requires publication authorization; no push or PR performed.

## Acceptance and checks
- Fast job remains Docker-free and uses -short.
- e2e job runs go test -v -race -count=1 ./internal/http/api/... without -short and remains in ci-required.
- Existing disposable Testcontainers PostgreSQL fixture used; no developer database changes.
- CI wiring has no meaningful runtime RED locally; structural baseline showed missing API step. No runtime RED is claimed.

## Evidence
- Explorer confirmed Makefile test-e2e only covered ./test/...; API newTestServer skips under -short. Existing e2e job has Docker and is required.
- Writer: git diff --check and Ruby YAML structural validation PASS.
- Writer API: PASS, 271s, 92 PASS records, 0 SKIP/FAIL; TestPermissionMatrix_ForeignResourceDenied PASS; matrix 34/34 routes, 180 assertions.
- Writer make test-e2e: PASS, 32s. Docker 29.8.2 available.
- Independent verifier API: PASS, 268.842s, 92 PASS records, 0 SKIP/FAIL; matrix PASS, 34/34 routes, 180 assertions. YAML structural assertions and git diff --check PASS.
- Parent spot check: git diff --check PASS before commit; mechanically corrected scope-job citation from ci.yml:245-258 to ci.yml:249-262.
- Existing 20-minute timeout retained; observed sequential tests totaled about 303s, remote variance unmeasured.
- Work-unit diff: 3 files, 99 additions/49 deletions (148 authored changed lines).
- Initial assess unassessable due to untracked task file; independent verification ran conservatively. Committed-range assess returned high, security/shell signals.
- Native review: all four reviewers prepared/submitted; approved, exact acknowledgement burned authority. Target: sha256:3812322ca8f70b7f2dfbd8d66892ea1c25419c74b8d9e4f80b47fd574ff9a417.
- A stale expired pre-lineage consent produced no mutation/lineage; fresh scoped START succeeded. No recovery/reset needed.
- No remote execution or evidence archived. Checkpoint A remains OPEN.

## Next step
Human-authorized publication and applicable remote required CI run; archive explicit matrix PASS/not-SKIP evidence before evaluating Checkpoint A closure. Local implementation and review are complete.
