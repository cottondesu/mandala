# Mandala

An offline Go CLI for tracking declared coverage gaps in human and coding agent work.

> Tasks tell agents what to do. Cells tell you what they forgot.

Mandala tracks the cells you declare for a goal and reports open required leaves as gaps, in deterministic text or JSON. It works with human workflows and coding agents, but does not generate a complete plan, inspect code, execute tasks, or call an LLM. Zero gaps means only that the required cells you declared are resolved.

## Install

mandala requires Go 1.26 or newer.

Install the published v0.4.1 release:

```sh
go install github.com/cottondesu/mandala/cmd/mandala@v0.4.1
```

Or install the latest published release:

```sh
go install github.com/cottondesu/mandala/cmd/mandala@latest
```

`@v0.4.1` always installs exactly v0.4.1; `@latest` resolves to the newest published release, and what it resolves to changes as releases are published.

A successful `go install` places the binary in `GOBIN` (if set) or the `bin` directory of the first `GOPATH` entry, which may not be on your `PATH`. See [docs/INSTALLATION.md](docs/INSTALLATION.md) for `PATH` setup, stale or duplicate binary resolution, isolated verification, and troubleshooting for `404 Not Found` and checksum errors.

From a local checkout, the baseline supported commands are (macOS and Linux):

```sh
go build -o mandala ./cmd/mandala
go install ./cmd/mandala
```

Windows PowerShell:

```powershell
go build -o mandala.exe .\cmd\mandala
go install .\cmd\mandala
```

Version reporting: `mandala --version` prints the version embedded from the main module. Pseudo-versions are preserved, and build metadata such as `+dirty` is preserved when present. When the embedded version information is absent, empty, or `(devel)`, it prints `mandala (devel)`. A binary built from a local checkout can report a different version depending on the Go toolchain and build context, while `go install` at an exact tag reports that tag. `--version` alone, without a command and without `--project`, needs no project state, no Git repository, and no network access.

## Quick start

```sh
mandala init "Implement OAuth"
mandala add security
mandala add tests
mandala add compatibility
mandala add security.csrf
mandala add security.token-storage
mandala done security.csrf
mandala gaps --required
```

The final command exits `1` and prints the unresolved required leaf IDs:

```text
compatibility
security.token-storage
tests
```

Adding children expands `security`, so its open leaves become the gaps. `done` is shorthand for `mark <id> done`. Use `mandala mark <id> na` for a leaf that does not apply, or `mandala mark <id> open` to reopen it. Reopen a `done` or `na` leaf before expanding it.

## Commands

| Command | Purpose |
| --- | --- |
| `init <goal>` | Create one active state in the current directory |
| `add [--optional] <id>` | Add a cell; children inherit an optional parent's optionality |
| `mark <id> <open\|done\|na>` | Set a leaf's status |
| `done <id>` | Mark a leaf done |
| `status [--json]` | Show cell counts and required gap count |
| `gaps [--required] [--json]` | List open leaf cells |
| `show [--json]` | Show the complete sparse tree |
| `batch --file <file\|-> [--dry-run] [--json]` | Apply a JSON batch of `add`/`mark` operations atomically |
| `clean` | Remove Mandala's valid `state.json` explicitly |

All commands are non-interactive. Place command flags before positional arguments. `--project DIR` is a global flag placed **before** the command, for example `mandala --project ../repo gaps --json`. It accepts a project root, not its `.mandala` directory. Without it, read and update commands resolve symlinks, then search that physical directory and its parents up to the filesystem root. The nearest `.mandala` wins; a broken one is an error rather than a reason to use an ancestor. `init` creates state at the specified directory or the current directory and refuses to create a nested project inside an existing one. For deletion safety, `clean` acts only on the current directory or an explicit `--project DIR`; it does not search parents.

IDs are stable, lowercase ASCII paths such as `security` and `security.csrf`. Each segment starts with `a`–`z`, then contains up to 63 lowercase letters, digits, or hyphens. v0.1 allows at most eight root cells and eight children per root cell: at most 72 cells in two levels. IDs cannot be renamed by the CLI.

## Output and exit codes

stdout contains requested results; stderr contains errors. Mutating commands produce no stdout on success. JSON mode produces one valid JSON object and a newline, with no progress text. `gaps` includes optional open cells by default, marking them `[optional]` in text; `--required` filters them out. Optional gaps never cause exit `1`.

| Exit | Meaning |
| --- | --- |
| `0` | Success; for `status` and `gaps`, no required gap |
| `1` | `status` or `gaps` found at least one required gap |
| `2` | Usage, project, state, or I/O error; stdout is empty unless writing output fails |

`status --json` emits compact JSON with exactly one trailing newline, including all zero-valued counts. `--json` is a command-local flag; an explicit project is selected with `mandala --project DIR status --json`. `--json=false` retains the existing text output. Status is read-only and emits valid JSON even when required gaps make it exit `1`; optional-only gaps exit `0`.

