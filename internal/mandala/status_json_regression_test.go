package mandala

import (
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"
)

type statusFileSnapshot struct {
	data  string
	size  int64
	mtime time.Time
	mode  os.FileMode
}

func snapshotStatusFile(t *testing.T, path string) statusFileSnapshot {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	return statusFileSnapshot{string(data), info.Size(), info.ModTime(), info.Mode()}
}

func TestRunStatusJSONReadOnlyAndDeterministic(t *testing.T) {
	root := initFixtureRepository(t, map[string]string{".gitignore": "# keep\n"})
	initStatusFixture(t, root)
	statePath := filepath.Join(root, ".mandala", "state.json")
	paths := []string{statePath, filepath.Join(root, ".gitignore"), fixtureExcludePath(t, root)}
	before := make([]statusFileSnapshot, len(paths))
	for i, path := range paths {
		before[i] = snapshotStatusFile(t, path)
	}
	for range 3 {
		code, out, diagnostics := runTestCommand(root, "status", "--json")
		if code != 1 || out != oauthStatusJSON || diagnostics != "" {
			t.Fatalf("repeat: exit=%d stdout=%q stderr=%q", code, out, diagnostics)
		}
	}
	for i, path := range paths {
		if after := snapshotStatusFile(t, path); after != before[i] {
			t.Fatalf("status modified %s: before=%+v after=%+v", path, before[i], after)
		}
	}
	entries, err := os.ReadDir(filepath.Join(root, ".mandala"))
	if err != nil || len(entries) != 1 || entries[0].Name() != "state.json" {
		t.Fatalf("unexpected state files: %v / %v", entries, err)
	}
	s, err := Load(root)
	if err != nil {
		t.Fatal(err)
	}
	slices.Reverse(s.Cells)
	if err := s.Validate(); err != nil {
		t.Fatal(err)
	}
	data, err := json.Marshal(s)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(statePath, data, 0600); err != nil {
		t.Fatal(err)
	}
	permuted := snapshotStatusFile(t, statePath)
	code, out, diagnostics := runTestCommand(root, "status", "--json")
	if code != 1 || out != oauthStatusJSON || diagnostics != "" {
		t.Fatalf("permuted state: exit=%d stdout=%q stderr=%q", code, out, diagnostics)
	}
	if after := snapshotStatusFile(t, statePath); after != permuted {
		t.Fatal("status rewrote permuted state")
	}
}

func TestRunStatusJSONDoesNotCreateGitIgnoreFiles(t *testing.T) {
	root := t.TempDir()
	runFixtureGit(t, root, nil, "init", "--quiet")
	if code, _, diagnostics := runTestCommand(root, "init", "Example"); code != 0 {
		t.Fatal(diagnostics)
	}
	exclude := fixtureExcludePath(t, root)
	if err := os.Remove(exclude); err != nil {
		t.Fatal(err)
	}
	code, out, diagnostics := runTestCommand(root, "status", "--json")
	if code != 0 || !json.Valid([]byte(out)) || diagnostics != "" {
		t.Fatalf("status: exit=%d stdout=%q stderr=%q", code, out, diagnostics)
	}
	for _, path := range []string{exclude, filepath.Join(root, ".gitignore")} {
		if _, err := os.Stat(path); !os.IsNotExist(err) {
			t.Fatalf("status created ignore file %s: %v", path, err)
		}
	}
}

