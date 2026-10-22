# Mandala

An offline Go CLI for tracking declared coverage gaps in human and coding agent work.

> Tasks tell agents what to do. Cells tell you what they forgot.

Mandala tracks the cells you declare for a goal and reports open required leaves as gaps, in deterministic text or JSON. It works with human workflows and coding agents, but does not generate a complete plan, inspect code, execute tasks, or call an LLM. Zero gaps means only that the required cells you declared are resolved.

## Install

mandala requires Go 1.26 or newer.

Install the published v0.4.0 release:

```sh
go install github.com/cottondesu/mandala/cmd/mandala@v0.4.0
```

Or install the latest published release:

```sh
go install github.com/cottondesu/mandala/cmd/mandala@latest
```

`@v0.4.0` always installs exactly v0.4.0; `@latest` resolves to the newest published release, and what it resolves to changes as releases are published.

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
| `clean` | Remove Mandala's valid `state.json` explicitly |

All commands are non-interactive. Place command flags before positional arguments. `--project DIR` is a global flag placed **before** the command, for example `mandala --project ../repo gaps --json`. It accepts a project root, not its `.mandala` directory. Without it, read and update commands resolve symlinks, then search that physical directory and its parents up to the filesystem root. The nearest `.mandala` wins; a broken one is an error rather than a reason to use an ancestor. `init` creates state at the specified directory or the current directory and refuses to create a nested project inside an existing one. For deletion safety, `clean` acts only on the current directory or an explicit `--project DIR`; it does not search parents.

IDs are stable, lowercase ASCII paths such as `security` and `security.csrf`. Each segment starts with `a`–`z`, then contains up to 63 lowercase letters, digits, or hyphens. v0.1 allows at most eight root cells and eight children per root cell: at most 72 cells in two levels. IDs cannot be renamed by the CLI.

## Output and exit codes

stdout contains requested results; stderr contains errors. Mutating commands produce no stdout on success. JSON mode produces one valid JSON object and a newline, with no progress text. `gaps` includes optional open cells by default, marking them `[optional]` in text; `--required` filters them out. Optional gaps never cause exit `1`.

| Exit | Meaning |
| --- | --- |
| `0` | Success; for `status` and `gaps`, no required gap |
| `1` | `status` or `gaps` found at least one required gap |
| `2` | Usage, project, state, or I/O error; stdout is empty |

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

## State lifecycle and limits

Mandala stores one state per project in `.mandala/state.json`. `init` creates it, and completing all declared required leaves **does not** remove it. `status`, `gaps`, and `show` continue to work after completion. `clean` deletes a valid Mandala state file, preserves unrelated files in `.mandala`, and removes the directory only when it becomes empty after that deletion. Repeating `clean` when state is absent succeeds. An invalid state is retained for inspection rather than deleted.

When `init` creates a Mandala project inside a Git working tree, Mandala keeps `.mandala/` local by ensuring it is ignored through the repository-local Git exclude when needed. Mandala does not modify the project's `.gitignore` or global Git ignore and configuration. Non-Git projects continue to work normally. `clean` does not remove the repository-local exclude rule.

State writes use a temporary file in `.mandala`, sync and close it, then replace `state.json`. v0.1 assumes a single writer; callers must serialize concurrent mutations. This is not a power-loss durability or multi-process transaction guarantee.

Mandala detects gaps only among **declared** required leaves. An empty plan has no required gap and exits `0`; that is not proof that the goal was comprehensively decomposed. `done` and `na` are user declarations, not evidence verification. The Go CLI core has no database, network operation, daemon, generated facet, or Agent runtime.

## CI and releases

GitHub Actions is configured to validate every pull request (including documentation-only changes) and every push to `main`: a six-job test matrix (ubuntu-24.04, macos-15, windows-2025 x Go 1.26.x and 1.27.x) running version/environment checks, `go test`, `go vet`, and `go build`; Ubuntu jobs running the race detector for both Go versions; and an Ubuntu CLI smoke test that builds the real binary and drives it against a temporary project. This describes configured validation, not observed results; see [.github/workflows/ci.yml](.github/workflows/ci.yml) for the configuration and the repository's Actions runs for what actually passed on any given commit. Releases are manual and source-only; the procedure and its evidence gates are documented in [docs/RELEASING.md](docs/RELEASING.md).

## License

MIT. See [LICENSE](LICENSE).