For a plan with one expanded group, three required leaves and two optional leaves:

```json
{"schema_version":1,"goal":"Implement OAuth","cells":6,"groups":1,"required":{"open":2,"done":1,"na":0},"optional":{"open":1,"done":1,"na":0},"required_gaps":2}
```

The status output schema version is `1`, independent of the state-file schema. `cells` includes expanded parents; `groups` counts those parents, while `required` and `optional` count leaves only. Thus `cells` equals `groups` plus all leaf counts, and `required_gaps` equals `required.open`. An empty plan emits zero for every count and exits `0`; zero declared gaps does not prove comprehensive coverage or implementation quality.

`gaps --json` emits valid JSON even when it exits `1`:

```json
{"schema_version":1,"gaps":[{"id":"compatibility","required":true},{"id":"security.token-storage","required":true},{"id":"tests","required":true}]}
```

With no matching cells, `gaps` emits `{"schema_version":1,"gaps":[]}` in JSON mode and nothing in text mode. `show --json` emits `schema_version`, `goal`, and an ID-sorted `cells` array. Each cell has `id`, `parent`, `status`, and `required`. JSON output schema version and state-file schema version are separate contracts; both are `1` in v0.1.

## Batch operations

`batch` applies a JSON document of `add` and `mark` operations in one all-or-nothing step:

```sh
mandala batch --file operations.json          # apply
mandala batch --file operations.json --dry-run # preview without writing
mandala batch --file - --dry-run --json        # read the document from stdin
```

`--file` is required; `-` reads stdin. The path is resolved against the invocation directory, not `--project`. `--dry-run` and `--json` are optional booleans, default false. `batch` operates only on an existing project and never initializes one. Extra positional arguments, a repeated `--file`, or unknown flags are usage errors.

Input document (`batch_version` 1):

```json
{
  "batch_version": 1,
  "operations": [
    {"op": "add", "id": "security"},
    {"op": "add", "id": "security.csrf"},
    {"op": "add", "id": "tests", "optional": true},
    {"op": "mark", "id": "security.csrf", "status": "done"}
  ]
}
```

An `add` accepts `op`, `id`, and optional boolean `optional` (default `false`); `status` is forbidden. A `mark` accepts exactly `op`, `id`, and `status` (`open`, `done`, or `na`; `optional` is forbidden even when `false`). Parsing is strict: unknown keys (case-sensitive), duplicate keys (including duplicates that collide only after escape decoding), `null`, wrong types, trailing JSON values, invalid UTF-8, a UTF-8 BOM, a non-integer or non-`1` `batch_version`, zero operations, more than 128 operations, input over 65,536 bytes, and non-regular files are all rejected before any state is read — with `E_LIMIT` for the size and operation-count limits, `E_IO` for non-regular input files (FIFOs and directories), and `E_BATCH_INVALID` for every other case above. The input path may be a symlink to a regular file. `--file -` is the recommended way to read a batch from standard input. Paths like `/dev/stdin` are accepted only when they resolve to a regular file; on macOS, `/dev/stdin` and `/dev/fd/N` are fdesc nodes whose stat identity never matches the opened file, so they are always rejected there — use `--file -` instead. On Windows, a metadata handle captures the input identity before the data handle is opened and stays open through the identity comparison. On Unix, a nonblocking open prevents a FIFO replacement from waiting for a writer. The opened file must be regular and match the identity snapshot before any input is read. These checks do not prevent another process from changing the contents of the same file while it is read, so use input locations you trust.

Operations apply in array order with no implicit sorting; a child `add` fails if its parent is not registered earlier in the batch or already present in state. Operations reuse the same rules as the single-cell commands: ID grammar, the two-level limit, 8 roots, 8 children per root, 72 cells total, optional inheritance, parent expansion on child add, and no marking an expanded cell. Marking the same cell repeatedly is allowed. Adding an ID that already exists — in state or earlier in the batch — is a duplicate error. If any operation fails, nothing is saved.

`batch` never exits `1`: a successful apply is `0` even when required gaps remain.

### Batch output

Apply prints nothing on success unless `--json` is given. Dry-run always prints a preview: a deterministic diff of the final cell states, not the operation list. Cells are listed by ID in ascending string order, and a preview of the same input is byte-identical across runs, platforms, and cell storage order.

The following previews start with one open required `security` cell. The batch has two operations that add `security.csrf` and `tests`.

Text dry-run:

```text
batch dry-run: 2 operation(s), 3 changed cell(s)
required gaps: 1 -> 2
~ security [open, required] -> [expanded, required]
+ security.csrf [open, required]
+ tests [open, required]
```

`+` marks added cells and `~` changed cells; a parent that becomes `expanded` counts as changed. `required gaps` uses the same count as `status`. A semantic no-op (for example marks that cancel out) reports `0 changed cell(s)`, exits `0`, and — like a no-op apply — does not rewrite `state.json`.

