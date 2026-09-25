# Mandala

An offline Go CLI for tracking declared coverage gaps in human and coding agent work.

> Tasks tell agents what to do. Cells tell you what they forgot.

Mandala tracks the cells you declare for a goal and reports open required leaves as gaps, in deterministic text or JSON. It works with human workflows and coding agents, but does not generate a complete plan, inspect code, execute tasks, or call an LLM. Zero gaps means only that the required cells you declared are resolved.

## Install

Requires Go 1.26 or newer. Install the latest published version:

```sh
go install github.com/cottondesu/mandala/cmd/mandala@latest
```

From a checkout, you can instead build a local binary or install it into your Go binary directory:

```sh
go build -o mandala ./cmd/mandala
go install ./cmd/mandala
```

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
| `status` | Show cell counts and required gap count |
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

`gaps --json` emits valid JSON even when it exits `1`:

```json
{"schema_version":1,"gaps":[{"id":"compatibility","required":true},{"id":"security.token-storage","required":true},{"id":"tests","required":true}]}
```

With no matching cells, `gaps` emits `{"schema_version":1,"gaps":[]}` in JSON mode and nothing in text mode. `show --json` emits `schema_version`, `goal`, and an ID-sorted `cells` array. Each cell has `id`, `parent`, `status`, and `required`. JSON output schema version and state-file schema version are separate contracts; both are `1` in v0.1.

## State lifecycle and limits

Mandala stores one state per project in `.mandala/state.json`. `init` creates it, and completing all declared required leaves **does not** remove it. `status`, `gaps`, and `show` continue to work after completion. `clean` deletes a valid Mandala state file, preserves unrelated files in `.mandala`, and removes the directory only when it becomes empty after that deletion. Repeating `clean` when state is absent succeeds. An invalid state is retained for inspection rather than deleted.

State writes use a temporary file in `.mandala`, sync and close it, then replace `state.json`. v0.1 assumes a single writer; callers must serialize concurrent mutations. This is not a power-loss durability or multi-process transaction guarantee.

Mandala detects gaps only among **declared** required leaves. An empty plan has no required gap and exits `0`; that is not proof that the goal was comprehensively decomposed. `done` and `na` are user declarations, not evidence verification. There is no database, network operation, daemon, generated facet, Agent runtime, Skill, or Plugin in v0.1.

## License

MIT. See [LICENSE](LICENSE).
