# mandala v0.5.0 release notes (DRAFT — not yet published)

Status: **draft for the v0.5.0 candidate.** v0.5.0 has not been tagged, released, or published. This note is written before publication; the baseline is v0.4.1 (commit `bdff5992e2935d1a55919ec58e111cdaa824f7fc`). Release execution follows [RELEASING.md](RELEASING.md); nothing below implies any release step has run.

## Scope

One feature theme: atomic batch operations for human and coding-agent workflows, with four capabilities:

1. **Atomic batch operations** — `mandala batch --file <file|->` applies a JSON document of `add` and `mark` operations in one all-or-nothing step on the existing single state.
2. **Dry-run** — `--dry-run` runs the identical validation, apply, and final validation path but never writes. `state.json` keeps its bytes and mtime; no temporary files are created.
3. **Deterministic change preview** — text and JSON previews show the final per-cell diff (ID-ascending), not the operation list. Same input, same bytes, across runs and platforms. A semantic no-op reports zero changes and rewrites nothing.
4. **JSON file / stdin input** — `--file` reads a regular file; `--file -` reads stdin with the same limits: 65,536 bytes max, 1–128 operations, one JSON document.

## Compatibility

- No changes to existing commands, flags, or exit-code meanings; the root help command list gains a single `batch` row; output schemas (`status --json`, `gaps --json`, `show --json`) are unchanged.
- State-file schema stays `1`; saves reuse the existing validate → temp file → sync → rename path. `batch` adds a separate result schema `batch_result_version: 1`.
- No new dependencies; Go standard library only; offline.
- Existing published tags are untouched.

## Strict input contract (summary)

`{"batch_version":1,"operations":[...]}` with `add` (`op`, `id`, optional boolean `optional`) and `mark` (`op`, `id`, `status` ∈ `open|done|na`). Unknown or duplicate keys (including escape-collision duplicates), `null`, wrong types, non-integer or non-`1` versions, trailing JSON, invalid UTF-8, BOM, oversized input, and non-regular files are rejected before any state is read. Operations apply in array order and reuse the single-cell rules (ID grammar, two levels, 8 roots, 8 children per root, 72 cells, optional inheritance, expansion semantics, duplicate-`add` rejection).

## Guarantees and non-guarantees

- Atomic within one CLI run: every operation succeeds and the final state validates, or nothing is written. Input, operation, validation, and save failures exit `2` with empty stdout and an unchanged state file. A save failure never emits success JSON.
- `batch` never exits `1`; required gaps remaining after a successful batch still exit `0`.
- Not provided, by design: multi-process locking, lost-update prevention between dry-run and apply, exactly-once execution, and power-loss durability beyond the existing save path. Single writer remains the caller's responsibility.
- With `--json` on a real apply, output is printed only after a successful save. If writing stdout fails after the save, the saved state is not rolled back and partial JSON may remain on stdout. A write error returned to the CLI reports `E_IO` with exit `2`. Exit `2` alone does not prove that the batch was not applied. Do not retry automatically. Check the current state with `show --json` before deciding whether to retry. Exactly-once execution is not guaranteed. Re-running a batch is safe but not result-identical: repeating an `add` whose cell now exists fails atomically with `E_DUPLICATE_ID` (nothing applied), while re-running `mark`-only operations converges to a semantic no-op (`changed:false`, no save).
- `--file -` is the recommended way to read a batch from standard input. `--file` follows symlinks and accepts special paths such as `/dev/stdin` only when they resolve to a regular file; on macOS, `/dev/stdin` and `/dev/fd/N` are fdesc nodes that are always rejected — use `--file -` there. FIFOs and directories are rejected with `E_IO` before opening. An external process that can write to the input location may still swap the file between the pre-open check and the open (for example with a FIFO, which can block); run batch only with input locations you trust.

## Evidence at draft time

### Historical evidence (verified)

- Implementation: PR [#5](https://github.com/cottondesu/mandala/pull/5) `feat: add atomic batch operations and deterministic dry-run`, **merged** to `main` on 2026-10-25 (merge commit `a56576891ec7006c3ae65609f48763dbc1245fe7`).
- PR commits: feature implementation `2a680b9c1b26e44203792239d903acc70de9614e`; Windows CRLF test fix `1658865157009d13bfda1a4d315411629dac073c` (PR HEAD).
- Independent audit (Claude) of the implementation across review rounds: final state **0 BLOCKER / 0 MAJOR**; MINOR-4 recorded as a human-approved exception. The Windows CRLF fix was audited and passed.
- Remote CI on the PR HEAD SHA `1658865`: run [40158802643](https://github.com/cottondesu/mandala/actions/runs/40158802643) — **10/10 jobs success**.
- Remote CI on the main merge SHA `a565768`: run [40191966945](https://github.com/cottondesu/mandala/actions/runs/40191966945) — **10/10 jobs success**.
- MINOR-4 (Windows skip of `TestBatchSaveFailure`): human exception **approved for v0.5.0 only** ([comment](https://github.com/cottondesu/mandala/pull/5#issuecomment-6397343300), 2026-10-25). That approval explicitly does **not** authorize the release itself.
- Local (pre-merge): `go test -count=1 ./...`, `go test -shuffle=on -count=1 ./...`, `go test -race -shuffle=on -count=1 ./...`, `go vet ./...`, `go build ./...`, `git diff --check` all pass on the candidate tree with both the Go 1.26.10 and 1.27 toolchains; mutation checks (the 7 specified defects plus the 5 audit-round-2 survivors are each caught by the batch tests); CI smoke gained byte-exact batch cases with `actionlint` clean.

### Final-candidate gates (not yet run)

The evidence above is historical: it covers PR #5's commits and the merge SHA, not the final release candidate. Merging the release-preparation PR creates a new `main` SHA, and no CI has run on that SHA yet. Per [RELEASING.md](RELEASING.md), every gate — local tests, independent review, remote CI, tag, publication, install — must be re-run and recorded against the exact final candidate SHA; a green run on any earlier SHA does not carry over.
