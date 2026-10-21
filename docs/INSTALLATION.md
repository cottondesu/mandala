# Installing mandala

mandala is distributed as Go source. Published versions are installed from the module proxy with `go install`, and building from a local checkout is also documented below; the repository ships no binaries and no install scripts. This guide covers installing a published release, finding the installed binary, fixing `PATH` issues, verifying an installation in isolation, and building from a local checkout.

## Requirements

- Go 1.26 or newer. Go 1.26 is the minimum supported version. CI is configured for Go 1.26 and 1.27; the repository's Actions runs determine the observed results, and this guide makes no claim about which remote runs have passed.
- macOS, Linux, or Windows, matching the CI matrix (ubuntu-24.04, macos-15, windows-2025).
- A network connection is needed only while `go install` downloads the module. The installed CLI itself runs offline.

## Install a published release

Install exactly the currently published release, v0.4.0:

```sh
go install github.com/cottondesu/mandala/cmd/mandala@v0.4.0
```

Or install whatever release is currently newest:

```sh
go install github.com/cottondesu/mandala/cmd/mandala@latest
```

- `@v0.4.0` always installs exactly v0.4.0; pin an exact version when you want reproducible setups.
- `@latest` resolves to the newest published stable release, and what it resolves to changes over time as new releases are published.

v0.4.1 is an unpublished candidate at the time of writing; do not assume `@v0.4.1` is installable until its tag actually exists on GitHub.

A successful `go install` only downloads, builds, and places the binary. It does not guarantee that the `mandala` command resolves in your shell: the install directory may not be on your `PATH`. See the next section.

## Where the binary is installed

`go install` places the binary in:

1. `GOBIN`, if the `GOBIN` environment variable is set; otherwise
2. the `bin` subdirectory of the **first** `GOPATH` entry.

`GOPATH` may be a list of directories. The list separator is `:` on macOS/Linux and `;` on Windows, and only the first entry is used for `go install` destinations when `GOBIN` is unset.

Check your effective values (read-only):

```sh
go env GOBIN
go env GOPATH
```

Determine the effective install directory:

- macOS/Linux (bash, zsh): if `go env GOBIN` prints a path, that is the directory. Otherwise take the first `GOPATH` entry and append `/bin`:

  ```sh
  go env GOPATH | cut -d: -f1
  ```

  With the default `GOPATH` this is `$HOME/go/bin`.

- Windows (PowerShell): if `go env GOBIN` prints a path, that is the directory. Otherwise take the first `GOPATH` entry and append `\bin`:

  ```powershell
  (go env GOPATH).Split(';')[0]
  ```

  With the default `GOPATH` this is `$env:USERPROFILE\go\bin`.

Custom `GOBIN` and `GOPATH` settings are honored exactly as set. Paths containing spaces work, provided you quote them in shell commands as shown below.

## Add the install directory to PATH

The examples below first derive your effective install directory (a nonempty `GOBIN`, otherwise the first `GOPATH` entry plus `bin`); the default `$HOME/go/bin` (`$env:USERPROFILE\go\bin` on Windows) is kept only as an illustration. On Unix the examples prepend the directory to the existing `PATH`, which also lets it win over stale copies later on `PATH`; on Windows the examples append. Never overwrite `PATH`; only add to it.

### macOS (zsh)

Determine the effective install directory once (a nonempty `GOBIN` wins; otherwise the first `GOPATH` entry plus `/bin`, which is `$HOME/go/bin` by default):

```sh
GOBIN_DIR="$(go env GOBIN)"
if [ -z "$GOBIN_DIR" ]; then GOBIN_DIR="$(go env GOPATH | cut -d: -f1)/bin"; fi
printf '%s\n' "$GOBIN_DIR"
```

Current terminal only (prepending puts this directory ahead of stale copies elsewhere on `PATH`):

```sh
export PATH="$GOBIN_DIR:$PATH"
```

Persistent: add a line exporting your actual quoted effective directory to `~/.zshrc` manually - for example `export PATH="$HOME/go/bin:$PATH"` when your directory is the default. Inserting the quoted line manually avoids fragile interpolation, especially if the path contains spaces. Then reload:

```sh
source ~/.zshrc
```

### Linux (bash)

Determine the effective install directory once, with the same rule as above:

