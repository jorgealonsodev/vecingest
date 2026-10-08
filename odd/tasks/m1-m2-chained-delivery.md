# Chained delivery of the M1 and M2 branch

## Goal
Deliver the 48 local commits on `feature/m2-incidents-api` as nine chained pull
requests against `origin/main`, following the work-unit/PR structure the branch
history already records, so each pull request is a unit the repository's
`PULL_REQUEST_TEMPLATE.md` Definition of Done and security checklist can
honestly be verified against.

## Authorization
- The user authorized delivery (push and pull request creation) and selected the
  nine chained pull requests over a single 30k-line pull request.
- Target: `https://github.com/jorgealonsodev/vecingest.git`, default branch
  `main`. `gh` is authenticated as `jorgealonsodev` with `repo` and `workflow`
  scopes.
- No merge is authorized. Merging each pull request stays the user's decision.

## Target policy actually observed
- The repository has never had a pull request, has no issues, and carries only
  GitHub's default labels. There is no `type:*` label and no `size:exception`
  label, so Gentle's issue-first and type-label rules are not this target's
  policy and no label is invented here.
- `.github/PULL_REQUEST_TEMPLATE.md` is the real policy: a per-pull-request
  Definition of Done plus a mandatory security checklist, and a rollback plan
  for anything touching auth, money, votes, files or personal data.
- No workflow validates branch names, so the repository's existing `feature/`
  convention is kept over Gentle's `feat/` regex for consistency with the
  already-pushed `origin/feature/m1-communities-pr5-turnstile`.
- `ci.yml` and `security.yml` trigger on `pull_request` with any base and on
  `push` to `main` only. Pushing the nine branches triggers no workflow.

## Slice plan
Measured with `git diff --shortstat` over the exact commit ranges.

| # | Branch | Base | Tip | Commits | Size |
|---|---|---|---|---|---|
| 0 | `docs/m1-openspec-artifacts` | `main` | 1755844 | 4 | +1999/-11 |
| 1 | `feature/m1-tenant-schema-authz` | 0 | a47eadd | 2 | +3194/-26 |
| 2 | `feature/m1-office-community` | 1 | 86f8edc | 3 | +3028/-22 |
| 3 | `feature/m1-units-csv-import` | 2 | 4970237 | 2 | +3640/-50 |
| 4 | `feature/m1-invitations` | 3 | 604f6a5 | 2 | +3594/-44 |
| 5 | `feature/m1-communities-pr5-turnstile` | 4 | 9ba4715 | 10 | +5742/-225 |
| 6 | `feature/m1-mfa-enrollment` | 5 | f5434cf | 8 | +1767/-51 |
| 7 | `feature/m1-portal-memberships` | 6 | 7ff0eca | 12 | +3375/-205 |
| 8 | `feature/m2-incidents-api` | 7 | 479dc8d | 6 | +3763/-20 |

Slice 5 already exists on `origin` at exactly its tip commit `9ba4715` and
needs no push. Slice 8 is the current local branch.

## Known honest limits
- No slice reaches the advisory 400 authored-line review budget. Rewriting
  history to shrink them is rejected: these commits carry burned native review
  authority, and rewriting would destroy that evidence.
- Intermediate slices are historically accurate, not independently green. Later
  commits fix earlier ones (`4970237` fixes the bootstrap role statement,
  `604f6a5` fixes River sequence grants), so required CI may fail on slices 1-3
  and only converge further down the chain. This is reported, never hidden or
  worked around by amending reviewed commits.

## Tasks
- [x] D-1: Nine local branches created at their exact tip commits; chained
      ancestry verified and no commit rewritten.
- [x] D-2: The eight branches `origin` lacked are pushed, no force push; all
      nine local and remote tips match.
- [x] D-3: Nine pull request bodies drafted in `odd/pr-bodies/`, each from the
      repository template, with only evidence-backed boxes checked.
- [x] D-4: Nine chained pull requests opened, #1 through #9, oldest slice first
      with explicit `--repo` and `--base`.
- [x] D-5: Required CI reported per pull request below. Nothing is declared
      merge-ready.

## Evidence
- Branch state at planning time: `feature/m2-incidents-api`, clean tree, HEAD
  `479dc8d`, 48 commits ahead of `origin/main` (merge base
  `f6c79eb`), 177 files, +30110/-662.
- `origin/feature/m1-communities-pr5-turnstile` is at `9ba4715` and is a direct
  ancestor of HEAD; its pull request was never opened.
### D-3 evidence
- Bodies written by worker `gentle-ai-worker` into `odd/pr-bodies/`, surface
  limited to that directory. Checked boxes per body range from 1 to 5 out of 18;
  every checked box names its evidence inline. No fabricated issue reference and
  no `type:*` or `size:exception` label is requested, because neither exists in
  this repository.
- The worker corrected four facts in the parent's brief, all verified
  independently before acceptance: slice 4 does add the Go migration
  `api/migrations/schema/00009_river_sequence_grants.go`; slice 3 edits
  `api/migrations/bootstrap/00001_roles.go`; slice 5's diff carries `main`'s own
  `security.yml` change through the merge `9ba4715`.
- PARENT CORRECTION, worker overstated a defect: the worker wrote that slice 4
  leaked the decrypted invitation short code into a log. The code disproves it.
  At `604f6a5`, `api/internal/platform/mail/log_mailer.go:41` defines the only
  `LogMailer` method, `SendRaw`, which logs `subject` and `body_len` only and
  discards the recipient explicitly. A grep for a logged `code`, `secret` or
  `token` across `internal/queue`, `internal/http/handlers` and
  `internal/platform/mail` at that tip returns nothing. The real defect fixed by
  `4e45d03` is a silent delivery failure: the job decrypted, rendered, returned
  `nil`, and was recorded completed without retry or dead-letter while the API
  answered 201. `4e45d03`'s own commit message wording "wrote it to a log"
  overstates what the code did. The pr4 body was corrected in three places.

