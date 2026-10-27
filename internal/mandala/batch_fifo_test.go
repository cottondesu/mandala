//go:build unix

package mandala

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"
)

// TestBatchRejectsFIFO guards the nonblocking open in openBatchInputFile.
// Opening a FIFO with no writer blocks forever unless O_NONBLOCK is used, so
// a FIFO input must be rejected as a non-regular file and the command must
// fail fast with E_IO.
func TestBatchRejectsFIFO(t *testing.T) {
	root := initBatchProject(t, "goal")
	fifo := filepath.Join(t.TempDir(), "ops.fifo")
	if err := syscall.Mkfifo(fifo, 0o600); err != nil {
		t.Skipf("mkfifo %s: %v", fifo, err)
	}
	done := make(chan struct{})
	var code int
	var out, diagnostics string
	go func() {
		defer close(done)
		code, out, diagnostics = runBatchCommand(root, "", "--file", fifo)
	}()
	select {
	case <-done:
	case <-time.After(10 * time.Second):
		t.Fatal("batch --file <fifo> did not return: open blocked waiting for a writer")
	}
	if code != 2 || out != "" || !strings.Contains(diagnostics, "E_IO") || !strings.Contains(diagnostics, "not a regular file") {
		t.Fatalf("fifo input: exit=%d stdout=%q stderr=%q", code, out, diagnostics)
	}
}

const batchFIFOSwapChildEnv = "MANDALA_BATCH_FIFO_SWAP_CHILD"

// TestBatchInputFIFOSwap replaces the regular input with a FIFO at the same
// path between the identity snapshot and the data open. The scenario runs in a
// child test process so a regression that blocks in open is killed by the
// timeout instead of leaking a blocked goroutine into this process.
func TestBatchInputFIFOSwap(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, os.Args[0], "-test.run=^TestBatchInputFIFOSwapChild$", "-test.count=1", "-test.v")
	cmd.Env = append(os.Environ(), batchFIFOSwapChildEnv+"=1")
	cmd.WaitDelay = time.Second
	output, err := cmd.CombinedOutput()
	if ctx.Err() != nil {
		t.Fatalf("FIFO swap blocked until timeout: open waited for a writer\n%s", output)
	}
	if err != nil {
		t.Fatalf("FIFO swap child failed: %v\n%s", err, output)
	}
	if !strings.Contains(string(output), "--- PASS: TestBatchInputFIFOSwapChild") {
		t.Fatalf("FIFO swap child did not run:\n%s", output)
	}
}

// TestBatchInputFIFOSwapChild is the child half of TestBatchInputFIFOSwap. It
// does nothing unless launched by the parent.
func TestBatchInputFIFOSwapChild(t *testing.T) {
	if os.Getenv(batchFIFOSwapChildEnv) != "1" {
		return
	}
	for _, args := range [][]string{{"--dry-run"}, {}} {
		t.Run(strings.Join(append([]string{"args"}, args...), "-"), func(t *testing.T) {
			runBatchInputFIFOSwap(t, args)
		})
	}
}

func runBatchInputFIFOSwap(t *testing.T, args []string) {
	root := initBatchProject(t, "FIFO swap")
	if code, _, diagnostics := runTestCommand(root, "add", "alpha"); code != 0 {
		t.Fatal(diagnostics)
	}
	path := filepath.Join(t.TempDir(), "ops.json")
	if err := os.WriteFile(path, []byte(batchDocument(batchAdd("beta"))), 0600); err != nil {
		t.Fatal(err)
	}
	swapped := 0
	withBatchInputOpenHook(t, func(p string) {
		swapped++
		if err := os.Rename(p, p+".orig"); err != nil {
			t.Fatal(err)
		}
		if err := syscall.Mkfifo(p, 0o600); err != nil {
			t.Fatal(err)
		}
	})
	before := readBatchState(t, root)
	mtime := batchStateModTime(t, root)
	identity := batchStateIdentity(t, root)
	code, out, diagnostics := runBatchCommand(root, "", append([]string{"--file", path}, args...)...)
	if swapped != 1 {
		t.Fatalf("swap hook ran %d times, want 1", swapped)
	}
	if code != 2 || out != "" || !strings.Contains(diagnostics, "E_IO") || !strings.Contains(diagnostics, "not a regular file") {
		t.Fatalf("fifo swap %v: exit=%d stdout=%q stderr=%q", args, code, out, diagnostics)
	}
	requireStateUnchanged(t, root, before, mtime, identity)
	requireOnlyStateFile(t, root)
}