`--json` emits a compact one-line object with a single trailing newline (shown indented here for readability):

```json
{
  "batch_result_version": 1,
  "dry_run": true,
  "changed": true,
  "operations": 2,
  "required_gaps_before": 1,
  "required_gaps_after": 2,
  "changes": [
    {
      "id": "security",
      "before": {"parent": "", "status": "open", "required": true},
      "after": {"parent": "", "status": "expanded", "required": true}
    },
    {
      "id": "security.csrf",
      "before": null,
      "after": {"parent": "security", "status": "open", "required": true}
    },
    {
      "id": "tests",
      "before": null,
      "after": {"parent": "", "status": "open", "required": true}
    }
  ]
}
```

`operations` is the input operation count; `changes` contains only cells whose final state differs, with `before: null` for added cells. The result schema version (`batch_result_version`) is independent of the state-file schema. With `--json` on a real apply, the result is printed only after the save succeeds (`dry_run: false`). Validation and save failures produce no success JSON.

If writing stdout fails after a successful save, the saved state is not rolled back and partial JSON may remain on stdout. A write error returned to the CLI reports `E_IO` with exit `2`. On Unix, writing to a pipe whose reader has closed can instead terminate the process with `SIGPIPE`; shells commonly report status `141`, and stderr may be empty. If this happens while a real apply writes its result, the save has already committed. A dry-run remains non-writing even if its preview triggers SIGPIPE. A signal death (or exit `2`) is therefore not proof that the batch was not applied. Do not retry automatically. Inspect the current state with `show --json` before deciding whether to retry. Exactly-once execution is not guaranteed.

Re-running a batch is safe but not result-identical: a re-run that repeats `add` for cells that now exist fails atomically with `E_DUPLICATE_ID` (nothing is applied), while re-running `mark`-only operations converges to a semantic no-op (`changed:false`, no save).

### Atomicity scope

The batch is atomic within one CLI run: the state is loaded, copied, all operations and final validation run on the copy, and `state.json` is replaced at most once — via the same temporary-file, sync, rename write as other commands — and only when something changed. On any input, operation, validation, or save failure the state file keeps its exact bytes and mtime, no partial batch is stored, stdout is empty, and the exit code is `2`.

This does not add multi-process coordination. v0.5.0 keeps the single-writer assumption: no file locking, no lost-update detection between a dry-run and a later apply, no exactly-once execution guarantee, and no cross-process transaction or power-loss guarantee beyond the existing save path. Callers must serialize concurrent mutations.

## State lifecycle and limits

Mandala stores one state per project in `.mandala/state.json`. `init` creates it, and completing all declared required leaves **does not** remove it. `status`, `gaps`, and `show` continue to work after completion. `clean` deletes a valid Mandala state file, preserves unrelated files in `.mandala`, and removes the directory only when it becomes empty after that deletion. Repeating `clean` when state is absent succeeds. An invalid state is retained for inspection rather than deleted.

When `init` creates a Mandala project inside a Git working tree, Mandala keeps `.mandala/` local by ensuring it is ignored through the repository-local Git exclude when needed. Mandala does not modify the project's `.gitignore` or global Git ignore and configuration. Non-Git projects continue to work normally. `clean` does not remove the repository-local exclude rule.

State writes use a temporary file in `.mandala`, sync and close it, then replace `state.json`. v0.1 assumes a single writer; callers must serialize concurrent mutations. This is not a power-loss durability or multi-process transaction guarantee.

Mandala detects gaps only among **declared** required leaves. An empty plan has no required gap and exits `0`; that is not proof that the goal was comprehensively decomposed. `done` and `na` are user declarations, not evidence verification. The Go CLI core has no database, network operation, daemon, generated facet, or Agent runtime.

## CI and releases

GitHub Actions is configured to validate every pull request (including documentation-only changes) and every push to `main`: a six-job test matrix (ubuntu-24.04, macos-15, windows-2025 x Go 1.26.x and 1.27.x) running version/environment checks, `go test`, `go vet`, and `go build`; Ubuntu jobs running the race detector for both Go versions; and an Ubuntu CLI smoke test that builds the real binary and drives it against a temporary project, including batch dry-run, apply, JSON, stdin, and failure cases. This describes configured validation, not observed results; see [.github/workflows/ci.yml](.github/workflows/ci.yml) for the configuration and the repository's Actions runs for what actually passed on any given commit. Releases are manual and source-only; the procedure and its evidence gates are documented in [docs/RELEASING.md](docs/RELEASING.md).

For v0.5.1 maintenance, [the local evidence collector](docs/release-evidence-collector.md) records command output, exit codes, exact toolchain versions, source identity, UTC times, and log hashes outside the checkout. Its results support local validation; native Windows CI, independent audit, and release approval remain separate requirements.

## License

MIT. See [LICENSE](LICENSE).
