package mandala

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestProjectRootFindsNearestAncestor(t *testing.T) {
	root := t.TempDir()
	if err := os.Mkdir(filepath.Join(root, ".mandala"), 0700); err != nil {
		t.Fatal(err)
	}
	sub := filepath.Join(root, "internal", "auth")
	if err := os.MkdirAll(sub, 0700); err != nil {
		t.Fatal(err)
	}
	got, err := ProjectRoot(sub, "")
	want, err2 := filepath.EvalSymlinks(root)
	if err2 != nil {
		t.Fatal(err2)
	}
	if err != nil || got != want {
		t.Fatalf("root = %q, err = %v", got, err)
	}
}

func TestLoadRejectsMalformedState(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, ".mandala")
	if err := os.Mkdir(dir, 0700); err != nil {
		t.Fatal(err)
	}
	for _, input := range []string{
		`{broken`,
		`{"schema_version":1,"goal":"goal","cells":[{"id":"a","parent":"","status":"open"}]}`,
		`{"schema_version":2,"goal":"goal","cells":[]}`,
		`{"schema_version":1,"goal":"goal","cells":[]} {}`,
		`{"schema_version":1,"goal":"first","goal":"second","cells":[]}`,
		`{"schema_version":1,"goal":"goal","cells":[{"id":"a","parent":"","status":"open","required":true,"required":false}]}`,
		`{"schema_version":1,"goal":"goal","cells":[{"id":"a","parent":"","status":"open","required":true,"Required":false}]}`,
		`{"schema_version":1,"Goal":"goal","cells":[]}`,
		`{"schema_version":1,"goal":"goal","cells":[],"cellſ":[]}`,
	} {
		if err := os.WriteFile(filepath.Join(dir, "state.json"), []byte(input), 0600); err != nil {
			t.Fatal(err)
		}
		if _, err := Load(root); err == nil {
			t.Fatalf("accepted malformed state: %s", input)
		}
	}
}

