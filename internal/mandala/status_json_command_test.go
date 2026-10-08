package mandala

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

const oauthStatusJSON = `{"schema_version":1,"goal":"Implement OAuth","cells":6,"groups":1,"required":{"open":2,"done":1,"na":0},"optional":{"open":1,"done":1,"na":0},"required_gaps":2}` + "\n"

func initStatusFixture(t *testing.T, root string) {
	t.Helper()
	for _, args := range [][]string{
		{"init", "Implement OAuth"}, {"add", "security"}, {"add", "tests"}, {"add", "compatibility"},
		{"add", "security.csrf"}, {"done", "security.csrf"},
		{"add", "--optional", "docs"}, {"add", "--optional", "performance"}, {"done", "performance"},
	} {
		if code, out, diagnostics := runTestCommand(root, args...); code != 0 || out != "" || diagnostics != "" {
			t.Fatalf("%v: exit=%d stdout=%q stderr=%q", args, code, out, diagnostics)
		}
	}
}

func TestRunStatusJSONProjectResolutionAndFlags(t *testing.T) {
	root, other := t.TempDir(), t.TempDir()
	initStatusFixture(t, root)
	sub := filepath.Join(root, "internal", "auth")
	if err := os.MkdirAll(sub, 0700); err != nil {
		t.Fatal(err)
	}
	relative, err := filepath.Rel(other, root)
	if err != nil {
		t.Fatal(err)
	}
	for _, tt := range []struct {
		start string
		args  []string
	}{
		{root, []string{"status", "--json"}},
		{sub, []string{"status", "--json"}},
		{root, []string{"status", "--json=true"}},
		{other, []string{"--project", root, "status", "--json"}},
		{other, []string{"--project", relative, "status", "--json"}},
	} {
		code, out, diagnostics := runTestCommand(tt.start, tt.args...)
		if code != 1 || out != oauthStatusJSON || diagnostics != "" {
			t.Fatalf("%v from %s: exit=%d stdout=%q stderr=%q", tt.args, tt.start, code, out, diagnostics)
		}
	}
}

func TestRunStatusJSONExitCodes(t *testing.T) {
	for _, tt := range []struct {
		name     string
		commands [][]string
		code     int
		want     string
	}{
		{"empty", nil, 0, `{"schema_version":1,"goal":"Example","cells":0,"groups":0,"required":{"open":0,"done":0,"na":0},"optional":{"open":0,"done":0,"na":0},"required_gaps":0}` + "\n"},
		{"required open", [][]string{{"add", "a"}}, 1, `{"schema_version":1,"goal":"Example","cells":1,"groups":0,"required":{"open":1,"done":0,"na":0},"optional":{"open":0,"done":0,"na":0},"required_gaps":1}` + "\n"},
		{"optional inheritance", [][]string{{"add", "--optional", "a"}, {"add", "a.b"}}, 0, `{"schema_version":1,"goal":"Example","cells":2,"groups":1,"required":{"open":0,"done":0,"na":0},"optional":{"open":1,"done":0,"na":0},"required_gaps":0}` + "\n"},
		{"all required resolved", [][]string{{"add", "a"}, {"done", "a"}, {"add", "b"}, {"mark", "b", "na"}}, 0, `{"schema_version":1,"goal":"Example","cells":2,"groups":0,"required":{"open":0,"done":1,"na":1},"optional":{"open":0,"done":0,"na":0},"required_gaps":0}` + "\n"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			root := t.TempDir()
			commands := append([][]string{{"init", "Example"}}, tt.commands...)
			for _, args := range commands {
				if code, out, diagnostics := runTestCommand(root, args...); code != 0 || out != "" || diagnostics != "" {
					t.Fatalf("%v: exit=%d stdout=%q stderr=%q", args, code, out, diagnostics)
				}
			}
			code, out, diagnostics := runTestCommand(root, "status", "--json")
			if code != tt.code || out != tt.want || !json.Valid([]byte(out)) || diagnostics != "" {
				t.Fatalf("status: exit=%d stdout=%q stderr=%q", code, out, diagnostics)
			}
		})
	}
}

func TestRunStatusJSONErrorsAndParsingPrecedence(t *testing.T) {
	root := t.TempDir()
	for _, args := range [][]string{
		{"status", "--unknown"}, {"status", "--json", "extra"}, {"--json", "status"},
		{"status", "--project", root}, {"status", "--json=invalid"}, {"status", "--required"},
		{"status", "--json=false", "extra"},
	} {
		code, out, diagnostics := runTestCommand(filepath.Join(root, "missing"), args...)
		if code != 2 || out != "" || !strings.Contains(diagnostics, "E_USAGE") {
			t.Fatalf("%v: exit=%d stdout=%q stderr=%q", args, code, out, diagnostics)
		}
	}
	code, out, diagnostics := runTestCommand(root, "status", "--json")
	if code != 2 || out != "" || !strings.Contains(diagnostics, "E_NO_PROJECT") {
		t.Fatalf("missing project: exit=%d stdout=%q stderr=%q", code, out, diagnostics)
	}
	if _, err := os.Stat(filepath.Join(root, ".mandala")); !os.IsNotExist(err) {
		t.Fatalf("status created a state boundary: %v", err)
	}
	if code, _, diagnostics := runTestCommand(root, "init", "Example"); code != 0 {
		t.Fatal(diagnostics)
	}
	path := filepath.Join(root, ".mandala", "state.json")
	for _, input := range []string{
		"{broken",
		`{"schema_version":2,"goal":"Example","cells":[]}`,
		`{"schema_version":1,"goal":"Example","cells":[{"id":"a","parent":"","status":"expanded","required":true}]}`,
	} {
		if err := os.WriteFile(path, []byte(input), 0600); err != nil {
			t.Fatal(err)
		}
		before := snapshotStatusFile(t, path)
		code, out, diagnostics = runTestCommand(root, "status", "--json")
		if code != 2 || out != "" || !strings.Contains(diagnostics, "E_STATE_INVALID") {
			t.Fatalf("invalid state: exit=%d stdout=%q stderr=%q", code, out, diagnostics)
		}
		if after := snapshotStatusFile(t, path); after != before {
			t.Fatal("status repaired or changed invalid state")
		}
	}
}

