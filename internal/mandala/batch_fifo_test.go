//go:build unix

package mandala

import (
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"
)

// TestBatchRejectsFIFO guards the stat-before-open ordering in
// readBatchInput. Opening a FIFO with no writer blocks forever, so a FIFO
// must be rejected as a non-regular file before the open, and the command
// must fail fast with E_IO.
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