func TestRunStatusJSONExistingOutputsUnchanged(t *testing.T) {
	root := t.TempDir()
	initStatusFixture(t, root)
	text := "goal: Implement OAuth\ncells: 6\ngroups: 1\nrequired: open=2 done=1 na=0\noptional: open=1 done=1 na=0\nrequired gaps: 2\n"
	for _, tt := range []struct {
		args []string
		code int
		want string
	}{
		{[]string{"status"}, 1, text},
		{[]string{"status", "--json=false"}, 1, text},
		{[]string{"gaps"}, 1, "compatibility\ndocs [optional]\ntests\n"},
		{[]string{"gaps", "--json"}, 1, `{"schema_version":1,"gaps":[{"id":"compatibility","required":true},{"id":"docs","required":false},{"id":"tests","required":true}]}` + "\n"},
		{[]string{"gaps", "--required", "--json"}, 1, `{"schema_version":1,"gaps":[{"id":"compatibility","required":true},{"id":"tests","required":true}]}` + "\n"},
		{[]string{"show"}, 0, "goal: Implement OAuth\ncompatibility [open, required]\ndocs [open, optional]\nperformance [done, optional]\nsecurity [expanded, required]\n  security.csrf [done, required]\ntests [open, required]\n"},
		{[]string{"show", "--json"}, 0, `{"schema_version":1,"goal":"Implement OAuth","cells":[{"id":"compatibility","parent":"","status":"open","required":true},{"id":"docs","parent":"","status":"open","required":false},{"id":"performance","parent":"","status":"done","required":false},{"id":"security","parent":"","status":"expanded","required":true},{"id":"security.csrf","parent":"security","status":"done","required":true},{"id":"tests","parent":"","status":"open","required":true}]}` + "\n"},
	} {
		code, out, diagnostics := runTestCommand(root, tt.args...)
		if code != tt.code || out != tt.want || diagnostics != "" {
			t.Fatalf("%v: exit=%d stdout=%q stderr=%q", tt.args, code, out, diagnostics)
		}
	}
}

func TestRunStatusJSONSelectsExplicitProject(t *testing.T) {
	root, other := t.TempDir(), t.TempDir()
	initStatusFixture(t, root)
	if code, out, diagnostics := runTestCommand(other, "init", "Other"); code != 0 || out != "" || diagnostics != "" {
		t.Fatalf("other init: exit=%d stdout=%q stderr=%q", code, out, diagnostics)
	}
	for _, start := range []string{root, other} {
		code, out, diagnostics := runTestCommand(start, "--project", root, "status", "--json")
		if code != 1 || out != oauthStatusJSON || diagnostics != "" {
			t.Fatalf("explicit target: exit=%d stdout=%q stderr=%q", code, out, diagnostics)
		}
	}
	code, out, diagnostics := runTestCommand(root, "--project", other, "status", "--json")
	want := `{"schema_version":1,"goal":"Other","cells":0,"groups":0,"required":{"open":0,"done":0,"na":0},"optional":{"open":0,"done":0,"na":0},"required_gaps":0}` + "\n"
	if code != 0 || out != want || diagnostics != "" {
		t.Fatalf("other target: exit=%d stdout=%q stderr=%q", code, out, diagnostics)
	}
}

func TestRunStatusJSONBrokenNearestBoundary(t *testing.T) {
	root := t.TempDir()
	initStatusFixture(t, root)
	before := snapshotStatusFile(t, filepath.Join(root, ".mandala", "state.json"))
	sub := filepath.Join(root, "sub")
	if err := os.MkdirAll(filepath.Join(sub, ".mandala"), 0700); err != nil {
		t.Fatal(err)
	}
	code, out, diagnostics := runTestCommand(sub, "status", "--json")
	if code != 2 || out != "" || !strings.Contains(diagnostics, "E_NO_PROJECT") {
		t.Fatalf("broken boundary: exit=%d stdout=%q stderr=%q", code, out, diagnostics)
	}
	entries, err := os.ReadDir(filepath.Join(sub, ".mandala"))
	if err != nil || len(entries) != 0 {
		t.Fatalf("status repaired boundary: entries=%v err=%v", entries, err)
	}
	if after := snapshotStatusFile(t, filepath.Join(root, ".mandala", "state.json")); after != before {
		t.Fatal("broken boundary changed ancestor state")
	}
}

func TestRunStatusJSONSymlinkProject(t *testing.T) {
	root := t.TempDir()
	initStatusFixture(t, root)
	link := filepath.Join(t.TempDir(), "project")
	if err := os.Symlink(root, link); err != nil {
		t.Skipf("symlink unavailable: %v", err)
	}
	for _, args := range [][]string{{"status", "--json"}, {"--project", link, "status", "--json"}} {
		code, out, diagnostics := runTestCommand(link, args...)
		if code != 1 || out != oauthStatusJSON || diagnostics != "" {
			t.Fatalf("symlink: exit=%d stdout=%q stderr=%q", code, out, diagnostics)
		}
	}
}
