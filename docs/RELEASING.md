# Releasing mandala

Manual, human-controlled, source-only releases. This document is a procedure: **none of the publication commands below were executed as part of writing it**, and nothing in the repository auto-releases. A human performs each step. The tracked [release evidence snapshot](release-evidence-v0.5.0.md) preserves preparation history and initial gate states. After candidate selection, record all live evidence in new comments on [merged PR #6](https://github.com/cottondesu/mandala/pull/6), outside the Git source tree.

Current candidate: **v0.5.0** (not yet published). Baseline: v0.4.1 at commit `bdff5992e2935d1a55919ec58e111cdaa824f7fc` — v0.4.1 **is already published** (tag and GitHub Release exist), so nothing in this procedure touches it. Release type: **feature release**. Candidate scope: one feature theme — atomic batch operations (`mandala batch --file <file|->`), deterministic dry-run (`--dry-run`), ID-sorted change preview (text and JSON), and strict JSON file/stdin input with bounded size and operation count. The root command list gains a single `batch` row; existing commands, flags, exit-code meanings, and the `status`/`gaps`/`show` output schemas are unchanged; the state-file schema stays `1` (batch adds a separate result schema `batch_result_version: 1`); no new dependencies.

Version and publication statements here describe preparation time. After publication, use the GitHub Release and its linked live records for current status. Any later installation-document updates belong to a separate post-release main commit, never a rewritten release tag.

The **final candidate SHA is not recorded here and must not be guessed**. Select the intended `main` HEAD only **after PR #6 is merged**, with `HEAD` = freshly fetched `origin/main` at Step 1. Keep that full SHA fixed through all eight gates. Do not edit any tracked file after selection, including this procedure, the evidence snapshot, and the repository release-notes draft. Historical evidence on PR #5 or PR #6 commits does not satisfy final-candidate gates. If source corrections are needed, stop this attempt, obtain review and merge approval for the corrections, then start a new candidate and new gates. Never silently replace the SHA or carry old results forward.

One recorded, human-approved exception applies to v0.5.0 only: on Windows, `TestBatchSaveFailure` is skipped (`os.Chmod` cannot make a directory unwritable there); `TestBatchSaveErrorPropagation` provides compensating coverage on every platform, and real-filesystem save failure remains covered on Unix. Native Windows real-I/O save failure inside `Save` is unverified. This exception (MINOR-4) was approved for v0.5.0 by a human in [the approval comment](https://github.com/cottondesu/mandala/pull/5#issuecomment-6397343300), which states explicitly that it does **not** authorize the release. See the tracked evidence snapshot for the full risk list.

## Record live evidence outside the source tree

The tracked snapshot's eight NOT RUN rows describe release preparation, even when later viewed in a published tag. They are not a live release dashboard. Use PR #6 comments for the candidate, gate results, limitations, approvals, and publication outcomes. Link the specific comment URLs in the GitHub Release description.

Append a new comment for each gate result or correction. Link the preceding records, and preserve earlier comments. GitHub comments can be edited or deleted, so this convention is not cryptographic immutability. An independently stored archive and hashes can provide additional audit evidence. Keep logs, comment drafts, and Release-description drafts outside every release checkout. Posting a comment or changing the GitHub Release description does not change a Git commit.

Each live record must include these fields, with no placeholders left in an actual record:

```text
Release version: v0.5.0
Candidate SHA: <same full 40-character SHA in every record>
Gate name: <one of the eight gates below>
Gate status: PASS / FAIL / NOT RUN / BLOCKED
Execution date (UTC): <ISO 8601 timestamp>
Executor / reviewer: <identity>
Evidence URL / workflow run ID: <links, or not applicable with reason>
Command output or evidence reference: <output or accessible external log>
Approved limitations: <references and human decisions>
Remaining blockers: <list, or none>
Explicit human decisions: <decision and reference, or none>
Previous evidence comments: <specific URLs, or none for the first record>
```

Read back each posted comment and retain its URL in the external record chain. A gate is complete only when its result and supporting evidence are recorded and accessible. If a required comment cannot be posted or verified, mark the attempt BLOCKED in an outside-tree log and stop before any further tag or publication action. Missing or deleted evidence also blocks progress. If recording fails after a tag push or publication, report the actual public state and the incomplete gate; do not claim all gates passed or rewrite the tag. No comments are posted by preparing these documents.

## Evidence gates

Each gate is independent evidence. A gate that has not been run is recorded as NOT RUN - never as pass. Local tests, independent review, remote CI, human approval, the tag, the GitHub Release, and the install check are separate; none substitutes for another. **A green result on any commit other than the exact final candidate SHA does not satisfy any gate** - not the PR #5 audits, not the 10/10 CI runs on `1658865` or `a565768`.

| Gate | Required evidence |
| --- | --- |
| SOURCE_VALIDATED | Preflight complete: origin fetched, HEAD matches origin/main, tree clean including untracked files, candidate commit recorded explicitly |
| LOCAL_TESTS_VALIDATED | On the exact candidate commit: `go test -count=1 ./...`, `go test -shuffle=on -count=1 ./...`, `go test -race -shuffle=on -count=1 ./...`, `go vet ./...`, `go build ./...` all pass, and the diff check is clean |
| INDEPENDENT_REVIEW_VALIDATED | Independent code and security review of the exact candidate commit: 0 blocker, 0 major; minor findings resolved or explicitly human-approved and recorded; source unchanged; external records verified |
| REMOTE_CI_VALIDATED | CI on the exact candidate SHA (not a branch name): all 10 jobs green — the six Test matrix jobs (ubuntu-24.04, macos-15, windows-2025 x Go 1.26.x, 1.27.x), both Race jobs, and both CLI smoke jobs |
| RELEASE_APPROVED | A human explicitly approves publication of v0.5.0 (tag push and release), after reviewing the gates above; date, person, and reference recorded. The MINOR-4 Windows exception approval is not this approval |
| TAG_VERIFIED | Annotated tag `v0.5.0` newly created (absent locally and remotely beforehand), tag object type `tag`, peeled commit equals the candidate commit, pushed as a single ref; remote object and peeled commit verified |
| RELEASE_PUBLISHED | Draft release reviewed, prerelease off, Latest chosen deliberately, published by a human, public state verified |
| MODULE_INSTALL_VERIFIED | After publication: proxy metadata and checksum record visible; isolated `go install` at `@v0.5.0` succeeds; `mandala --version` prints `mandala v0.5.0` |

Run Steps 1-8 in order. Steps 1-4 must be PASS in live records before Step 5 approval. Steps 6-7 require that approval. Step 8 requires publication; never require its PASS before publication or report it early. All eight results belong in external PR #6 comments for the same candidate.

## Step 1 - Preflight (SOURCE_VALIDATED)

Start a fresh Bash shell for each candidate attempt. Confirm on GitHub that PR #6 is merged and that the intended main commit includes its reviewed changes. If the current checkout has unrelated modifications or untracked files such as `.serena/`, preserve them. Use a separate clean clone outside the current checkout instead of deleting files or changing ignore rules:

```sh
RELEASE_CHECKOUT_ROOT="$(mktemp -d)" || exit 1
git clone --branch main https://github.com/cottondesu/mandala.git "$RELEASE_CHECKOUT_ROOT/mandala" || exit 1
cd "$RELEASE_CHECKOUT_ROOT/mandala" || exit 1
```

Record the absolute clone path in an outside-tree log for resuming later. Run the following in Bash. The capture explicitly starts Bash, so its error handling also works if pasted from zsh. A failed query or guard stops before candidate assignment:

```sh
CANDIDATE_SHA="$(bash -c '
  set -euo pipefail
  git fetch origin
  current_head="$(git rev-parse HEAD)"
  main_head="$(git rev-parse origin/main)"
  worktree_status="$(git status --porcelain --untracked-files=all)"
  [ "$current_head" = "$main_head" ]
  [ -z "$worktree_status" ]
  [[ "$current_head" =~ ^[0-9a-f]{40}$ ]]
  printf "%s\n" "$current_head"
')" || { echo 'STOP: source preflight failed' >&2; exit 1; }
readonly CANDIDATE_SHA
printf 'Candidate SHA: %s\n' "$CANDIDATE_SHA"
```

Review the printed full SHA, then post the SOURCE_VALIDATED record to merged PR #6 and read it back. Store its actual comment URL in `SOURCE_EVIDENCE_URL` in your shell and in the external record chain. Do not continue until the record is accessible. Do not write the SHA into the tracked snapshot.

If a Git query fails, the verdict is UNKNOWN, never clean or matching by default. If HEAD differs from fetched origin/main, stop and re-evaluate the candidate. Do not merge or reset as an automatic recovery. A new candidate requires fresh gates and approval.

Use the same candidate in every later step. When resuming in a new shell, restore `CANDIDATE_SHA` from the verified SOURCE_VALIDATED comment, mark it readonly, and compare HEAD and freshly fetched origin/main to it with a clean-tree check. Never re-derive a replacement from current HEAD. Keep generated files and logs outside the clone. Recheck candidate identity and clean status before and after each gate; stop if source changes or fetched main advances. Human approval is separately required at Step 5.

## Step 2 - Local validation (LOCAL_TESTS_VALIDATED)

Run on the exact candidate commit. The snippet is bash (macOS/Linux) in a `set -euo pipefail` subshell, so the first failing command stops the block and no earlier failure can be hidden by a later exit 0. On Windows, run the same checks in Git Bash, or run the five `go` commands plus `git diff --check` and `git status --porcelain` individually in PowerShell. Each must exit 0, status output must be empty, and HEAD must equal the externally restored candidate SHA before and after the checks. General PowerShell patterns appear in [INSTALLATION.md](INSTALLATION.md).

```sh
(
  set -euo pipefail
  [ -n "$CANDIDATE_SHA" ]
  current_head="$(git rev-parse HEAD)"
  worktree_status="$(git status --porcelain --untracked-files=all)"
  [ "$current_head" = "$CANDIDATE_SHA" ]
  [ -z "$worktree_status" ]
  go version
  go test -count=1 ./...
  go test -shuffle=on -count=1 ./...
  go test -race -shuffle=on -count=1 ./...
  go vet ./...
  go build ./...
  git diff --check
  current_head="$(git rev-parse HEAD)"
  worktree_status="$(git status --porcelain --untracked-files=all)"
  [ "$current_head" = "$CANDIDATE_SHA" ]
  [ -z "$worktree_status" ]
)
```

The five Go commands must each pass on their own. `git diff --check` checks whitespace errors and conflict markers introduced in a diff; it does not prove a clean checkout. The separate status guard rejects modifications and untracked files. Failed Git queries stop the block. When both Go 1.26 and 1.27 toolchains are available locally, run the block under each and record exact versions, command results, and the unchanged candidate SHA in an external PR #6 comment. CI covers both remotely in any case. Never edit tracked documents to record results.

## Step 3 - Independent review (INDEPENDENT_REVIEW_VALIDATED)

An independent code and security review of the **exact candidate SHA** must report **0 blocker and 0 major** findings. The reviewer verifies the full SHA, unchanged clean source tree, and accessible external evidence, then records the target, findings, date, identity, and supporting references in a new PR #6 comment. Minor findings must be resolved or explicitly human-approved and recorded. PR #5 and PR #6 preparation audits are historical and do not carry over. If a source fix is necessary, stop this attempt and restart at Step 1 on the reviewed, merged replacement candidate. Do not append the final review to a tracked document.

## Step 4 - Remote CI (REMOTE_CI_VALIDATED)

Open the GitHub Actions CI run triggered by a `push` to `main`, with `headSha` equal to the exact candidate SHA. A pull-request run on a synthetic merge ref does not count. Required green jobs on that SHA:

- all six Test matrix jobs (ubuntu-24.04, macos-15, windows-2025 x Go 1.26.x, 1.27.x),
- both Race jobs,
- both CLI smoke jobs.

That is 10 of 10 jobs. Record the run URL, run ID, full commit SHA, completed status, overall conclusion, and each job's completed status and success conclusion in an external PR #6 comment. Green runs on other SHAs do not count, including PR #5 runs 40158802643 and 40191966945 and PR #6 run 40197731240 on `0c4c0868`. Those are history only. This gate needs a completed successful run on the selected post-merge candidate.

## Step 5 - Release approval (RELEASE_APPROVED)

Before any tag or release action, a human reviews the accessible external PASS records for Steps 1-4 and the approved limitations. The human posts an explicit approval comment on merged PR #6, with UTC date, identity, links to those records, and these decisions:

```text
Release: v0.5.0
Candidate SHA: <full 40-character candidate SHA>
Decision: APPROVED FOR RELEASE
Actions authorized:
- Annotated tag creation and push
- GitHub Release publication
```

Read back the comment and retain its URL externally. Do not write approval into the source tree. This template is not approval and is not posted during documentation preparation.

- Approval must name v0.5.0 and cover both actions that follow: pushing the tag and publishing the GitHub Release.
- The MINOR-4 Windows exception approval is **not** a release approval; its own text ends with "RELEASE: Not authorized by this approval".
- If any gate above is missing, NOT RUN, BLOCKED, or failed, approval must not be requested or granted. Approval for a different SHA is invalid.

## Step 6 - Tag (TAG_VERIFIED)

Existing tags are immutable: never delete, recreate, or force-push any tag - this protects v0.1.0 through v0.4.1 alike. If a release is wrong, the remedy is a new patch version, not tag rewriting.

Confirm the tag is absent locally and remotely. These are read-only diagnostics - no tag is created and nothing is pushed here; tag creation remains a separately human-authorized step. Each query runs as a standalone command inside a `set -euo pipefail` subshell, so if either query itself fails (for example a network or authentication error) the block stops and the absence verdict is UNKNOWN - a failed query is never treated as absence:

```sh
(
  set -euo pipefail
  local_tags="$(git tag --list v0.5.0)"
  remote_tags="$(git ls-remote --tags origin refs/tags/v0.5.0)"
  if [ -n "$local_tags" ]; then
    echo 'STOP: local tag v0.5.0 already exists' >&2
    exit 1
  fi
  if [ -n "$remote_tags" ]; then
    echo 'STOP: remote tag v0.5.0 already exists' >&2
    exit 1
  fi
  echo 'Tag v0.5.0 absent locally and remotely.'
)
```

After verifying the Step 5 approval for this candidate, fetch origin again immediately before tag creation. Keep the guards and creation in the same fail-closed block. A failed query or guard stops before creation:

```sh
(
  set -euo pipefail
  [ -n "$CANDIDATE_SHA" ]
  git fetch origin
  current_head="$(git rev-parse HEAD)"
  main_head="$(git rev-parse origin/main)"
  worktree_status="$(git status --porcelain --untracked-files=all)"
  local_tags="$(git tag --list v0.5.0)"
  remote_tags="$(git ls-remote --tags origin refs/tags/v0.5.0)"
  [ "$current_head" = "$CANDIDATE_SHA" ]
  [ "$main_head" = "$CANDIDATE_SHA" ]
  [ -z "$worktree_status" ]
  [ -z "$local_tags" ]
  [ -z "$remote_tags" ]
  git tag -a v0.5.0 -m 'mandala v0.5.0 (feature: atomic batch operations and deterministic dry-run)' "$CANDIDATE_SHA"
)
```

If any check fails or main has advanced, STOP and re-evaluate the attempt. Do not replace `CANDIDATE_SHA`. Never recreate an existing tag, even if it targets the intended commit.

Verify the tag object and target with executable guards, not by eyeballing output. The type must be `tag` (annotated, not lightweight). The tag object SHA and the peeled commit SHA are two different values. Each query runs as a standalone assignment before any result is tested or printed, so a failing query stops the block under `set -euo pipefail` instead of being hidden inside a substitution or `echo`; the values are printed with `printf` after capture. The `^{commit}` peeling suffix is single-quoted so zsh does not interpret the caret or braces:

```sh
(
  set -euo pipefail
  tag_type="$(git cat-file -t refs/tags/v0.5.0)"
  tag_object_sha="$(git rev-parse refs/tags/v0.5.0)"
  peeled_commit_sha="$(git rev-parse 'refs/tags/v0.5.0^{commit}')"
  [ "$tag_type" = 'tag' ]
  printf 'tag object SHA: %s\n' "$tag_object_sha"
  printf 'peeled commit SHA: %s\n' "$peeled_commit_sha"
  [ "$peeled_commit_sha" = "$CANDIDATE_SHA" ]
  echo 'Annotated tag verified at the recorded candidate commit.'
)
```

If any guard fails, stop - do not move, recreate, or force the tag.

Immediately before pushing, re-read the approval and required evidence, then fetch and recheck source identity, cleanliness, tag type and target, and remote absence. Push exactly one ref only if every check passes. Never `--tags`, never `--force`:

```sh
(
  set -euo pipefail
  [ -n "$CANDIDATE_SHA" ]
  git fetch origin
  current_head="$(git rev-parse HEAD)"
  main_head="$(git rev-parse origin/main)"
  worktree_status="$(git status --porcelain --untracked-files=all)"
  tag_type="$(git cat-file -t refs/tags/v0.5.0)"
  peeled_commit_sha="$(git rev-parse 'refs/tags/v0.5.0^{commit}')"
  remote_tags="$(git ls-remote --tags origin refs/tags/v0.5.0)"
  [ "$current_head" = "$CANDIDATE_SHA" ]
  [ "$main_head" = "$CANDIDATE_SHA" ]
  [ -z "$worktree_status" ]
  [ "$tag_type" = 'tag' ]
  [ "$peeled_commit_sha" = "$CANDIDATE_SHA" ]
  [ -z "$remote_tags" ]
  git push origin 'refs/tags/v0.5.0'
)
```

Verify the remote tag before recording TAG_VERIFIED as PASS or creating a GitHub Release:

```sh
(
  set -euo pipefail
  tag_object_sha="$(git rev-parse refs/tags/v0.5.0)"
  remote_tags="$(git ls-remote --tags origin refs/tags/v0.5.0 'refs/tags/v0.5.0^{}')"
  remote_object_sha="$(printf '%s\n' "$remote_tags" | awk '$2 == "refs/tags/v0.5.0" {print $1}')"
  remote_commit_sha="$(printf '%s\n' "$remote_tags" | awk '$2 == "refs/tags/v0.5.0^{}" {print $1}')"
  [ "$remote_object_sha" = "$tag_object_sha" ]
  [ "$remote_commit_sha" = "$CANDIDATE_SHA" ]
  printf 'Remote tag object: %s\nRemote candidate: %s\n' "$remote_object_sha" "$remote_commit_sha"
)
```

Record the local and remote tag object SHAs, peeled candidate SHA, checks, and single-ref push outcome in a new external comment. Fresh fetches detect observed main changes; they cannot atomically prevent a remote update immediately after a check. Recheck before proceeding. If a later check detects main advancement or tag mismatch, stop and seek human re-evaluation. Preserve any created or pushed tag; never delete or move it to recover. A wrong published release requires a new version.

## Step 7 - GitHub Release (RELEASE_PUBLISHED)

1. Verify Steps 1-6 are PASS in accessible external records for this candidate, including human approval. Recheck HEAD, freshly fetched origin/main, clean status, and the remote tag target. On GitHub, create a **draft** release targeting the existing verified tag `v0.5.0`.
2. Copy the feature content from [the repository draft](release-notes-v0.5.0-draft.md) into the **GitHub Release description**. Leave the repository draft unchanged. Edit only the GitHub description to state the full candidate SHA, actual Steps 1-6 results, approval and MINOR-4 references, known limits, and specific external evidence comment URLs. Mark publication and install verification pending until performed. Do not copy the snapshot's draft-time status as current status. The feature content remains: atomic batch operations and deterministic preview, strict bounded JSON input, the new `batch` help row, existing schemas and exit codes preserved, state schema `1`, `batch_result_version: 1`, no new dependencies, and a source-only release.
3. Set **pre-release: off** (feature release).
4. Choose **Latest** deliberately; it is a human decision, not a default.
5. A human reviews the completed GitHub description and its evidence links, confirms the candidate approval, then clicks **Publish**.
6. Verify the public Release has tag name `v0.5.0`, is live rather than draft, and is not a prerelease. Repeat the remote tag object and peel checks from Step 6 against the candidate; a branch-like Release target label alone is insufficient. The only assets are GitHub's auto-generated source archives. Never attach or promise binary assets. Record the Release URL, tag object, candidate SHA, public state, and decision in a new PR #6 comment. A failed verification blocks completion.

Completing the GitHub description never requires a source commit. The Release can link to PR #6 while Step 8 is pending, then link its final install-evidence comment after Step 8.

## Step 8 - Module install verification (MODULE_INSTALL_VERIFIED)

Only after the tag and release are actually published.

Read-only distribution checks:

- Proxy metadata: https://proxy.golang.org/github.com/cottondesu/mandala/@v/v0.5.0.info
- Checksum record: https://sum.golang.org/lookup/github.com/cottondesu/mandala@v0.5.0

Isolated install with failure guards and an exact version assertion (macOS/Linux):

```sh
(
  set -euo pipefail
  VERIFY_GOBIN="$(mktemp -d)"
  trap 'rm -rf -- "$VERIFY_GOBIN"' EXIT
  GOBIN="$VERIFY_GOBIN" go install github.com/cottondesu/mandala/cmd/mandala@v0.5.0
  actual="$("$VERIFY_GOBIN/mandala" --version)"
  [ "$actual" = 'mandala v0.5.0' ]
  printf '%s\n' "$actual"
)
```

`set -euo pipefail` runs the binary only after a successful isolated install, and the equality guard asserts the exact version line `mandala v0.5.0` (command substitution strips the trailing newline, so the comparison is against the line itself). The check never runs an existing user binary: it uses only the freshly installed temporary copy, and the `EXIT` trap removes the temporary directory on both success and failure. For the Windows equivalent, adapt the isolated pattern from [INSTALLATION.md](INSTALLATION.md) with `@v0.5.0` (only after v0.5.0 is actually published; the INSTALLATION example pins `@v0.4.1`, the newest published release) and `mandala.exe`, compare the emitted version line to `mandala v0.5.0` the same way, and restore the original `GOBIN` environment in `finally`. This install check remains an independent gate.

If the install fails with `404` or `unknown revision`, follow the ordered diagnostics in [INSTALLATION.md](INSTALLATION.md). Do not delete, recreate, or force-move the tag to force the proxy to update.

Record proxy and checksum lookup results, exact Go version, install command and exit status, exact version output, candidate SHA, and Release URL in a new PR #6 comment. Record FAIL or BLOCKED honestly if distribution or evidence recording is incomplete; publication alone is not all eight gates passing. Verify the source checkout remains clean at the same SHA. The GitHub Release description may link the final evidence comment and update its pending status after verification, without editing repository documents, changing the candidate, or moving the tag.

Platform-specific installation and `PATH` details: [INSTALLATION.md](INSTALLATION.md).
