// Command release-evidence collects local release-gate evidence for one exact
// candidate commit of github.com/cottondesu/mandala (input to the
// LOCAL_TESTS_VALIDATED gate in docs/RELEASING.md). It runs a fixed, whitelisted
// set of commands only - never an arbitrary shell - and writes manifest.json,
// summary.md, and raw per-command stdout/stderr captures into a new, unique
// output directory outside the source root. Local evidence only: it does not
// assert remote CI, Windows, independent review, or release approval.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"
)

const usage = `release-evidence - local release evidence collector (LOCAL_TESTS_VALIDATED input)

Usage:
  go run ./cmd/release-evidence -output DIR -candidate FULLSHA \
      [-expected-go-minor 1.27] [-allow-dirty] [-go PATH]

-output must be a new, never-used directory outside the source root.
See docs/release-evidence-collector.md for the full contract.`

type config struct {
	output        string
	candidate     string
	allowDirty    bool
	goPath        string
	expectedMinor string
}

func main() {
	os.Exit(run(os.Args[1:]))
}

func run(args []string) int {
	var cfg config
	flags := flag.NewFlagSet("release-evidence", flag.ContinueOnError)
	flags.StringVar(&cfg.output, "output", "", "new, unique output directory outside the source root (required)")
	flags.StringVar(&cfg.candidate, "candidate", "", "expected full git commit SHA of the release candidate (required)")
	flags.BoolVar(&cfg.allowDirty, "allow-dirty", false, "development mode: collect even when the tree is dirty; release status stays BLOCKED and no exact-candidate claim is made")
	flags.StringVar(&cfg.goPath, "go", "go", "path of the Go toolchain to interrogate and use")
	flags.StringVar(&cfg.expectedMinor, "expected-go-minor", "", "required Go minor version, e.g. 1.27; a local PASS verdict requires a match")
	flags.Usage = func() { fmt.Fprintln(os.Stderr, usage) }
	if err := flags.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return 0
		}
		return 2
	}
	if cfg.output == "" || cfg.candidate == "" {
		fmt.Fprintln(os.Stderr, "release-evidence: -output and -candidate are required")
		fmt.Fprintln(os.Stderr, usage)
		return 2
	}
	if err := validateSHA(cfg.candidate); err != nil {
		fmt.Fprintf(os.Stderr, "release-evidence: -candidate: %v\n", err)
		return 2
	}

	// SIGINT/SIGTERM cancel the context: the running gate command is marked
	// INTERRUPTED and the manifest is persisted. A hard kill leaves the
	// last-persisted RUNNING status in place - never PASS either way.
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	probe := &execRunner{ctx: ctx}
	root, err := repoRoot(probe)
	if err != nil {
		fmt.Fprintf(os.Stderr, "release-evidence: %v\n", err)
		return 1
	}
	outDir, err := createOutputDir(cfg.output, root)
	if err != nil {
		fmt.Fprintf(os.Stderr, "release-evidence: %v\n", err)
		return 1
	}

	c := &Collector{
		RepoRoot:      root,
		OutputDir:     outDir,
		Candidate:     strings.ToLower(cfg.candidate),
		GoPath:        cfg.goPath,
		ExpectedMinor: normalizeMinor(cfg.expectedMinor),
		AllowDirty:    cfg.allowDirty,
		Runner:        &execRunner{ctx: ctx, dir: root},
		Ctx:           ctx,
		StartedUTC:    nowUTC(),
	}
	if err := c.Collect(); err != nil {
		fmt.Fprintf(os.Stderr, "release-evidence: %v\n", err)
		return 1
	}
	m := c.Manifest()
	fmt.Fprintf(os.Stdout, "release-evidence: release_status=%s output=%s\n", m.ReleaseStatus, outDir)
	for _, reason := range m.BlockedReasons {
		fmt.Fprintf(os.Stdout, "  blocked: %s\n", reason)
	}
	switch {
	case m.Interrupted:
		return 130
	case m.ReleaseStatus == StatusPASS:
		return 0
	default:
		return 1
	}
}

func repoRoot(r Runner) (string, error) {
	res := r.Run([]string{"git", "rev-parse", "--show-toplevel"})
	switch {
	case res.LaunchError != "":
		return "", fmt.Errorf("launching git to find the repo root: %s", res.LaunchError)
	case res.ExitCode != 0:
		return "", fmt.Errorf("git rev-parse --show-toplevel failed (exit %d): %s", res.ExitCode, firstLine(res.Stderr))
	}
	root := strings.TrimSpace(string(res.Stdout))
	if root == "" {
		return "", errors.New("git rev-parse --show-toplevel returned an empty root")
	}
	return filepath.Clean(root), nil
}

// createOutputDir makes the evidence directory. The final component is created
// with os.Mkdir only, so a rerun can never overwrite previous evidence, and it
// must live outside the source root.
func createOutputDir(path, repoRoot string) (string, error) {
	abs, err := filepath.Abs(path)
	if err != nil {
		return "", err
	}
	root, err := filepath.Abs(repoRoot)
	if err != nil {
		return "", err
	}
	root, err = filepath.EvalSymlinks(root)
	if err != nil {
		return "", err
	}
	parent, err := filepath.EvalSymlinks(filepath.Dir(abs))
	if err != nil {
		return "", fmt.Errorf("output parent must already exist: %w", err)
	}
	abs = filepath.Join(parent, filepath.Base(abs))
	compareRoot, compareOutput := root, abs
	if runtime.GOOS == "windows" {
		compareRoot, compareOutput = strings.ToLower(root), strings.ToLower(abs)
	}
	rel, err := filepath.Rel(compareRoot, compareOutput)
	if err != nil && (runtime.GOOS != "windows" || strings.EqualFold(filepath.VolumeName(compareRoot), filepath.VolumeName(compareOutput))) {
		return "", err
	}
	if err == nil && (rel == "." || (rel != ".." && !strings.HasPrefix(rel, ".."+string(os.PathSeparator)))) {
		return "", fmt.Errorf("-output %s is inside the source root %s; evidence must be written outside it", abs, root)
	}
	if err := os.Mkdir(abs, 0o755); err != nil {
		if errors.Is(err, os.ErrExist) {
			return "", fmt.Errorf("-output %s already exists; earlier evidence is never overwritten, choose a new unique directory", abs)
		}
		return "", err
	}
	return abs, nil
}

func validateSHA(s string) error {
	if len(s) != 40 && len(s) != 64 {
		return fmt.Errorf("want a full 40- (or 64-) hex-character SHA, got %d characters", len(s))
	}
	for _, ch := range strings.ToLower(s) {
		if !((ch >= '0' && ch <= '9') || (ch >= 'a' && ch <= 'f')) {
			return errors.New("contains a non-hex character")
		}
	}
	return nil
}
