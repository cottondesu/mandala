package mandala

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestBatchInputWindowsSymlink covers the Windows snapshot, which follows
// symlinks through CreateFile: a symlink to a regular file is accepted, and
// retargeting it after the snapshot is rejected. Symlink creation failure is a
// test failure, not a skip, so a runner without the privilege is visible.
func TestBatchInputWindowsSymlink(t *testing.T) {
	target := writeBatchFile(t, batchDocument(batchAdd("beta")))
	other := writeBatchFile(t, batchDocument(batchAdd("gamma")))
	link := filepath.Join(t.TempDir(), "link.json")
	if err := os.Symlink(target, link); err != nil {
		t.Fatalf("create symlink to regular file: %v", err)
	}

	t.Run("accepted", func(t *testing.T) {
		root := initBatchProject(t, "Windows symlink")
		code, out, diagnostics := runBatchCommand(root, "", "--file", link, "--dry-run")
		if code != 0 || !strings.Contains(out, "+ beta") || diagnostics != "" {
			t.Fatalf("symlink input: exit=%d stdout=%q stderr=%q", code, out, diagnostics)
		}
	})

	t.Run("retarget-rejected", func(t *testing.T) {
		root := initBatchProject(t, "Windows symlink retarget")
		retargeted := 0
		withBatchInputOpenHook(t, func(string) {
			retargeted++
			if err := os.Remove(link); err != nil {
				t.Fatalf("remove symlink: %v", err)
			}
			if err := os.Symlink(other, link); err != nil {
				t.Fatalf("retarget symlink: %v", err)
			}
		})
		before := readBatchState(t, root)
		mtime := batchStateModTime(t, root)
		identity := batchStateIdentity(t, root)
		code, out, diagnostics := runBatchCommand(root, "", "--file", link)
		if retargeted != 1 {
			t.Fatalf("retarget hook ran %d times, want 1", retargeted)
		}
		if code != 2 || out != "" || !strings.Contains(diagnostics, "E_IO") || !strings.Contains(diagnostics, "changed while opening") {
			t.Fatalf("retargeted symlink: exit=%d stdout=%q stderr=%q", code, out, diagnostics)
		}
		requireStateUnchanged(t, root, before, mtime, identity)
		requireOnlyStateFile(t, root)
	})
}

// TestBatchInputWindowsLongPath reads an input through an absolute path longer
// than MAX_PATH, which the CreateFile snapshot must prefix itself because it
// does not go through os.Open's long path handling.
func TestBatchInputWindowsLongPath(t *testing.T) {
	dir := t.TempDir()
	for len(dir) < 300 {
		dir = filepath.Join(dir, strings.Repeat("d", 60))
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatalf("create long directory: %v", err)
	}
	path := filepath.Join(dir, "ops.json")
	if err := os.WriteFile(path, []byte(batchDocument(batchAdd("beta"))), 0600); err != nil {
		t.Fatalf("create long input path: %v", err)
	}
	if len(path) <= 260 {
		t.Fatalf("input path has %d characters, want more than 260", len(path))
	}
	root := initBatchProject(t, "Windows long path")
	code, out, diagnostics := runBatchCommand(root, "", "--file", path, "--dry-run")
	if code != 0 || !strings.Contains(out, "+ beta") || diagnostics != "" {
		t.Fatalf("long path input: exit=%d stdout=%q stderr=%q", code, out, diagnostics)
	}
}
