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
- [ ] CI-1 (in progress): Implement the blocking API test step and gate documentation; verify locally and commit as one coherent work unit.
- [ ] CI-2 (pending): Assess and complete applicable native review and independent verification for the work unit.
- [ ] CI-3 (external pending): Obtain an applicable remote CI run proving TestPermissionMatrix_ForeignResourceDenied PASS, not SKIP, before closing Checkpoint A.

## Acceptance and checks
- Existing fast job remains Docker-free and uses -short.
- e2e job runs go test -v -race -count=1 ./internal/http/api/... without -short and remains in ci-required.
- Use existing disposable Testcontainers PostgreSQL fixture; no developer database changes.
- Verify YAML/workflow structure and git diff --check.
- Run cd api && go test -v -race -count=1 ./internal/http/api/... and make test-e2e if Docker is available; record unavailable/failed checks honestly.
- CI wiring has no meaningful runtime RED locally; use structural before/after evidence rather than inventing a CI failure.

## Evidence
Explorer confirmed Makefile test-e2e only covers ./test/...; API newTestServer skips under -short. Existing e2e job has Docker and is required. Tree was clean at a0486e29b0f18c5006226b8821f1bd17bd949bd9.

## Next step
Independent verifier PASS; CI-1 commit pending, then CI-2 native assessment/review. Initial assess was unassessable because task document was untracked, so independent verification ran. Keep CI-3 pending without fabricated remote results.

## Writer evidence
- git diff --check and Ruby YAML structural validation PASS.
- API suite: go test -v -race -count=1 ./internal/http/api/... PASS, 271s, 92 PASS records, 0 SKIP/FAIL; TestPermissionMatrix_ForeignResourceDenied PASS; matrix 34/34 routes, 180 assertions.
- make test-e2e PASS, 32s. Docker 29.8.2 available.
- Existing 20-minute timeout retained; observed sequential tests totaled about 303s, remote variance unmeasured.
- Diff: workflow +4 lines; gate 55 additions/48 deletions before citation correction. No remote execution.
- Independent verifier: API PASS 268.842s, 92 PASS records, 0 SKIP/FAIL; matrix PASS, 34/34 routes, 180 assertions. YAML structural assertions and git diff --check PASS.
- Verifier found only scope-job citation drift; parent mechanically corrected ci.yml:245-258 to ci.yml:249-262.
