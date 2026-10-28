package main

import (
	"bytes"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"syscall"
	"testing"
	"time"
)

// holdStateWithoutShareDelete opens state.json the way an editor or scanner
// might, with read access and FILE_SHARE_READ|FILE_SHARE_WRITE but without
// FILE_SHARE_DELETE. Other processes can still read and open the file, but
// replacing it by rename needs delete access and fails while this handle is
// open. That makes the real writer's final rename fail inside the CLI.
//
// A read-only attribute was the other candidate. The handle is preferred
// because it leaves the file's metadata alone, is undone by closing the
// handle, and cannot leave an undeletable file behind if the test aborts.
func holdStateWithoutShareDelete(t *testing.T, path string) *os.File {
	t.Helper()
	name, err := syscall.UTF16PtrFromString(path)
	if err != nil {
		t.Fatal(err)
	}
	h, err := syscall.CreateFile(name, syscall.GENERIC_READ,
		syscall.FILE_SHARE_READ|syscall.FILE_SHARE_WRITE,
		nil, syscall.OPEN_EXISTING, syscall.FILE_ATTRIBUTE_NORMAL, 0)
	if err != nil {
		t.Fatalf("hold state without FILE_SHARE_DELETE: %v", err)
	}
	f := os.NewFile(uintptr(h), path)
	t.Cleanup(func() { f.Close() })
	return f
}

// windowsFileIdentity reads the file ID from an open handle. File.Stat is
// eager, while os.Stat loads the ID lazily from the path on first SameFile.
func windowsFileIdentity(t *testing.T, path string) os.FileInfo {
	t.Helper()
	f, err := os.Open(path)
	if err != nil {
		t.Fatalf("open state: %v", err)
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil {
		t.Fatalf("stat state: %v", err)
	}
	return info
}

type windowsStateSnapshot struct {
	data     []byte
	modTime  time.Time
	identity os.FileInfo
}

func snapshotWindowsState(t *testing.T, path string) windowsStateSnapshot {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	return windowsStateSnapshot{data: data, modTime: info.ModTime(), identity: windowsFileIdentity(t, path)}
}

func (want windowsStateSnapshot) requireUnchanged(t *testing.T, path string) {
	t.Helper()
	got := snapshotWindowsState(t, path)
	if !bytes.Equal(got.data, want.data) {
		t.Fatalf("state bytes changed:\nbefore=%s\nafter=%s", want.data, got.data)
	}
	if !got.modTime.Equal(want.modTime) {
		t.Fatalf("state mtime changed: before=%v after=%v", want.modTime, got.modTime)
	}
	if !os.SameFile(want.identity, got.identity) {
		t.Fatal("state file identity changed: state.json was replaced")
	}
}

func requireOnlyStateJSON(t *testing.T, stateDir string) {
	t.Helper()
	entries, err := os.ReadDir(stateDir)
	if err != nil {
		t.Fatal(err)
	}
	names := make([]string, 0, len(entries))
	for _, entry := range entries {
		names = append(names, entry.Name())
	}
	if !slices.Equal(names, []string{"state.json"}) {
		t.Fatalf("unexpected .mandala contents (temporary state left behind?): %v", names)
	}
}

// TestCLIBatchWindowsSaveFailure drives the real binary against a real OS
// failure inside the real state writer: an open handle without
// FILE_SHARE_DELETE on state.json lets Load and temporary-file creation
// succeed and makes the final rename fail. Nothing is mocked.
func TestCLIBatchWindowsSaveFailure(t *testing.T) {
	binary := buildCLI(t)
	root := t.TempDir()
	if code, _, diagnostics := invokeCLI(t, binary, root, "init", "goal"); code != 0 {
		t.Fatal(diagnostics)
	}
	stateDir := filepath.Join(root, ".mandala")
	statePath := filepath.Join(stateDir, "state.json")
	ops := writeIntegrationBatchFile(t, `{"batch_version":1,"operations":[{"op":"add","id":"alpha"}]}`)
	before := snapshotWindowsState(t, statePath)

	held := holdStateWithoutShareDelete(t, statePath)

	for _, args := range [][]string{
		{"batch", "--file", ops, "--dry-run"},
		{"batch", "--file", ops, "--dry-run", "--json"},
	} {
		code, out, diagnostics := invokeCLI(t, binary, root, args...)
		if code != 0 || out == "" || diagnostics != "" {
			t.Fatalf("dry-run %v with held state: exit=%d stdout=%q stderr=%q", args, code, out, diagnostics)
		}
		before.requireUnchanged(t, statePath)
		requireOnlyStateJSON(t, stateDir)
	}

	for _, args := range [][]string{
		{"batch", "--file", ops},
		{"batch", "--file", ops, "--json"},
	} {
		code, out, diagnostics := invokeCLI(t, binary, root, args...)
		if code != 2 || out != "" || !strings.Contains(diagnostics, "E_IO") || !strings.Contains(diagnostics, "replace state") {
			t.Fatalf("apply %v with held state: exit=%d stdout=%q stderr=%q", args, code, out, diagnostics)
		}
		before.requireUnchanged(t, statePath)
		requireOnlyStateJSON(t, stateDir)
	}

	if err := held.Close(); err != nil {
		t.Fatal(err)
	}
	code, out, diagnostics := invokeCLI(t, binary, root, "batch", "--file", ops)
	if code != 0 || out != "" || diagnostics != "" {
		t.Fatalf("apply after releasing state: exit=%d stdout=%q stderr=%q", code, out, diagnostics)
	}
	after, err := os.ReadFile(statePath)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(after), `"id": "alpha"`) {
		t.Fatalf("apply after release did not save alpha:\n%s", after)
	}
	requireOnlyStateJSON(t, stateDir)
}
