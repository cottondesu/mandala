//go:build unix

// Real-binary tests pinning the batch stdout failure contract on Unix.
//
// The contract (see also the README "Batch output" section):
//
//   - The batch save happens before the result is written, so once output
//     starts, a committed state may already exist on disk.
//   - When a stdout write fails with an error the CLI can observe, run
//     reports E_IO with exit 2 (covered for real by the /dev/full test in
//     batch_stdout_linux_test.go, and in-process by the injected-writer
//     unit test TestBatchPartialStdoutFailureKeepsSavedState).
//   - When stdout is a broken pipe, the write to file descriptor 1 fails
//     with EPIPE and the Go runtime delivers SIGPIPE with its default
//     disposition: the process is terminated by the signal. It does not
//     exit 2 and prints nothing to stderr. A shell renders this death as
//     status 141 (128 + SIGPIPE), which is a shell convention, not a
//     process exit code.
//
// These tests must never be "fixed" by suppressing SIGPIPE, resetting its
// handler, or converting the signal death into an E_IO exit 2: that would
// change the product's signal behavior rather than test it.

package main

import (
	"bytes"
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"syscall"
	"testing"
	"time"
)

// invokeCLIBrokenStdout runs the real binary with stdout connected to a
// pipe whose read end is closed BEFORE the child starts. Every write the
// child makes to fd 1 then fails with EPIPE immediately, so the outcome is
// deterministic: no reader has to close at the right moment, and there is
// no reader-timing race. It returns the finished command (whose
// ProcessState carries the WaitStatus) and the captured stderr.
func invokeCLIBrokenStdout(t *testing.T, binary, dir, stdin string, args ...string) (*exec.Cmd, string) {
	t.Helper()
	readEnd, writeEnd, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	// Determinism: the peer is gone before the child exists.
	if err := readEnd.Close(); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, binary, args...)
	cmd.Dir = dir
	cmd.Stdin = strings.NewReader(stdin)
	cmd.Stdout = writeEnd
	cmd.Env = append(os.Environ(), "TERM=dumb")
	var diagnostics bytes.Buffer
	cmd.Stderr = &diagnostics
	if err := cmd.Start(); err != nil {
		writeEnd.Close()
		t.Fatalf("start CLI: %v", err)
	}
	// The child owns its descriptor from here on; drop the parent copy.
	writeEnd.Close()
	if err := cmd.Wait(); err != nil {
		// An ExitError is expected (the child dies from a signal); it is
		// inspected by the caller via cmd.ProcessState. Anything else,
		// such as a failure to even run the binary, fails the test below.
		var exitErr *exec.ExitError
		if !errors.As(err, &exitErr) {
			t.Fatalf("run CLI: %v", err)
		}
	}
	if ctx.Err() != nil {
		t.Fatalf("CLI blocked: %v", ctx.Err())
	}
	return cmd, diagnostics.String()
}

// requireSIGPIPE asserts that the child was terminated by the SIGPIPE
// signal itself, judged from the WaitStatus — not from a numeric
// shell-style status. Shells commonly report this death as 141, but
// exec.ExitCode() returns -1 for a signaled process; asserting 141 here
// would test shell arithmetic, not mandala. It also asserts stderr is
// empty: the signal arrives during the stdout write, before any error
// could be reported, so there is nothing to print.
func requireSIGPIPE(t *testing.T, cmd *exec.Cmd, diagnostics string) {
	t.Helper()
	if cmd.ProcessState == nil {
		t.Fatal("missing process state")
	}
	if code := cmd.ProcessState.ExitCode(); code != -1 {
		t.Fatalf("expected a signaled process (ExitCode -1), got exit code %d (stderr=%q)", code, diagnostics)
	}
	ws, ok := cmd.ProcessState.Sys().(syscall.WaitStatus)
	if !ok || !ws.Signaled() || ws.Signal() != syscall.SIGPIPE {
		t.Fatalf("expected termination by SIGPIPE, got process state %v (stderr=%q)", cmd.ProcessState.Sys(), diagnostics)
	}
	if diagnostics != "" {
		t.Fatalf("expected empty stderr on SIGPIPE death, got %q", diagnostics)
	}
}

// requireMandalaEntries asserts .mandala holds exactly the expected entry
// names — in particular no leftover ".state-*" temporary files from the
// atomic save path.
func requireMandalaEntries(t *testing.T, root string, want ...string) {
	t.Helper()
	entries, err := os.ReadDir(filepath.Join(root, ".mandala"))
	if err != nil {
		t.Fatal(err)
	}
	got := make([]string, 0, len(entries))
	for _, entry := range entries {
		got = append(got, entry.Name())
	}
	slices.Sort(got)
	if !slices.Equal(got, want) {
		t.Fatalf(".mandala entries = %v, want %v", got, want)
	}
}