```sh
GOBIN_DIR="$(go env GOBIN)"
if [ -z "$GOBIN_DIR" ]; then GOBIN_DIR="$(go env GOPATH | cut -d: -f1)/bin"; fi
printf '%s\n' "$GOBIN_DIR"
```

Current terminal only (prepending puts this directory ahead of stale copies elsewhere on `PATH`):

```sh
export PATH="$GOBIN_DIR:$PATH"
```

Persistent for interactive shells: add a line exporting your actual quoted effective directory to `~/.bashrc` manually - for example `export PATH="$HOME/go/bin:$PATH"` when your directory is the default. Then reload:

```sh
source ~/.bashrc
```

Depending on your distribution, login shells may read `~/.profile` or `~/.bash_profile` instead; add the line there if needed.

### Windows (PowerShell)

Determine the effective install directory first (a nonempty `GOBIN` wins; otherwise the first `GOPATH` entry plus `\bin`, which is `$env:USERPROFILE\go\bin` by default). Each `go env` call is checked through its native exit code:

```powershell
$gobin = go env GOBIN
if ($LASTEXITCODE -ne 0) { throw "go env GOBIN failed with exit code $LASTEXITCODE" }
if ($gobin) {
    $bin = $gobin
} else {
    $gopath = go env GOPATH
    if ($LASTEXITCODE -ne 0) { throw "go env GOPATH failed with exit code $LASTEXITCODE" }
    $firstGopath = ($gopath -split ';')[0]
    $bin = Join-Path -Path $firstGopath -ChildPath 'bin'
}
$bin
```

Current session only (append to the existing `Path`, preserving every existing entry):

```powershell
$env:Path = "$env:Path;$bin"
```

Persistent for the current user - check first, then append only if missing (this preserves the existing user `Path`):

```powershell
$userPath = [Environment]::GetEnvironmentVariable('Path', 'User')
if (-not ($userPath -split ';' -contains $bin)) {
    [Environment]::SetEnvironmentVariable('Path', "$userPath;$bin", 'User')
}
```

Changes to the persistent user `Path` only affect new terminals. You can also edit the user `Path` in system settings (Settings > System > About > Advanced system settings > Environment Variables); append the directory and keep every existing entry.

## Resolve which mandala runs

Shells cache the location of commands. After installing a new binary into a directory that is already on `PATH`, refresh the cache:

- bash: `hash -r`
- zsh: `rehash`

Inspect what resolves:

- `command -v mandala` prints the first match on `PATH`, or nothing.
- bash: `type -a mandala` lists every match in `PATH` order.
- zsh: `whence -a mandala` does the same.
- Windows PowerShell: `Get-Command -All mandala`; `where.exe mandala` also works (PowerShell and cmd.exe).

If an older `mandala` appears earlier on `PATH` than your install directory, that stale copy wins. Remove the stale binary or reorder `PATH` so the intended directory comes first. Only delete files you have identified.

## Verify an installation in isolation (recommended)

The checks below install into a throwaway `GOBIN`, so they never modify, replace, or upgrade an existing `mandala` installation.

macOS/Linux (bash, zsh):

```sh
(
  set -e
  VERIFY_GOBIN="$(mktemp -d)"
  trap 'rm -rf -- "$VERIFY_GOBIN"' EXIT
  GOBIN="$VERIFY_GOBIN" go install github.com/cottondesu/mandala/cmd/mandala@v0.4.0
  "$VERIFY_GOBIN/mandala" --version
)
```

Because of `set -e`, the binary is executed only if `go install` exited 0. The `EXIT` trap is installed immediately after a successful `mktemp -d` and removes the temporary `GOBIN` whether the checks succeed or fail, and the subshell leaves the reader's own shell settings untouched.

Windows (PowerShell):

```powershell
$hadGobin = Test-Path Env:GOBIN
$previousGobin = $env:GOBIN
$verifyGobin = Join-Path ([System.IO.Path]::GetTempPath()) ("mandala-verify-" + [System.Guid]::NewGuid().ToString('N'))
New-Item -ItemType Directory -Path $verifyGobin | Out-Null
try {
    $env:GOBIN = $verifyGobin
    go install github.com/cottondesu/mandala/cmd/mandala@v0.4.0
    if ($LASTEXITCODE -ne 0) { throw "go install failed with exit code $LASTEXITCODE" }
    & (Join-Path $verifyGobin 'mandala.exe') --version
    if ($LASTEXITCODE -ne 0) { throw "mandala --version failed with exit code $LASTEXITCODE" }
}
finally {
    if ($hadGobin) { $env:GOBIN = $previousGobin } else { Remove-Item Env:GOBIN -ErrorAction SilentlyContinue }
    Remove-Item -Recurse -Force $verifyGobin
}
```

