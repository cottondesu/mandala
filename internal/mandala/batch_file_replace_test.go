package mandala

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// withBatchInputOpenHook runs hook at the data-open step of readBatchInputFile,
// after the identity snapshot and before the file is opened.
func withBatchInputOpenHook(t *testing.T, hook func(path string)) {
	t.Helper()
	original := batchOpenInputFile
	batchOpenInputFile = func(path string) (*os.File, error) {
		hook(path)
		return original(path)
	}
	t.Cleanup(func() { batchOpenInputFile = original })
}

// replaceBatchInputAtSamePath moves the original aside and puts a different
// regular file at the same path. Renames work while the identity snapshot
// handle is open on Windows because the handle shares delete.
func replaceBatchInputAtSamePath(t *testing.T, path, content string) {
	t.Helper()
	if err := os.Rename(path, path+".orig"); err != nil {
		t.Fatalf("move original input aside: %v", err)
	}
	if err := os.WriteFile(path+".new", []byte(content), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(path+".new", path); err != nil {
		t.Fatalf("place replacement input: %v", err)
	}
}

// TestBatchInputSamePathReplacement replaces the input file at the original
// path between the identity snapshot and the data open. The replacement is a
// valid batch that would change state if it were read, so the command must
// refuse it with E_IO before touching state.
func TestBatchInputSamePathReplacement(t *testing.T) {
	replacement := batchDocument(batchMark("alpha", "done"))
	cases := []struct {
		name string
		args []string
	}{
		{"dry-run", []string{"--dry-run"}},
		{"dry-run-json", []string{"--dry-run", "--json"}},
		{"apply", nil},
		{"apply-json", []string{"--json"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			root := initBatchProject(t, "Same path replacement")
			if code, _, diagnostics := runTestCommand(root, "add", "alpha"); code != 0 {
				t.Fatal(diagnostics)
			}
			path := filepath.Join(t.TempDir(), "ops.json")
			if err := os.WriteFile(path, []byte(batchDocument(batchAdd("beta"))), 0600); err != nil {
				t.Fatal(err)
			}
			replaced := 0
			withBatchInputOpenHook(t, func(p string) {
				replaced++
				replaceBatchInputAtSamePath(t, p, replacement)
			})

			before := readBatchState(t, root)
			mtime := batchStateModTime(t, root)
			identity := batchStateIdentity(t, root)
			code, out, diagnostics := runBatchCommand(root, "", append([]string{"--file", path}, tc.args...)...)
			if replaced != 1 {
				t.Fatalf("replacement hook ran %d times, want 1", replaced)
			}
			if code != 2 || out != "" {
				t.Fatalf("replaced input: exit=%d stdout=%q stderr=%q", code, out, diagnostics)
			}
			if !strings.Contains(diagnostics, "E_IO") || !strings.Contains(diagnostics, "changed while opening") {
				t.Fatalf("replaced input: stderr=%q", diagnostics)
			}
			requireStateUnchanged(t, root, before, mtime, identity)
			requireOnlyStateFile(t, root)
		})
	}

	t.Run("control-unreplaced", func(t *testing.T) {
		root := initBatchProject(t, "Same path control")
		path := filepath.Join(t.TempDir(), "ops.json")
		if err := os.WriteFile(path, []byte(batchDocument(batchAdd("beta"))), 0600); err != nil {
			t.Fatal(err)
		}
		opened := 0
		withBatchInputOpenHook(t, func(string) { opened++ })
		code, out, diagnostics := runBatchCommand(root, "", "--file", path, "--dry-run")
		if opened != 1 || code != 0 || !strings.Contains(out, "+ beta") || diagnostics != "" {
			t.Fatalf("unreplaced input: opened=%d exit=%d stdout=%q stderr=%q", opened, code, out, diagnostics)
		}
	})
}