func TestRunStatusHelp(t *testing.T) {
	code, out, diagnostics := runTestCommand(t.TempDir(), "status", "--help")
	if code != 0 || out != "Usage: mandala [--project DIR] status [--json]\n" || diagnostics != "" {
		t.Fatalf("help: exit=%d stdout=%q stderr=%q", code, out, diagnostics)
	}
}

func TestRunStatusJSONBooleanAndHelpPrecedence(t *testing.T) {
	root := t.TempDir()
	initStatusFixture(t, root)
	text := "goal: Implement OAuth\ncells: 6\ngroups: 1\nrequired: open=2 done=1 na=0\noptional: open=1 done=1 na=0\nrequired gaps: 2\n"
	statePath := filepath.Join(root, ".mandala", "state.json")
	before := snapshotStatusFile(t, statePath)
	for _, tt := range []struct {
		args []string
		want string
	}{
		{[]string{"status", "--json", "--json=false"}, text},
		{[]string{"status", "--json=false", "--json"}, oauthStatusJSON},
	} {
		code, out, diagnostics := runTestCommand(root, tt.args...)
		if code != 1 || out != tt.want || diagnostics != "" {
			t.Fatalf("%v: exit=%d stdout=%q stderr=%q", tt.args, code, out, diagnostics)
		}
	}
	for _, args := range [][]string{
		{"status", "--json=false", "extra"}, {"status", "--json", "init", "unexpected"},
	} {
		code, out, diagnostics := runTestCommand(root, args...)
		if code != 2 || out != "" || !strings.Contains(diagnostics, "E_USAGE") {
			t.Fatalf("%v: exit=%d stdout=%q stderr=%q", args, code, out, diagnostics)
		}
	}
	if after := snapshotStatusFile(t, statePath); after != before {
		t.Fatal("boolean parsing changed state")
	}
	missing := filepath.Join(t.TempDir(), "missing")
	for _, args := range [][]string{{"status", "--help"}, {"status", "--json", "--help"}} {
		code, out, diagnostics := runTestCommand(missing, args...)
		if code != 0 || out != "Usage: mandala [--project DIR] status [--json]\n" || diagnostics != "" {
			t.Fatalf("help without directory: %v: exit=%d stdout=%q stderr=%q", args, code, out, diagnostics)
		}
	}
}

type statusFailingWriter struct {
	partial bool
	output  bytes.Buffer
}

func (w *statusFailingWriter) Write(p []byte) (int, error) {
	n := 0
	if w.partial {
		n, _ = w.output.Write(p[:5])
	}
	return n, errors.New("output unavailable")
}

func TestRunStatusJSONWriterError(t *testing.T) {
	root := t.TempDir()
	initStatusFixture(t, root)
	for _, partial := range []bool{false, true} {
		writer := &statusFailingWriter{partial: partial}
		var diagnostics bytes.Buffer
		code := Run([]string{"status", "--json"}, root, writer, &diagnostics)
		want := ""
		if partial {
			want = oauthStatusJSON[:5]
		}
		if code != 2 || writer.output.String() != want || !strings.Contains(diagnostics.String(), "E_IO") {
			t.Fatalf("writer: exit=%d stdout=%q stderr=%q", code, writer.output.String(), diagnostics.String())
		}
	}
}

func TestRunStatusJSONStateIOError(t *testing.T) {
	if runtime.GOOS == "windows" || os.Geteuid() == 0 {
		t.Skip("requires Unix file permissions and a non-root user")
	}
	root := t.TempDir()
	initStatusFixture(t, root)
	path := filepath.Join(root, ".mandala", "state.json")
	before := snapshotStatusFile(t, path)
	if err := os.Chmod(path, 0); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := os.Chmod(path, before.mode.Perm()); err != nil {
			t.Error(err)
		}
	})
	code, out, diagnostics := runTestCommand(root, "status", "--json")
	if code != 2 || out != "" || !strings.Contains(diagnostics, "E_IO") {
		t.Fatalf("state I/O: exit=%d stdout=%q stderr=%q", code, out, diagnostics)
	}
	info, err := os.Stat(path)
	if err != nil || info.Size() != before.size || !info.ModTime().Equal(before.mtime) || info.Mode().Perm() != 0 {
		t.Fatalf("state I/O changed metadata: info=%v err=%v", info, err)
	}
	if err := os.Chmod(path, before.mode.Perm()); err != nil {
		t.Fatal(err)
	}
	if after := snapshotStatusFile(t, path); after != before {
		t.Fatal("state I/O changed state")
	}
}
