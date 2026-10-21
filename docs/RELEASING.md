# Releasing mandala

Manual, human-controlled, source-only releases. This document is a procedure: **none of the publication commands below were executed as part of writing it**, and nothing in the repository auto-releases. A human performs each step and records the evidence.

Current candidate: **v0.4.1** (not yet published). Baseline: v0.4.0 at commit 2fd15fb1ae32d0e13b6d9a1ed8107eb3f3a9deb9. Candidate scope: CI and documentation only - no new features (nothing planned for v0.5.0), CLI behavior and state schema unchanged. Confirm the actual current tag state before starting; if any fact below is stale, stop and update this document first.

## Evidence gates

Each gate is independent evidence. A gate that has not been run is recorded as NOT RUN - never as pass. Local tests, remote CI, the tag, the GitHub Release, and the install check are separate; none substitutes for another.

| Gate | Required evidence |
| --- | --- |
| SOURCE_VALIDATED | Preflight complete: origin fetched, HEAD matches origin/main, tree clean including untracked files, candidate commit recorded explicitly |
| LOCAL_TESTS_VALIDATED | On the candidate commit: `go test -count=1 ./...`, `go test -race -shuffle=on -count=1 ./...`, `go vet ./...`, `go build ./...` all pass, and the diff check is clean |
| INDEPENDENT_REVIEW_VALIDATED | Independent code and security review of the exact candidate commit: 0 blocker, 0 major |
| REMOTE_CI_VALIDATED | CI on the exact candidate SHA (not a branch name): all 6 test-matrix jobs, both race jobs, and the smoke jobs green |
| TAG_VERIFIED | Annotated tag `v0.4.1` newly created (absent locally and remotely beforehand), tag object type `tag`, peeled commit equals the candidate commit, pushed as a single ref |
| RELEASE_PUBLISHED | Draft release reviewed, prerelease off, Latest chosen deliberately, published by a human, public state verified |
| MODULE_INSTALL_VERIFIED | After publication: proxy metadata and checksum record visible; isolated `go install` at `@v0.4.1` succeeds; `mandala --version` prints `mandala v0.4.1` |

Record for each gate: date, person, and evidence (log, URL, or run ID).

## Step 1 - Preflight (SOURCE_VALIDATED)

```sh
(
  set -euo pipefail
  git fetch origin
  current_head="$(git rev-parse HEAD)"
  main_head="$(git rev-parse origin/main)"
  worktree_status="$(git status --porcelain)"
  [ "$current_head" = "$main_head" ]
  [ -z "$worktree_status" ]
  echo "HEAD: $current_head"
  echo "origin/main: $main_head"
)
```

- Every Git query runs as a standalone assignment before its result is tested, under `set -euo pipefail`: a failed fetch, rev-parse, or status query stops the subshell and the preflight verdict is UNKNOWN - a query error is never mistaken for a clean tree or a matching SHA. The guards assert that HEAD equals `origin/main` and that the status query produced no output (clean, including untracked files); both SHAs are then printed for review.
- `git rev-parse HEAD` and `git rev-parse origin/main` must print the same full SHA. That SHA is the explicit candidate commit. After the preflight succeeds, derive it and record it in your shell, then review the printed value before continuing - it must be the full 40-hex-digit commit you intend to release:

```sh
CANDIDATE_SHA="$(git rev-parse HEAD)"
printf '%s\n' "$CANDIDATE_SHA"
```

If HEAD differs from origin/main, stop: merging to main requires explicit human approval first. Human approval is separately required before tag publication and before publishing the release.

## Step 2 - Local validation (LOCAL_TESTS_VALIDATED)

Run on the candidate commit. The snippet is bash (macOS/Linux) in a `set -euo pipefail` subshell, so the first failing command stops the block and no earlier failure can be hidden by a later exit 0. On Windows, run the same checks in Git Bash, or run the four `go` commands plus `git diff --check` and `git status --porcelain` individually in PowerShell - each must exit 0 and the status output must be empty; general PowerShell patterns appear in [INSTALLATION.md](INSTALLATION.md).

```sh
(
  set -euo pipefail
  go test -count=1 ./...
  go test -race -shuffle=on -count=1 ./...
  go vet ./...
  go build ./...
  git diff --check
  worktree_status="$(git status --porcelain)"
  [ -z "$worktree_status" ]
)
```

The four Go commands must each pass on their own. `git diff --check` fails on conflict markers and whitespace damage. The status query is captured by a standalone assignment before its result is tested, so a failed query stops the block under `set -euo pipefail` rather than implying a clean tree; the guard then fails if the captured status is nonempty (modified or untracked files). The clean-status check is separate evidence from the diff check.

## Step 3 - Independent review (INDEPENDENT_REVIEW_VALIDATED)

An independent code and security review of the exact candidate SHA must report **0 blocker and 0 major** findings. Record the review reference (link, ID, date). If findings exist, resolve them and restart validation from Step 1 on the new commit.

## Step 4 - Remote CI (REMOTE_CI_VALIDATED)

Open the GitHub Actions runs for the exact candidate SHA. Required green jobs on that SHA:

- all six Test matrix jobs (ubuntu-24.04, macos-15, windows-2025 x Go 1.26.x, 1.27.x),
- both Race jobs,
- the CLI smoke jobs.

Green runs on any other SHA do not count.

## Step 5 - Tag (TAG_VERIFIED)

Existing tags are immutable: never delete, recreate, or force-push any tag. If a release is wrong, the remedy is a new patch version, not tag rewriting.

