# mandala v0.5.0 release evidence

Version-controlled snapshot of v0.5.0 release preparation. This file preserves history, gate definitions, and initial gate states. It is frozen when the final candidate is selected. It is not a live release-status record. Two layers are kept separate:

- **Tracked historical snapshot** records PR #5 implementation, PR #6 preparation, past CI, known limits, approvals, audits, and the initial eight NOT RUN states. No earlier green run or approval satisfies a final gate.
- **External live release evidence** records all eight gate results on the exact final candidate SHA in new comments on [merged PR #6](https://github.com/cottondesu/mandala/pull/6). Follow [RELEASING.md](RELEASING.md#record-live-evidence-outside-the-source-tree) for required fields, comment chaining, failure handling, and optional external archives. Each gate is PASS / FAIL / NOT RUN / BLOCKED; an unexecuted gate is never PASS.

After candidate selection, do not update this file with the candidate SHA, gate results, approval, or publication outcomes. External comments leave the Git source tree and candidate SHA unchanged. The NOT RUN rows below remain a preparation-time snapshot even after publication. The GitHub Release description links the specific live evidence comments, including the final install result.

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
| Release-preparation PR | PR #6 open at remediation preflight; original HEAD `0c4c0868efd5112f35a633decb258cd67632faca`, base main `a56576891ec7006c3ae65609f48763dbc1245fe7` | https://github.com/cottondesu/mandala/pull/6 |
| CI on original PR #6 HEAD `0c4c0868` | Run 40197731240: completed, success, **10/10 jobs success**. Historical only; cannot validate a later PR HEAD or the post-merge candidate | https://github.com/cottondesu/mandala/actions/runs/40197731240 |

## Final release gates

**Final candidate SHA: not yet determined at snapshot time.** Select it from main only after PR #6 merges. The table defines the gates and their initial state, not their eventual outcome. Record the full candidate SHA and each actual result externally on merged PR #6; never fill this table after selection.

| Gate | Status | Evidence (URL / SHA / run ID) | Date | Decider | Open items |
| --- | --- | --- | --- | --- | --- |
| SOURCE_VALIDATED | NOT RUN | Candidate SHA TBD; preflight (HEAD = origin/main, clean tree) must be re-run after the release-prep merge | — | human | Run Step 1 of RELEASING.md and record the SHA in an external PR #6 comment |
| LOCAL_TESTS_VALIDATED | NOT RUN | — | — | human | Full local gate set on the exact candidate SHA, both Go toolchains when available |
| INDEPENDENT_REVIEW_VALIDATED | NOT RUN | — | — | auditor + human | Independent code and security review of exact candidate; 0 BLOCKER / 0 MAJOR; minors resolved or human-approved, source unchanged, external record verified; no historical audit carry-over |
| REMOTE_CI_VALIDATED | NOT RUN | — | — | human | Fresh 10/10 run on candidate SHA; runs 40158802643, 40191966945, and 40197731240 are history, not this gate |
| RELEASE_APPROVED | NOT RUN | — | — | human | After Steps 1-4 PASS, explicit external human approval naming v0.5.0 and full candidate SHA (annotated tag creation + push + release publish). MINOR-4 approval does not satisfy this |
| TAG_VERIFIED | NOT RUN | Tag absent locally and remotely as of 2026-10-25 (precondition confirmed); creation/verification/push not performed | 2026-10-25 (absence only) | human | Fresh main checks before creation and push; annotated explicit-SHA tag; local and remote object/peel guards; single-ref push |
| RELEASE_PUBLISHED | NOT RUN | — | — | human | Draft → review → prerelease off → Latest decided deliberately → human publishes → public state verified |
| MODULE_INSTALL_VERIFIED | NOT RUN | — | — | human | Isolated `go install ...@v0.5.0`; exact `mandala v0.5.0` version line; proxy metadata + checksum record |

All eight live records use one full SHA. Steps 1-4 precede human approval, Steps 6-7 require it, and Step 8 follows publication. Comment failures block further actions. GitHub comments are editable and deletable, not immutable evidence; append progress and corrections with links to earlier records. Store optional archives outside Git. See the external records for actual progress rather than changing this snapshot.

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

## RELEASE-EVIDENCE-01 remediation (2026-10-25)

- Original severity: MAJOR. The former procedure required writing the final candidate SHA and gate results into this tracked document. That dirtied the required clean tree; committing the results changed the candidate SHA. Updating repository notes before publication could repeat the same failure.
- Remediation: this document and repository draft notes are preparation snapshots. After selection, all live gate results, human decisions, tag checks, publication, and install evidence go to external PR #6 comments. The GitHub Release description incorporates the live results and their comment URLs without a source commit. The candidate remains fixed in the shell and the external record chain.
- Existing audit history and approval above remain unchanged. The first accepted MINOR is now technically addressed by fresh fetches and candidate checks immediately before both tag creation and push. The second MINOR remains an accepted historical-source limitation and satisfies no final gate. The standalone SHA-capture NOTE is addressed by fail-closed capture; the `.serena/` and conversation-evidence NOTEs remain applicable. These follow-ups do not revise or revoke the prior human acceptance.
- Independent re-audit round 1: Claude Opus 5.5 (`claudeAgent/claude-opus-5-5`) reviewed the actual unstaged three-file diff against HEAD `0c4c0868efd5112f35a633decb258cd67632faca`, cross-checked GitHub facts and all five related documents, and ran isolated Git guard fixtures. Result: RELEASE-EVIDENCE-01 RESOLVED, BLOCKER 0 / MAJOR 0 / new MINOR 1 / NOTE 5. This is a conversation-based preparation audit, not a final-candidate gate or GitHub approval artifact.
- New MINOR: candidate capture could ignore failures when pasted into zsh. The correction explicitly runs the capture body in a Bash subprocess. Preparation checks now reject dirty source, main mismatch, and fetch failure under both Bash and zsh. Independent verification below confirms this MINOR is resolved.
- Round 1 NOTE dispositions: record the audit outcome here before candidate selection; frame version statements as preparation-time facts and leave later installation updates to a separate post-release commit; require main-push CI rather than a synthetic PR merge; preserve the clone path externally and require PowerShell candidate checks; retain STOP and human re-evaluation if main advances after tag push. Resume instructions remain prose, and an error can exit the operator's shell. These are operational notes, not publication authorization.
- Preparation verification: Go 1.26.10 and 1.27.1 each passed Test, Shuffle, Race, Vet, Build, and diff check on the documentation working tree. Eighteen isolated scenarios exercise the documented guards; twelve failure scenarios were also reviewed. These are regression checks before commit, not LOCAL_TESTS_VALIDATED or any other final-candidate gate. No GitHub comment, real-repository tag, or publication action was performed.
- Independent final re-audit: Claude Opus 5.5, fresh context, independently extracted the corrected Step 1 snippet and tested clean success, dirty source, main mismatch, and fetch failure under both Bash and zsh. All 8 cases passed. The reviewer also checked the follow-up documentation changes and preservation of history. Final result: RELEASE-EVIDENCE-01 RESOLVED, BLOCKER 0 / MAJOR 0 / new MINOR 0 / NOTE 5 carried from round 1; no new findings; release procedure safe to merge YES. Remaining operational notes are listed above. The final round did not repeat unchanged tag tests or Go suites.
- Audit provenance: conversation-based T3 review tasks `release-evidence-01-independent-r1` and `release-evidence-01-independent-r3` in parent thread `f94417f2-f779-4cf7-bff3-f69c98d12dcd`. Round 2 was interrupted at a fixture-command approval boundary and is not counted as a completed audit. Audit-result recording edits themselves follow the reviewed behavioral diff and do not validate a new commit. No final-candidate gate has run and no release approval has been granted by this remediation.