The script records whether `GOBIN` was present in the process environment (`Test-Path Env:GOBIN`) and its original value, so the `finally` block restores the original process environment exactly - this stays correct even when `GOBIN` exists only in `go env -w` configuration while the environment variable was originally absent. The native exit code (`$LASTEXITCODE`) is checked immediately after `go install` and only then is `mandala.exe` run; it is checked again afterwards. The snippet touches only process environment variables and temporary files; it edits no source and no global Go configuration.

## Build from a local checkout

From the repository root:

```sh
go build -o mandala ./cmd/mandala
```

Windows PowerShell:

```powershell
go build -o mandala.exe .\cmd\mandala
```

This creates a binary in the current directory and does not affect any binary installed via `go install`.

## Version output

`mandala --version` prints the version embedded from the main module:

- Built by `go install` at a published tag, it prints that tag, for example `mandala v0.4.0`.
- Pseudo-versions such as `v0.0.0-20260101120000-abcdefabcdef` are preserved.
- Build metadata such as `+dirty` is preserved when present.
- When the embedded version is absent, empty, or `(devel)`, it prints `mandala (devel)`.

A binary built from a local checkout can print any of these depending on the Go toolchain and build context, so local builds carry no exact-tag guarantee. `--version` used alone - no command, no `--project` - needs no project state, no Git repository, and no network access. Do not pass `--project` together with `--version`.

## Troubleshooting

`go install` fetches modules through the Go module proxy and verifies them against the checksum database:

- proxy.golang.org distributes module metadata and source.
- sum.golang.org records and checks module integrity.

A published GitHub Release is not the same thing as an installable module: the proxy resolves Git tags, and it may have cached a negative answer from a query made before the tag was pushed. At the same time, not every `404` is a caching problem - the tag may simply not exist yet (for example, the unpublished v0.4.1).

### Common symptoms

- `404 Not Found` from proxy.golang.org
- `invalid version: unknown revision ...`
- sum.golang.org lookup failure
- module download failure / network errors

### Diagnostics, in order

These are non-publishing diagnostics: they modify neither the repository nor its tags, though `go list` may update your local module cache. Start at 1; do not skip ahead.

1. **GitHub tag exists.** Check https://github.com/cottondesu/mandala/tags for the version.
2. **Tag points at a commit.** Open the tag page and confirm it resolves to a real commit.
3. **Module and package paths are correct.** The module path is `github.com/cottondesu/mandala`; the `go install` target additionally carries the command package path `github.com/cottondesu/mandala/cmd/mandala`. A typo in either produces download or resolution errors. Verify the module root resolves:

   ```sh
   go list -m github.com/cottondesu/mandala@v0.4.0
   ```

4. **Module proxy state.** Browse https://proxy.golang.org/github.com/cottondesu/mandala/@v/list, or run:

   ```sh
   go list -m -versions github.com/cottondesu/mandala
   ```

5. **Checksum database.** Browse https://sum.golang.org/lookup/github.com/cottondesu/mandala@v0.4.0.
6. **Local network.** Check `go env GOPROXY`; retry later or from another network if the proxy or checksum database is temporarily unreachable.
7. **Distribution cache.** If steps 1-2 confirm the tag exists but the proxy still returns `404`, the proxy may be serving a cached negative answer from a query made before the tag was published. Recheck the metadata endpoints (step 4), the checksum database (step 5), your network (step 6), and the publication state (step 8). If the evidence remains unclear, stop with the cause unresolved rather than guessing; do not change integrity settings or the tag.
8. **Publication state.** Confirm the release actually finished (tag pushed and visible) before drawing conclusions. Publication of a GitHub Release does not by itself make a module installable, and installability can lag tag publication.

### What not to do

- Do not set `GOSUMDB=off`. It disables checksum verification and does not fix the root cause.
- Do not set `GOPRIVATE` for this module. It is public; `GOPRIVATE` is for private modules and only masks the real issue.
- Do not delete, recreate, or force-move a published tag. Tags are immutable; if a release is wrong, the fix is a new patch version.
- Do not run unverified install scripts (for example `curl ... | sh`). This project distributes only through `go install`.