func TestSaveReplacesCompleteJSON(t *testing.T) {
	root := t.TempDir()
	s, err := NewState("goal")
	if err != nil {
		t.Fatal(err)
	}
	if err := Init(root, s); err != nil {
		t.Fatal(err)
	}
	if err := s.Add("b", false); err != nil {
		t.Fatal(err)
	}
	if err := s.Add("a", false); err != nil {
		t.Fatal(err)
	}
	if err := Save(root, s); err != nil {
		t.Fatal(err)
	}
	loaded, err := Load(root)
	if err != nil || len(loaded.Cells) != 2 {
		t.Fatalf("loaded = %#v, err = %v", loaded, err)
	}
	data, err := os.ReadFile(filepath.Join(root, ".mandala", "state.json"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Index(string(data), `"id": "a"`) > strings.Index(string(data), `"id": "b"`) {
		t.Fatal("saved cells are not sorted")
	}
	entries, err := os.ReadDir(filepath.Join(root, ".mandala"))
	if err != nil || len(entries) != 1 {
		t.Fatalf("state directory entries = %v, err = %v", entries, err)
	}
}

func TestCleanPreservesUnrelatedFileAndIsIdempotent(t *testing.T) {
	root := t.TempDir()
	s, _ := NewState("goal")
	if err := Init(root, s); err != nil {
		t.Fatal(err)
	}
	note := filepath.Join(root, ".mandala", "user-note.txt")
	if err := os.WriteFile(note, []byte("keep"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := Clean(root, ""); err != nil {
		t.Fatal(err)
	}
	if err := Clean(root, ""); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(note); err != nil {
		t.Fatalf("unrelated file removed: %v", err)
	}
	if _, err := os.Stat(filepath.Join(root, ".mandala", "state.json")); !os.IsNotExist(err) {
		t.Fatalf("state still exists: %v", err)
	}
}

func TestCleanRemovesEmptyOwnedDirectory(t *testing.T) {
	root := t.TempDir()
	s, _ := NewState("goal")
	if err := Init(root, s); err != nil {
		t.Fatal(err)
	}
	if err := Clean(root, ""); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(root, ".mandala")); !os.IsNotExist(err) {
		t.Fatalf("empty directory remains: %v", err)
	}
}

func TestSaveFailurePreservesExistingState(t *testing.T) {
	root := t.TempDir()
	s, _ := NewState("goal")
	if err := Init(root, s); err != nil {
		t.Fatal(err)
	}
	statePath := filepath.Join(root, ".mandala", "state.json")
	before, err := os.ReadFile(statePath)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.Add("bad.nested", false); err == nil {
		t.Fatal("missing parent accepted")
	}
	if err := Save(root, State{SchemaVersion: 1, Goal: "", Cells: []Cell{}}); err == nil {
		t.Fatal("invalid save accepted")
	}
	after, err := os.ReadFile(statePath)
	if err != nil || string(after) != string(before) {
		t.Fatalf("failed save changed state: %v", err)
	}
}

func TestCleanRefusesCorruptState(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, ".mandala")
	if err := os.Mkdir(dir, 0700); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "state.json")
	if err := os.WriteFile(path, []byte("not JSON"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := Clean(root, ""); err == nil {
		t.Fatal("clean deleted corrupt state")
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("corrupt state disappeared: %v", err)
	}
}

func TestProjectRootDoesNotSkipBrokenNearestProject(t *testing.T) {
	root := t.TempDir()
	s, _ := NewState("goal")
	if err := Init(root, s); err != nil {
		t.Fatal(err)
	}
	sub := filepath.Join(root, "sub")
	if err := os.MkdirAll(filepath.Join(sub, ".mandala"), 0700); err != nil {
		t.Fatal(err)
	}
	project, err := ProjectRoot(sub, "")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := Load(project); err == nil {
		t.Fatal("broken nearest project was skipped")
	}
}

func TestCleanRefusesSymlinkState(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, ".mandala")
	if err := os.Mkdir(dir, 0700); err != nil {
		t.Fatal(err)
	}
	target := filepath.Join(root, "user.json")
	if err := os.WriteFile(target, []byte("keep"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(target, filepath.Join(dir, "state.json")); err != nil {
		t.Skipf("symlink unavailable: %v", err)
	}
	if err := Clean(root, ""); err == nil {
		t.Fatal("clean followed a symlink")
	}
	data, err := os.ReadFile(target)
	if err != nil || string(data) != "keep" {
		t.Fatalf("symlink target changed: %q %v", data, err)
	}
}

func TestInitRefusesBrokenAncestorBoundary(t *testing.T) {
	root := t.TempDir()
	other := t.TempDir()
	if err := os.Symlink(other, filepath.Join(root, ".mandala")); err != nil {
		t.Skipf("symlink unavailable: %v", err)
	}
	sub := filepath.Join(root, "sub")
	if err := os.Mkdir(sub, 0700); err != nil {
		t.Fatal(err)
	}
	s, _ := NewState("goal")
	if err := Init(sub, s); err == nil {
		t.Fatal("initialized below a broken project boundary")
	}
	if _, err := os.Stat(filepath.Join(sub, ".mandala")); !os.IsNotExist(err) {
		t.Fatalf("unexpected nested project: %v", err)
	}
}

func TestCleanDoesNotDeleteAncestorProject(t *testing.T) {
	root := t.TempDir()
	s, _ := NewState("goal")
	if err := Init(root, s); err != nil {
		t.Fatal(err)
	}
	sub := filepath.Join(root, "sub")
	if err := os.Mkdir(sub, 0700); err != nil {
		t.Fatal(err)
	}
	if err := Clean(sub, ""); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(root, ".mandala", "state.json")); err != nil {
		t.Fatalf("clean deleted ancestor state: %v", err)
	}
}

func TestPinnedStateDirectoryDoesNotFollowReplacementSymlink(t *testing.T) {
	root := t.TempDir()
	s, _ := NewState("goal")
	if err := Init(root, s); err != nil {
		t.Fatal(err)
	}
	pinned, err := openMandalaDirectory(root)
	if err != nil {
		t.Fatal(err)
	}
	defer pinned.Close()
	statePath := filepath.Join(root, ".mandala", "state.json")
	original, err := os.ReadFile(statePath)
	if err != nil {
		t.Fatal(err)
	}
	other := t.TempDir()
	otherState := filepath.Join(other, "state.json")
	if err := os.WriteFile(otherState, original, 0600); err != nil {
		t.Fatal(err)
	}
	moved := filepath.Join(root, ".mandala-moved")
	if err := os.Rename(filepath.Join(root, ".mandala"), moved); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(other, filepath.Join(root, ".mandala")); err != nil {
		t.Skipf("symlink unavailable: %v", err)
	}
	if err := s.Add("security", false); err != nil {
		t.Fatal(err)
	}
	if err := writeState(pinned, s, true); err != nil {
		t.Fatal(err)
	}
	after, err := os.ReadFile(otherState)
	if err != nil || string(after) != string(original) {
		t.Fatalf("replacement directory state changed: %v", err)
	}
	updated, err := os.ReadFile(filepath.Join(moved, "state.json"))
	if err != nil || string(updated) == string(original) {
		t.Fatalf("pinned directory was not updated: %v", err)
	}
}
