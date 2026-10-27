//go:build unix

package mandala

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestBatchInputSymlinkToRegular keeps the documented compatibility: an
// ordinary symlink to a regular file is accepted.
func TestBatchInputSymlinkToRegular(t *testing.T) {
	root := initBatchProject(t, "Symlink input")
	target := writeBatchFile(t, batchDocument(batchAdd("beta")))
	link := filepath.Join(t.TempDir(), "link.json")
	if err := os.Symlink(target, link); err != nil {
		t.Fatal(err)
	}
	code, out, diagnostics := runBatchCommand(root, "", "--file", link, "--dry-run")
	if code != 0 || !strings.Contains(out, "+ beta") || diagnostics != "" {
		t.Fatalf("symlink input: exit=%d stdout=%q stderr=%q", code, out, diagnostics)
	}
}

// TestBatchInputSymlinkRetarget points the symlink at a different regular file
// between the snapshot and the open. Identity follows the target, so the
// retarget is rejected.
func TestBatchInputSymlinkRetarget(t *testing.T) {
	root := initBatchProject(t, "Symlink retarget")
	target := writeBatchFile(t, batchDocument(batchAdd("beta")))
	other := writeBatchFile(t, batchDocument(batchAdd("gamma")))
	link := filepath.Join(t.TempDir(), "link.json")
	if err := os.Symlink(target, link); err != nil {
		t.Fatal(err)
	}
	withBatchInputOpenHook(t, func(string) {
		if err := os.Remove(link); err != nil {
			t.Fatal(err)
		}
		if err := os.Symlink(other, link); err != nil {
			t.Fatal(err)
		}
	})
	before := readBatchState(t, root)
	mtime := batchStateModTime(t, root)
	identity := batchStateIdentity(t, root)
	code, out, diagnostics := runBatchCommand(root, "", "--file", link)
	if code != 2 || out != "" || !strings.Contains(diagnostics, "E_IO") || !strings.Contains(diagnostics, "changed while opening") {
		t.Fatalf("retargeted symlink: exit=%d stdout=%q stderr=%q", code, out, diagnostics)
	}
	requireStateUnchanged(t, root, before, mtime, identity)
	requireOnlyStateFile(t, root)
}