Confirm the tag is absent locally and remotely. These are read-only diagnostics - no tag is created and nothing is pushed here; tag creation remains a separately human-authorized step. Each query runs as a standalone command inside a `set -euo pipefail` subshell, so if either query itself fails (for example a network or authentication error) the block stops and the absence verdict is UNKNOWN - a failed query is never treated as absence:

```sh
(
  set -euo pipefail
  local_tags="$(git tag --list v0.4.1)"
  remote_tags="$(git ls-remote --tags origin refs/tags/v0.4.1)"
  if [ -n "$local_tags" ]; then
    echo 'STOP: local tag v0.4.1 already exists' >&2
    exit 1
  fi
  if [ -n "$remote_tags" ]; then
    echo 'STOP: remote tag v0.4.1 already exists' >&2
    exit 1
  fi
  echo 'Tag v0.4.1 absent locally and remotely.'
)
```

Re-verify the recorded candidate immediately before tagging. Every guard must pass; a nonempty recorded SHA, HEAD and the fetched `origin/main` each equal to it, and a clean worktree (including untracked files) are required. The status query is a standalone assignment tested afterwards, so a query error fails the block rather than implying a clean tree:

```sh
(
  set -euo pipefail
  [ -n "$CANDIDATE_SHA" ]
  [ "$(git rev-parse HEAD)" = "$CANDIDATE_SHA" ]
  [ "$(git rev-parse origin/main)" = "$CANDIDATE_SHA" ]
  worktree_status="$(git status --porcelain)"
  [ -z "$worktree_status" ]
  echo 'Candidate re-verified: HEAD and origin/main equal the recorded SHA; worktree clean.'
)
```

Create an annotated tag on the explicit intended commit, after human authorization:

```sh
git tag -a v0.4.1 -m 'mandala v0.4.1 (maintenance: CI and documentation)' "$CANDIDATE_SHA"
```

Verify the tag object and target with executable guards, not by eyeballing output. The type must be `tag` (annotated, not lightweight). The tag object SHA and the peeled commit SHA are two different values. Each query runs as a standalone assignment before any result is tested or printed, so a failing query stops the block under `set -euo pipefail` instead of being hidden inside a substitution or `echo`; the values are printed with `printf` after capture:

```sh
(
  set -euo pipefail
  tag_type="$(git cat-file -t refs/tags/v0.4.1)"
  tag_object_sha="$(git rev-parse refs/tags/v0.4.1)"
  peeled_commit_sha="$(git rev-parse 'refs/tags/v0.4.1^{commit}')"
  [ "$tag_type" = 'tag' ]
  printf 'tag object SHA: %s\n' "$tag_object_sha"
  printf 'peeled commit SHA: %s\n' "$peeled_commit_sha"
  [ "$peeled_commit_sha" = "$CANDIDATE_SHA" ]
  echo 'Annotated tag verified at the recorded candidate commit.'
)
```

If any guard fails, stop - do not move, recreate, or force the tag.

Push exactly one tag ref, only after human approval. Never `--tags`, never `--force`:

```sh
git push origin 'refs/tags/v0.4.1'
```

## Step 6 - GitHub Release (RELEASE_PUBLISHED)

1. On GitHub, create a **draft** release targeting the verified tag `v0.4.1`.
2. Review the notes. They state: scope is CI and documentation; the CLI contract and state schema are unchanged from v0.4.0; no new features; source-only release.
3. Set **pre-release: off** (maintenance release).
4. Choose **Latest** deliberately; it is a human decision, not a default.
5. A human clicks **Publish**.
6. Verify the public state: the tag page shows `v0.4.1` on the candidate commit; the release page is live; the only assets are GitHub's auto-generated source archives. Never attach or promise binary assets.

## Step 7 - Module install verification (MODULE_INSTALL_VERIFIED)

Only after the tag and release are actually published.

Read-only distribution checks:

- Proxy metadata: https://proxy.golang.org/github.com/cottondesu/mandala/@v/v0.4.1.info
- Checksum record: https://sum.golang.org/lookup/github.com/cottondesu/mandala@v0.4.1

Isolated install with failure guards and an exact version assertion (macOS/Linux):

```sh
(
  set -euo pipefail
  VERIFY_GOBIN="$(mktemp -d)"
  trap 'rm -rf -- "$VERIFY_GOBIN"' EXIT
  GOBIN="$VERIFY_GOBIN" go install github.com/cottondesu/mandala/cmd/mandala@v0.4.1
  actual="$("$VERIFY_GOBIN/mandala" --version)"
  [ "$actual" = 'mandala v0.4.1' ]
  printf '%s\n' "$actual"
)
```

`set -euo pipefail` runs the binary only after a successful isolated install, and the equality guard asserts the exact version line `mandala v0.4.1` (command substitution strips the trailing newline, so the comparison is against the line itself). The check never runs an existing user binary: it uses only the freshly installed temporary copy, and the `EXIT` trap removes the temporary directory on both success and failure. For the Windows equivalent, adapt the isolated pattern from [INSTALLATION.md](INSTALLATION.md) with `@v0.4.1` (only after v0.4.1 is actually published; the INSTALLATION example pins `@v0.4.0`) and `mandala.exe`, compare the emitted version line to `mandala v0.4.1` the same way, and restore the original `GOBIN` environment in `finally`. This install check remains an independent gate.

If the install fails with `404` or `unknown revision`, follow the ordered diagnostics in [INSTALLATION.md](INSTALLATION.md). Do not delete, recreate, or force-move the tag to force the proxy to update.

Platform-specific installation and `PATH` details: [INSTALLATION.md](INSTALLATION.md).
