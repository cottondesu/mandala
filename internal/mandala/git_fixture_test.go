package mandala

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"
)

func requireGit(t *testing.T) string {
	t.Helper()
	path, err := exec.LookPath("git")
	if err != nil {
		t.Skip("git executable is unavailable")
	}
	return path
}

func runFixtureGit(t *testing.T, root string, input []byte, args ...string) string {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	// core.autocrlf=false keeps fixture hashing independent of the host's
	// line-ending conversion (Windows defaults to true and its warnings
	// would otherwise leak into machine-readable stdout); the explicitly
	// empty core.excludesFile neutralizes config-derived global excludes
	// on every platform because Git rejects "NUL" on Windows.
	commandArgs := append([]string{"-c", "core.autocrlf=false", "-c", "core.excludesFile=", "-C", root}, args...)
	cmd := exec.CommandContext(ctx, requireGit(t), commandArgs...)
	cmd.Stdin = bytes.NewReader(input)
	cmd.Env = append(sanitizeGitEnvironment(os.Environ()),
		"GIT_AUTHOR_NAME=Mandala Test",
		"GIT_AUTHOR_EMAIL=mandala@example.invalid",
		"GIT_COMMITTER_NAME=Mandala Test",
		"GIT_COMMITTER_EMAIL=mandala@example.invalid",
	)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()
	if ctx.Err() != nil {
		t.Fatalf("git %v timed out: %v", args, ctx.Err())
	}
	if err != nil {
		t.Fatalf("git %v: %v: %s", args, err, stderr.String())
	}
	return strings.TrimSpace(stdout.String())
}

func initFixtureRepository(t *testing.T, tracked map[string]string) string {
	t.Helper()
	root := t.TempDir()
	runFixtureGit(t, root, nil, "init", "--quiet")
	runFixtureGit(t, root, nil, "symbolic-ref", "HEAD", "refs/heads/main")
	excludesFile := filepath.Join(t.TempDir(), "empty-excludes")
	if err := os.WriteFile(excludesFile, nil, 0600); err != nil {
		t.Fatal(err)
	}
	runFixtureGit(t, root, nil, "config", "--local", "core.excludesFile", excludesFile)

	entries := make([]string, 0, len(tracked))
	for name, content := range tracked {
		path := filepath.Join(root, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(content), 0600); err != nil {
			t.Fatal(err)
		}
		blob := runFixtureGit(t, root, nil, "hash-object", "-w", "--", name)
		entries = append(entries, fmt.Sprintf("100644 blob %s\t%s\n", blob, name))
	}
	slices.Sort(entries)
	tree := runFixtureGit(t, root, []byte(strings.Join(entries, "")), "mktree")
	commit := runFixtureGit(t, root, []byte("fixture\n"), "commit-tree", tree)
	runFixtureGit(t, root, nil, "update-ref", "HEAD", commit)
	runFixtureGit(t, root, nil, "read-tree", "HEAD")
	return root
}

func fixtureExcludePath(t *testing.T, root string) string {
	t.Helper()
	return runFixtureGit(t, root, nil, "rev-parse", "--path-format=absolute", "--git-path", "info/exclude")
}

func newTestState(t *testing.T) State {
	t.Helper()
	state, err := NewState("goal")
	if err != nil {
		t.Fatal(err)
	}
	return state
}

func requireStateExists(t *testing.T, root string) {
	t.Helper()
	if _, err := os.Stat(filepath.Join(root, ".mandala", "state.json")); err != nil {
		t.Fatalf("state does not exist: %v", err)
	}
}