### D-4 and D-5 evidence
- Pull requests #1 to #9 opened and chained. GitHub's computed diffs match the
  plan exactly, confirming the chain resolves correctly: #1 14 files +1994,
  #2 37 +3194/-26, #3 24 +3028/-22, #4 30 +3640/-50, #5 32 +3594/-44,
  #6 74 +5742/-225, #7 32 +1767/-51, #8 35 +3375/-205, #9 23 +3763/-20.
- Required `ci`: SUCCESS on #1 through #8. FAILURE on #9 only.
- PREDICTION NOT BORNE OUT, recorded honestly: this tracker predicted required
  CI might fail on slices 1-3 because later commits fix earlier ones. It did
  not. `ci` passed on every intermediate slice, so that risk did not
  materialize. The pull request bodies state it as a possibility, not a fact.
- #9 `ci` failure is NARROW AND SLICE-CAUSED: only the `gofumpt check` step of
  the `go` job failed, listing `internal/authz/scoped/register_test.go`,
  `internal/http/api/incident_test.go` and `test/incidents_persistence_test.go`.
  Every other job passed, including `e2e`, `coverage`, `scope`, `gen` and `ts`.
  All three files were last modified by M2 commits (`34d3971`, `479dc8d`,
  `5634883`), which is why #2 passed with an earlier, formatted
  `register_test.go`.
- ROOT CAUSE OF THE LOCAL/CI GAP: `gofumpt` lives in the `lint` target
  (`Makefile:40`, `cd api && gofumpt -l -d .`) and nowhere in the verification
  that was actually run. Local runs covered focused tests, full suites and
  `lint-scope`, never `make lint`, and `gofumpt` is not installed in this
  worktree. The 117/117 local PASS is therefore still accurate and still
  incomplete: it never included a formatting check.
- Required `security`: FAILURE on all nine, confirmed PRE-EXISTING and
  repository-wide because it fails identically on #1, which is 14 markdown
  files and no code. Four jobs fail while `gosec` and `govulncheck` pass:
  - `gitleaks`: full blocking scan of the entire history reports 7 leaks over
    106 commits, one fingerprinted at
    `a6976ab:api/internal/http/api/invitation_test.go:generic-api-key:482`.
    Being a whole-history scan, it fails on every pull request regardless of
    content.
  - `pnpm audit --audit-level=high`: 20 vulnerabilities, 7 moderate, 12 high
    with 2 ignored, 1 critical, in transitive Expo dependencies such as
    `app>expo>@expo/cli>compression` (GHSA-vc2v-76pw-4v95).
  - `semgrep`: 1 finding across 170 rules and 231 files. The specific rule was
    not yet identified.
  - `trivy`: filesystem/dependency scan exits 1. The root cause was not yet
    isolated from the log.
- No pull request is merge-ready. `security` blocks all nine, and `gofumpt`
  additionally blocks #9.

### D-1 and D-2 evidence
- Two defects were found and corrected during branch creation, both before any
  push:
  1. The local `feature/m1-communities-pr5-turnstile` sat at `f5434cf`, eight
     commits ahead of `origin`'s `9ba4715`, and those eight commits are exactly
     the MFA enrollment slice. The local branch was moved back to `origin`'s
     `9ba4715` with `git branch -f`. Every one of the eight commits
     (`c6fe3a7`, `ef7e825`, `3b7d6cb`, `74f1ccd`, `311c4fd`, `ce447d3`,
     `cfb1a35`, `f5434cf`) was verified reachable from
     `feature/m1-mfa-enrollment` first, so nothing was lost.
  2. Slices 0-4 do not have current `origin/main` as an ancestor: their
     merge base is the older `47227c3`, because current main entered the branch
     later through the merge `9ba4715`. The chained three-dot diffs were
     therefore remeasured and they match the plan, so GitHub will compute each
     slice correctly. Only slice 0 differs slightly from the two-dot figure:
     14 files `+1994` rather than 15 files `+1999/-11`.
- Pushing the eight branches triggered no workflow, as expected: `ci.yml` and
  `security.yml` run on `pull_request` and on `push` to `main` only. `gh run
  list` shows no new run.
- PRE-EXISTING RED ON MAIN, not caused by these slices: the latest runs on
  `main` at `f6c79eb` are `ci` success but `security` FAILURE and `deploy`
  FAILURE (runs 35775418922 and 35775419322, 2026-09-22). The pull request
  template requires `security.yml` green, so that checklist item cannot be
  honestly checked on any slice until the pre-existing `security` failure on
  main is diagnosed. This is separate work and must not be hidden.
- This tracker is deliberately left uncommitted. Committing it onto
  `feature/m2-incidents-api` would alter the tip of slice 8, which already
  carries burned native review authority for target
  `sha256:273762f816ef8ca8a8163990450eb0b645470d65c432d4821adca9d00df666fd`.
  It needs its own branch off `main` or a later placement decision.
- Last full local verification of the M2 tip (previous session):
  `./internal/http/api/...` 117 RUN / 117 PASS, 0 FAIL, 0 SKIP, 303.099s;
  `./test/...` ok 36.818s; lint-scope OK; `git diff --check` clean; permission
  matrix 37/37 operations with 216 assertions. That evidence covers the tip of
  slice 8, not each intermediate slice.
