# mandala v0.5.0 release evidence

Evidence record for the v0.5.0 release. Two kinds of evidence are kept strictly separate:

- **Historical evidence** — verified facts about the v0.5.0 implementation (PR #5 and earlier). True, but **not transferable**: no green run or approval on any earlier SHA satisfies a final gate.
- **Final release gates** — evidence on the **exact final candidate SHA** (the `main` HEAD after the release-preparation PR merges). Each gate is PASS / FAIL / NOT RUN / BLOCKED; a gate that has not run is NOT RUN, never PASS.

Publication procedure: [RELEASING.md](RELEASING.md). Release notes: [release-notes-v0.5.0-draft.md](release-notes-v0.5.0-draft.md). Record date convention: ISO 8601 UTC.

## Verified historical evidence

| Item | Value | Source / evidence |
| --- | --- | --- |
| Baseline version | v0.4.1, published 2026-10-23T12:44:36Z, not a draft, not a prerelease, no attached assets (source-only) | https://github.com/cottondesu/mandala/releases/tag/v0.4.1 |
| Baseline tag | `v0.4.1` annotated tag object `c905818df51058b8112900fa8465c13657a5c97e`, peeled commit `bdff5992e2935d1a55919ec58e111cdaa824f7fc` | `git ls-remote --tags origin 'refs/tags/v0.4.1'`, checked 2026-10-25 |
| Feature PR | #5 `feat: add atomic batch operations and deterministic dry-run`, merged to `main` 2026-10-25T12:10:34Z, base `main` ← head `feat/mandala-v0.5.0-atomic-batch` | https://github.com/cottondesu/mandala/pull/5 |
| Feature implementation commit | `2a680b9c1b26e44203792239d903acc70de9614e` | PR #5 |
| Windows CRLF test fix commit (= PR HEAD) | `1658865157009d13bfda1a4d315411629dac073c` (`test: handle CRLF in batch README example validation`) | PR #5 |
| Main merge commit | `a56576891ec7006c3ae65609f48763dbc1245fe7` | PR #5 |
| CI on feature commit `2a680b9` | Run 40112527358: **failure** (Windows README-example CRLF mismatch) — resolved by `1658865` | https://github.com/cottondesu/mandala/actions/runs/40112527358 |
| CI on PR HEAD `1658865` | Run 40158802643: **10/10 jobs success** (6 Test matrix, 2 Race, 2 CLI smoke) | https://github.com/cottondesu/mandala/actions/runs/40158802643 |
| CI on main merge `a565768` | Run 40191966945: **10/10 jobs success** | https://github.com/cottondesu/mandala/actions/runs/40191966945 |
| Independent audit (implementation) | Claude independent audit rounds during PR #5 development; final state **0 BLOCKER / 0 MAJOR**; Windows CRLF fix audited and PASS; MINOR-4 remained open and was escalated to human decision. Conversation-based review; no GitHub artifact exists. Corroborated in-repo: design doc §14 records the audit rounds and the MINOR-4 outcome (`docs/design/v0.5.0-atomic-batch.md`) | PR #5 development record; design doc §14 |
| MINOR-4 exception approval | Human (cottondesu) approved the Windows skip of `TestBatchSaveFailure` **for v0.5.0 only**, 2026-10-25T11:58:34Z. The comment itself states: `RELEASE: Not authorized by this approval` — this is not release approval | https://github.com/cottondesu/mandala/pull/5#issuecomment-6397343300 |
| Local validation (pre-merge, PR tree) | `go test -count=1 ./...`, `go test -shuffle=on -count=1 ./...`, `go test -race -shuffle=on -count=1 ./...`, `go vet ./...`, `go build ./...`, `git diff --check` green under Go 1.26.10 and 1.27; mutation checks caught all specified defects | [release-notes-v0.5.0-draft.md](release-notes-v0.5.0-draft.md) historical section |
| Preflight check at release-prep start (2026-10-25) | `origin/main` = `a56576891ec7006c3ae65609f48763dbc1245fe7`; tag `v0.5.0` absent locally and remotely; existing tags v0.1.0–v0.4.1 present and untouched | `git fetch origin` + `git tag --list` / `git ls-remote --tags origin 'refs/tags/v0.5.0'` (empty), recorded 2026-10-25 |

## Final release gates

**Final candidate SHA: not yet determined.** It is the `main` HEAD after this release-preparation PR merges. Every gate below refers to that SHA; all are NOT RUN until then.

| Gate | Status | Evidence (URL / SHA / run ID) | Date | Decider | Open items |
| --- | --- | --- | --- | --- | --- |
| SOURCE_VALIDATED | NOT RUN | Candidate SHA TBD; preflight (HEAD = origin/main, clean tree) must be re-run after the release-prep merge | — | human | Run Step 1 of RELEASING.md and record the SHA here |
| LOCAL_TESTS_VALIDATED | NOT RUN | — | — | human | Full local gate set on the exact candidate SHA, both Go toolchains when available |
| INDEPENDENT_REVIEW_VALIDATED | NOT RUN | — | — | auditor + human | Independent review of the exact candidate SHA (docs diff since `a565768`); 0 BLOCKER / 0 MAJOR required; historical PR #5 audit does not carry over |
| REMOTE_CI_VALIDATED | NOT RUN | — | — | human | Fresh 10/10 run on the candidate SHA; runs 40158802643 and 40191966945 are history, not this gate |
| RELEASE_APPROVED | NOT RUN | — | — | human | Explicit publication approval naming v0.5.0 (tag push + release publish). MINOR-4 approval explicitly does not satisfy this |
| TAG_VERIFIED | NOT RUN | Tag absent locally and remotely as of 2026-10-25 (precondition confirmed); creation/verification/push not performed | 2026-10-25 (absence only) | human | Annotated tag on explicit candidate SHA; tag-object and peeled-commit guards; single-ref push |
| RELEASE_PUBLISHED | NOT RUN | — | — | human | Draft → review → prerelease off → Latest decided deliberately → human publishes → public state verified |
| MODULE_INSTALL_VERIFIED | NOT RUN | — | — | human | Isolated `go install ...@v0.5.0`; exact `mandala v0.5.0` version line; proxy metadata + checksum record |

## Known limitations carried into v0.5.0

### Windows MINOR-4 (human-approved exception, v0.5.0 only)

- `TestBatchSaveFailure` is **skipped on Windows**: `os.Chmod` cannot make a directory unwritable there, so this directory-level real-I/O injection is not portable.
- Compensating coverage on every platform: `TestBatchSaveErrorPropagation` verifies exit 2, empty stdout, the `E_IO` diagnostic, an untouched state file, and no success JSON via seam injection.
- Real-filesystem save failure is covered on Unix.
- **Windows-native real-I/O failure inside `Save` is unverified.**
- Human approved this exception for v0.5.0 only (see table above); the approval is not release approval.
- Follow-up: consider a Windows-specific real-I/O failure injection in a future maintenance release (a read-only `state.json` rename-failure approach is documented in design doc §14 but unverified).

### Other known limits (by design, documented in README and design doc §15)

- Single-writer assumption; callers must serialize concurrent mutations.
- No multi-process locking; no lost-update detection between `--dry-run` and a later apply (dry-run output is not a reservation).
- No exactly-once execution guarantee; no power-loss durability guarantee beyond the existing temp-file + fsync + rename writer.
- Input file swap / FIFO race: the pre-open and open same-file identity checks cannot prevent an external writer from swapping the input between them; run batch only with trusted input locations.
- stdout write failure after a successful save may leave a **committed batch with an error exit**: the saved state is not rolled back and partial JSON may remain on stdout; `E_IO` exit `2` results. **Exit `2` alone does not prove the batch was not applied — do not retry automatically on exit code 2 alone; inspect the current state with `show --json` first.**

## Independent audit of the release preparation (2026-10-25)

- **Auditor**: independent auditor (fresh context; implementer explanations treated as leads, not evidence). Facts verified directly against GitHub (`gh pr view`, `gh run view`, `gh api`, `gh release view`, `git ls-remote`) and the local repository; guard snippets sandbox-tested in an isolated `mktemp -d` repository (annotated tag + correct SHA accepted exit 0 under bash and zsh; wrong peeled SHA rejected exit 1; lightweight tag rejected on the type check; failed `ls-remote` aborts the block before any "absent" verdict; pre-existing local tag STOPs exit 1).
- **Verdict**: PASS WITH MINORS — **BLOCKER: 0 / MAJOR: 0 / MINOR: 2 / NOTE: 3**.
- Checklist 1–10 all CONFIRMED: factual accuracy, Git command safety, gate independence (no old-SHA green run can satisfy a final-candidate gate), version consistency (no unpublished-v0.5.0-as-installable, no stale v0.4.0), MINOR-4 scoping, known limits, source-only policy, evidence honesty, scope discipline (no product code / go.mod / schema / workflow changes), sandbox guard tests.
- **Findings — all explicitly approved as recorded by the human owner on 2026-10-25 (accepted without changes):**
  - MINOR — RELEASING.md Step 6 re-verify block reads the locally cached `origin/main` (last fetched at Step 1) without a fresh `git fetch origin`. If `main` advances between Step 1 and Step 6, the tag still targets the exact SHA all gates ran on, but the "candidate = current main HEAD" invariant is weaker than the wording implies. Approved as-is; adding `git fetch origin` to the re-verify block is recorded as an optional future revision.
  - MINOR — the historical "local validation on Go 1.26.10 and 1.27" row cites the release-notes draft, which makes the same claim with no independent source (circular sourcing). Mitigation found by the auditor: both toolchains re-verified green (test/vet/build) on 2026-10-25; the row remains historical and is excluded from all final gates. Approved as recorded.
  - NOTE — `CANDIDATE_SHA` derivation runs outside a `set -euo pipefail` subshell; mitigated by the printed-value human review and the `[ -n "$CANDIDATE_SHA" ]` guard before tagging. No unsafe path exists.
  - NOTE — the untracked `.serena/` files are not covered by the root `.gitignore`; when creating the release-preparation PR, stage files explicitly (`git add README.md docs/...`) rather than `git add .`.
  - NOTE — conversation-based historical claims (implementation audit outcome, mutation-check counts) are inherently unverifiable by a third party; they are labeled as such and quarantined from all final gates.
- **Unresolved concerns**: none blocking. Operational reminders for the PR stage: stage files explicitly; optionally add `git fetch origin` to the Step 6 re-verify block in a future revision.

## Document history

- 2026-10-25 — created during v0.5.0 release preparation; historical evidence verified against GitHub (`gh pr view`, `gh run view`, `gh api`, `gh release view`) and the local repository on branch `docs/mandala-v0.5.0-release-prep`. All final gates NOT RUN.
- 2026-10-25 — independent audit completed (PASS WITH MINORS: 0 BLOCKER / 0 MAJOR / 2 MINOR / 3 NOTE); all findings explicitly approved as recorded by the human owner.