// TestCLIBatchApplyBrokenStdoutSIGPIPEAfterCommit proves the ordering of
// the apply contract with a real binary: `batch --file - --json` reads its
// document from stdin, commits the save, and only then writes the JSON
// result to stdout. With stdout already a broken pipe the child dies from
// SIGPIPE during that write, yet the batch is fully committed, stderr is
// empty, and no temporary files remain. The committed state is verified
// through `show --json` — the same inspection command the README
// recommends after any output failure.
func TestCLIBatchApplyBrokenStdoutSIGPIPEAfterCommit(t *testing.T) {
	binary := buildCLI(t)
	root := initCLIProject(t, binary)
	cmd, diagnostics := invokeCLIBrokenStdout(t, binary, root, integrationBatchInput, "batch", "--file", "-", "--json")
	requireSIGPIPE(t, cmd, diagnostics)

	code, out, diagnostics := invokeCLI(t, binary, root, "show", "--json")
	if code != 0 || diagnostics != "" {
		t.Fatalf("show after SIGPIPE: exit=%d stderr=%q", code, diagnostics)
	}
	for _, want := range []string{
		`"id":"security"`,
		`"status":"expanded"`,
		`"id":"security.csrf"`,
		`"status":"done"`,
		`"id":"tests"`,
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("show output missing %s:\n%s", want, out)
		}
	}
	requireMandalaEntries(t, root, "state.json")
}

// TestCLIBatchDryRunBrokenStdoutSIGPIPEPreservesState proves a dry-run
// that dies writing its preview to a broken pipe never touched state.json:
// bytes and mtime are byte-for-byte identical afterwards and no temporary
// files remain.
func TestCLIBatchDryRunBrokenStdoutSIGPIPEPreservesState(t *testing.T) {
	binary := buildCLI(t)
	root := initCLIProject(t, binary)
	if code, _, diagnostics := invokeCLI(t, binary, root, "add", "security"); code != 0 {
		t.Fatal(diagnostics)
	}
	statePath := filepath.Join(root, ".mandala", "state.json")
	before, err := os.ReadFile(statePath)
	if err != nil {
		t.Fatal(err)
	}
	beforeInfo, err := os.Stat(statePath)
	if err != nil {
		t.Fatal(err)
	}
	input := `{"batch_version":1,"operations":[{"op":"add","id":"security.csrf"},{"op":"add","id":"tests"}]}`
	cmd, diagnostics := invokeCLIBrokenStdout(t, binary, root, input, "batch", "--file", "-", "--dry-run", "--json")
	requireSIGPIPE(t, cmd, diagnostics)

	after, err := os.ReadFile(statePath)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(before, after) {
		t.Fatalf("dry-run changed state bytes:\nbefore=%q\n after=%q", before, after)
	}
	afterInfo, err := os.Stat(statePath)
	if err != nil {
		t.Fatal(err)
	}
	if !afterInfo.ModTime().Equal(beforeInfo.ModTime()) {
		t.Fatalf("dry-run changed state mtime: before=%v after=%v", beforeInfo.ModTime(), afterInfo.ModTime())
	}
	requireMandalaEntries(t, root, "state.json")
}

// TestCLIBatchRetryAfterBrokenStdoutApply covers the retry semantics the
// README prescribes after an output failure. Because the SIGPIPE death
// happened after the save, the state already reflects the batch:
//
//   - Re-running the same add batch fails atomically with E_DUPLICATE_ID,
//     exit 2, and leaves the committed bytes and mtime untouched.
//   - Re-running the batch's marks converges to a semantic no-op:
//     changed:false, exit 0, no state rewrite.
func TestCLIBatchRetryAfterBrokenStdoutApply(t *testing.T) {
	binary := buildCLI(t)
	root := initCLIProject(t, binary)
	cmd, diagnostics := invokeCLIBrokenStdout(t, binary, root, integrationBatchInput, "batch", "--file", "-", "--json")
	requireSIGPIPE(t, cmd, diagnostics)

	statePath := filepath.Join(root, ".mandala", "state.json")
	committed, err := os.ReadFile(statePath)
	if err != nil {
		t.Fatal(err)
	}
	committedInfo, err := os.Stat(statePath)
	if err != nil {
		t.Fatal(err)
	}
	requireStateStable := func(stage string) {
		t.Helper()
		current, err := os.ReadFile(statePath)
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(committed, current) {
			t.Fatalf("%s changed committed bytes:\ncommitted=%q\ncurrent=%q", stage, committed, current)
		}
		info, err := os.Stat(statePath)
		if err != nil {
			t.Fatal(err)
		}
		if !info.ModTime().Equal(committedInfo.ModTime()) {
			t.Fatalf("%s rewrote state.json: committed=%v current=%v", stage, committedInfo.ModTime(), info.ModTime())
		}
	}

	code, out, diagnostics := invokeCLIStdin(t, binary, root, integrationBatchInput, "batch", "--file", "-")
	if code != 2 || out != "" || !strings.Contains(diagnostics, "E_DUPLICATE_ID") {
		t.Fatalf("duplicate retry: exit=%d stdout=%q stderr=%q", code, out, diagnostics)
	}
	requireStateStable("duplicate retry")

	markOnly := `{"batch_version":1,"operations":[` +
		`{"op":"mark","id":"security.csrf","status":"done"},` +
		`{"op":"mark","id":"tests","status":"open"}]}`
	code, out, diagnostics = invokeCLIStdin(t, binary, root, markOnly, "batch", "--file", "-", "--json")
	if code != 0 || diagnostics != "" || !strings.Contains(out, `"changed":false`) {
		t.Fatalf("mark-only retry: exit=%d stdout=%q stderr=%q", code, out, diagnostics)
	}
	requireStateStable("mark-only retry")
	requireMandalaEntries(t, root, "state.json")
}
