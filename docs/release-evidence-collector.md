# release-evidence collector

`cmd/release-evidence` is a cross-platform, Go-stdlib-only command that
collects **local** release-gate evidence for one exact candidate commit of
`mandala`. It automates the command set of the `LOCAL_TESTS_VALIDATED` gate
(Step 2 of [RELEASING.md](RELEASING.md)) and writes machine- and human-readable
records to a fresh directory outside the source tree. It is an evidence
**input** producer only — see [Scope and limits](#scope-and-limits).

## Usage

```sh
go run ./cmd/release-evidence \
  -output /tmp/mandala-v051-evidence/run-1 \
  -candidate <full 40-character commit SHA> \
  -expected-go-minor 1.27
```

Exit code: `0` only when `release_status` is `PASS`; `1` for `BLOCKED` or any
collection error; `130` when interrupted by SIGINT/SIGTERM.

### Flags

| Flag | Meaning |
| --- | --- |
| `-output DIR` | Required. A **new, never-used** directory **outside the source root**. The parent directory must already exist. The parent and source root are resolved through symlinks before the final component is created with `os.Mkdir`, so a rerun to the same path fails instead of overwriting earlier evidence. |
| `-candidate SHA` | Required. The expected full git commit SHA (40 or 64 hex characters). |
| `-expected-go-minor V` | Required in practice for a PASS: the Go minor version (e.g. `1.27`) that the selected toolchain must report. |
| `-go PATH` | Go toolchain to interrogate and use (default `go`). Both `go version` and `go env GOVERSION GOOS GOARCH` are run against it and recorded. |
| `-allow-dirty` | Development mode. Collect even when the tree is dirty. The release status stays **BLOCKED** and no exact-candidate claim is made, regardless of command results. |

## What it runs

The collector runs this fixed command set without a shell. Commands inherit
the normal process environment, including Go settings and executable lookup. `argv[0] == "go"` is replaced by the `-go` path.

1. `go test -count=1 ./...`
2. `go test -shuffle=on -count=1 ./...`
3. `go test -race -shuffle=on -count=1 ./...`
4. `go vet ./...`
5. `go build ./...`
6. `git diff --check`

Read-only identity queries (also whitelisted): `git rev-parse --show-toplevel`,
`git rev-parse HEAD`, `git status --porcelain --untracked-files=all`,
`<go> version`, `<go> env GOVERSION GOOS GOARCH`. The root discovery query
runs before the output directory exists and is the one exception to raw capture;
its resulting root is recorded in the manifest.

## Preflight (fail-closed)

Gate commands run only when all hold; otherwise every command is recorded
`NOT_RUN` and the run is `BLOCKED`:

- `git rev-parse HEAD` succeeds and equals `-candidate` exactly;
- the tree is clean per `git status --porcelain --untracked-files=all`
  (a dirty tree may only be collected with `-allow-dirty`, which is
  development evidence and always `BLOCKED`);
- the toolchain queries succeed, agree on the version and platform, and
  `GOVERSION` reports the required `-expected-go-minor`. Missing or malformed
  values block the run.

Only successful, clean initial and final snapshots matching the candidate permit
an exact-candidate claim. A dirty source is never claimed as the exact candidate
(`claimed_exact_candidate: false` in the manifest).

## Recorded evidence

For every gate command and source/toolchain identity query: `argv`, status, exit code, launch error (a failure to
launch the binary at all is kept distinct from a real exit code), start/end
UTC, and raw stdout/stderr bytes saved as separate files
`NN-<slug>.stdout` / `NN-<slug>.stderr` with SHA-256 hashes and byte counts
listed in both `manifest.json` and `summary.md`.

`manifest.json` (schema `1`) additionally carries: collector argv, start/finish
UTC, repo root, output dir, candidate SHA, `-allow-dirty` flag, source
snapshots **before and after** (full HEAD + porcelain status + dirty flag +
query errors), `claimed_exact_candidate`, toolchain details (version output,
GOVERSION/GOOS/GOARCH, expected vs. actual minor), gate-command records and ordered identity-query records,
interrupted flag, release status, blocked reasons, and disclaimers.
`summary.md` is the human-readable rendering of the same facts.

The manifest is persisted **atomically** (temp file + fsync + rename)
initially, before and after every command, and at the end; the derived summary
is refreshed at each persistence point. A hard kill between the two writes
can leave the summary one update behind the manifest; during collection it
remains `RUNNING`, never a stale `PASS`.

## Statuses and verdicts

Per-command: `PASS`, `FAIL`, `NOT_RUN`, `RUNNING`, `INTERRUPTED`,
`LAUNCH_ERROR`. Evaluation is fail-fast: the first `FAIL` or `LAUNCH_ERROR`
stops the sequence; later commands stay `NOT_RUN`. The final source check is
collected even after a command failure, and HEAD movement or a tree that became
dirty during collection blocks the run.

Overall: `release_status` is `PASS` only when the source was clean at the exact
candidate before and after, the toolchain minor matched, and every command
finished `PASS`. Otherwise it is `BLOCKED` with explicit reasons. `RUNNING`
and `INTERRUPTED` are never `PASS`.

## Signals

SIGINT/SIGTERM (where available) cancel the in-flight command: it is recorded
`INTERRUPTED`, remaining commands `NOT_RUN`, and the manifest + summary are
persisted before exiting `130`. A hard kill leaves the last persisted state —
the in-flight command `RUNNING` — which is likewise never `PASS`.

Descendants may outlive the canceled direct child. If they keep output pipes
open, captured logs end after a bounded one-second drain; partial output already
captured is retained.

## Scope and limits

- Local evidence input for `LOCAL_TESTS_VALIDATED` only. This tool **does not
  assert** remote CI (`REMOTE_CI_VALIDATED`), Windows results beyond what a
  local Windows run shows (`RELEASING.md` Step 2 notes the Git Bash /
  PowerShell equivalents), independent review (`INDEPENDENT_REVIEW_VALIDATED`),
  release approval (`RELEASE_APPROVED`), tagging, or publication.
- The collector never edits, cleans, or commits anything in the source tree,
  never writes into `.mandala` or other repo state, and never posts comments —
  recording evidence in PR comments remains a human step per RELEASING.md.
- Output lives outside the source root; keep logs out of every release
  checkout, as RELEASING.md requires.

## Mapping to RELEASING.md live-record fields

- `Release version` — not asserted by the tool; supplied by the human record.
- `Candidate SHA` — `candidate_sha`; `claimed_exact_candidate` states whether
  local evidence may attribute results to it.
- `Gate name` — `LOCAL_TESTS_VALIDATED` (this tool's scope).
- `Gate status` — `release_status` (`PASS` / `BLOCKED`); per-command statuses
  give the `PASS` / `FAIL` / `NOT RUN` detail.
- `Execution date (UTC)` — `started_utc` / `finished_utc` and per-command
  `start_utc` / `end_utc`.
- `Command output or evidence reference` — the raw capture files plus their
  SHA-256 hashes in `manifest.json` / `summary.md`.
- `Remaining blockers` — `blocked_reasons`.
- Executor identity, evidence URLs, approvals, and previous-comment links are
  human-recorded fields outside this tool's scope.

## Tests

`cmd/release-evidence` is covered by focused unit tests using an injected
scripted runner (no recursive full-suite runs): pass, fail-fast `NOT_RUN`
propagation, dirty tree with and without `-allow-dirty`, moved SHA, wrong Go
minor, launch-error vs. exit-code distinction, mid-run `RUNNING` persistence
and interruption handling, output-directory collision rejection, and the exact
gate command set.
