//go:build linux

// Real-binary counterpart of the injected-writer unit test
// (TestBatchPartialStdoutFailureKeepsSavedState): when a stdout write
// fails with an error the CLI can observe — ENOSPC from /dev/full on
// Linux — the error is returned and reported as `E_IO: write stdout` with
// exit 2, while the already-committed save stands. This is deliberately
// distinct from the fd 1 broken-pipe case in batch_stdout_unix_test.go,
// where the Go runtime's default SIGPIPE disposition kills the process
// before any error could be reported. macOS has no /dev/full, so this
// contract is only exercised where the device exists.

package main

import (
	"bytes"
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestCLIBatchStdoutDevFullReturnsEIOAfterCommit(t *testing.T) {
	binary := buildCLI(t)
	root := initCLIProject(t, binary)
	full, err := os.OpenFile("/dev/full", os.O_WRONLY, 0)
	if err != nil {
		t.Skipf("/dev/full unavailable: %v", err)
	}
	defer full.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, binary, "batch", "--file", "-", "--json")
	cmd.Dir = root
	cmd.Stdin = strings.NewReader(integrationBatchInput)
	cmd.Stdout = full
	cmd.Env = append(os.Environ(), "TERM=dumb")
	var diagnostics bytes.Buffer
	cmd.Stderr = &diagnostics
	err = cmd.Run()
	if ctx.Err() != nil {
		t.Fatalf("CLI blocked: %v", ctx.Err())
	}
	var exit *exec.ExitError
	if !errors.As(err, &exit) || exit.ExitCode() != 2 {
		t.Fatalf("expected exit 2, got %v (stderr=%q)", err, diagnostics.String())
	}
	if !strings.Contains(diagnostics.String(), "E_IO: write stdout:") {
		t.Fatalf("stderr missing E_IO write stdout: %q", diagnostics.String())
	}

	// The save committed before the failed write; the reported failure did
	// not roll it back, and no temporary files remain.
	state, err := os.ReadFile(filepath.Join(root, ".mandala", "state.json"))
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{`"id": "security"`, `"id": "security.csrf"`, `"id": "tests"`} {
		if !strings.Contains(string(state), want) {
			t.Fatalf("state missing %s:\n%s", want, state)
		}
	}
	requireMandalaEntries(t, root, "state.json")
}
