package mandala

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func runTestCommand(start string, args ...string) (int, string, string) {
	var out, diagnostics bytes.Buffer
	code := Run(args, start, &out, &diagnostics)
	return code, out.String(), diagnostics.String()
}

func TestRunOAuthWorkflow(t *testing.T) {
	root := t.TempDir()
	commands := [][]string{
		{"init", "Implement OAuth"},
		{"add", "security"},
		{"add", "tests"},
		{"add", "compatibility"},
		{"add", "security.csrf"},
		{"add", "security.token-storage"},
		{"done", "security.csrf"},
	}
	for _, args := range commands {
		code, out, diagnostics := runTestCommand(root, args...)
		if code != 0 || out != "" || diagnostics != "" {
			t.Fatalf("%v: code=%d stdout=%q stderr=%q", args, code, out, diagnostics)
		}
	}
	code, out, diagnostics := runTestCommand(root, "gaps", "--required")
	if code != 1 || out != "compatibility\nsecurity.token-storage\ntests\n" || diagnostics != "" {
		t.Fatalf("gaps: code=%d stdout=%q stderr=%q", code, out, diagnostics)
	}
	for _, id := range []string{"compatibility", "security.token-storage", "tests"} {
		if code, _, diagnostics := runTestCommand(root, "mark", id, "na"); code != 0 {
			t.Fatalf("mark %s: code=%d stderr=%q", id, code, diagnostics)
		}
	}
	code, out, _ = runTestCommand(root, "gaps", "--required", "--json")
	if code != 0 || out != "{\"schema_version\":1,\"gaps\":[]}\n" {
		t.Fatalf("complete gaps: code=%d stdout=%q", code, out)
	}
	if _, err := os.Stat(filepath.Join(root, ".mandala", "state.json")); err != nil {
		t.Fatalf("completion removed state: %v", err)
	}
}

func TestRunJSONStableFromSubdirectory(t *testing.T) {
	root := t.TempDir()
	for _, args := range [][]string{{"init", "goal"}, {"add", "z"}, {"add", "a"}} {
		if code, _, diagnostics := runTestCommand(root, args...); code != 0 {
			t.Fatalf("%v: %d %s", args, code, diagnostics)
		}
	}
	sub := filepath.Join(root, "internal", "auth")
	if err := os.MkdirAll(sub, 0700); err != nil {
		t.Fatal(err)
	}
	for _, args := range [][]string{{"gaps", "--json"}, {"show", "--json"}} {
		rootCode, rootOut, rootErr := runTestCommand(root, args...)
		subCode, subOut, subErr := runTestCommand(sub, args...)
		if rootCode != subCode || rootOut != subOut || rootErr != subErr {
			t.Fatalf("%v differs from subdirectory", args)
		}
		if !json.Valid([]byte(rootOut)) || rootErr != "" {
			t.Fatalf("%v invalid JSON or stderr: %q %q", args, rootOut, rootErr)
		}
	}
}

func TestRunErrorSeparation(t *testing.T) {
	root := t.TempDir()
	code, out, diagnostics := runTestCommand(root, "gaps", "--json")
	if code != 2 || out != "" || !strings.Contains(diagnostics, "E_NO_PROJECT") {
		t.Fatalf("missing project: code=%d stdout=%q stderr=%q", code, out, diagnostics)
	}
	code, out, diagnostics = runTestCommand(root, "unknown")
	if code != 2 || out != "" || !strings.Contains(diagnostics, "E_USAGE") {
		t.Fatalf("unknown command: code=%d stdout=%q stderr=%q", code, out, diagnostics)
	}
}

func TestRunRelativeProjectOverrideUsesInvocationDirectory(t *testing.T) {
	root := t.TempDir()
	other := t.TempDir()
	if code, _, diagnostics := runTestCommand(root, "init", "goal"); code != 0 {
		t.Fatal(diagnostics)
	}
	relative, err := filepath.Rel(other, root)
	if err != nil {
		t.Fatal(err)
	}
	code, out, diagnostics := runTestCommand(other, "--project", relative, "show", "--json")
	if code != 0 || !json.Valid([]byte(out)) || diagnostics != "" {
		t.Fatalf("relative override: exit=%d stdout=%q stderr=%q", code, out, diagnostics)
	}
}

func TestRunOutputIndependentOfStoredCellOrder(t *testing.T) {
	root := t.TempDir()
	for _, args := range [][]string{{"init", "goal"}, {"add", "z"}, {"add", "a"}} {
		if code, _, diagnostics := runTestCommand(root, args...); code != 0 {
			t.Fatalf("%v: %s", args, diagnostics)
		}
	}
	before := make([]string, 0)
	for _, args := range [][]string{{"gaps", "--json"}, {"show", "--json"}, {"gaps"}, {"status"}, {"show"}} {
		_, out, _ := runTestCommand(root, args...)
		before = append(before, out)
	}
	s, err := Load(root)
	if err != nil {
		t.Fatal(err)
	}
	s.Cells[0], s.Cells[1] = s.Cells[1], s.Cells[0]
	data, err := json.Marshal(s)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, ".mandala", "state.json"), data, 0600); err != nil {
		t.Fatal(err)
	}
	for i, args := range [][]string{{"gaps", "--json"}, {"show", "--json"}, {"gaps"}, {"status"}, {"show"}} {
		_, out, diagnostics := runTestCommand(root, args...)
		if out != before[i] || diagnostics != "" {
			t.Fatalf("%v changed after stored order changed: %q != %q; stderr=%q", args, out, before[i], diagnostics)
		}
	}
}

func TestRunRejectsDuplicateAndNinthChild(t *testing.T) {
	root := t.TempDir()
	for _, args := range [][]string{{"init", "goal"}, {"add", "parent"}} {
		if code, _, diagnostics := runTestCommand(root, args...); code != 0 {
			t.Fatalf("%v: %s", args, diagnostics)
		}
	}
	code, out, diagnostics := runTestCommand(root, "add", "parent")
	if code != 2 || out != "" || !strings.Contains(diagnostics, "E_DUPLICATE_ID") {
		t.Fatalf("duplicate: exit=%d stdout=%q stderr=%q", code, out, diagnostics)
	}
	for _, child := range []string{"a", "b", "c", "d", "e", "f", "g", "h"} {
		if code, _, diagnostics := runTestCommand(root, "add", "parent."+child); code != 0 {
			t.Fatal(diagnostics)
		}
	}
	code, out, diagnostics = runTestCommand(root, "add", "parent.i")
	if code != 2 || out != "" || !strings.Contains(diagnostics, "E_LIMIT") {
		t.Fatalf("ninth child: exit=%d stdout=%q stderr=%q", code, out, diagnostics)
	}
}

func TestRunOptionalOnlyGapDoesNotBlockCompletion(t *testing.T) {
	root := t.TempDir()
	for _, args := range [][]string{{"init", "goal"}, {"add", "--optional", "nice-to-have"}} {
		if code, out, diagnostics := runTestCommand(root, args...); code != 0 || out != "" || diagnostics != "" {
			t.Fatalf("%v: exit=%d stdout=%q stderr=%q", args, code, out, diagnostics)
		}
	}
	code, out, diagnostics := runTestCommand(root, "gaps", "--json")
	if code != 0 || out != "{\"schema_version\":1,\"gaps\":[{\"id\":\"nice-to-have\",\"required\":false}]}\n" || diagnostics != "" {
		t.Fatalf("optional gaps: exit=%d stdout=%q stderr=%q", code, out, diagnostics)
	}
	code, out, diagnostics = runTestCommand(root, "gaps", "--required", "--json")
	if code != 0 || out != "{\"schema_version\":1,\"gaps\":[]}\n" || diagnostics != "" {
		t.Fatalf("required gaps: exit=%d stdout=%q stderr=%q", code, out, diagnostics)
	}
	code, out, diagnostics = runTestCommand(root, "status")
	if code != 0 || !strings.Contains(out, "required gaps: 0\n") || diagnostics != "" {
		t.Fatalf("status: exit=%d stdout=%q stderr=%q", code, out, diagnostics)
	}
}

func TestRunShowGroupsChildWithItsParent(t *testing.T) {
	root := t.TempDir()
	for _, args := range [][]string{{"init", "goal"}, {"add", "a"}, {"add", "a-x"}, {"add", "a.z"}} {
		if code, _, diagnostics := runTestCommand(root, args...); code != 0 {
			t.Fatalf("%v: %s", args, diagnostics)
		}
	}
	code, out, diagnostics := runTestCommand(root, "show")
	want := "goal: goal\na [expanded, required]\n  a.z [open, required]\na-x [open, required]\n"
	if code != 0 || out != want || diagnostics != "" {
		t.Fatalf("show: exit=%d stdout=%q, want=%q stderr=%q", code, out, want, diagnostics)
	}
	code, out, diagnostics = runTestCommand(root, "show", "--json")
	var shown State
	if err := json.Unmarshal([]byte(out), &shown); err != nil {
		t.Fatal(err)
	}
	if code != 0 || diagnostics != "" || len(shown.Cells) != 3 || shown.Cells[0].ID != "a" || shown.Cells[1].ID != "a-x" || shown.Cells[2].ID != "a.z" {
		t.Fatalf("JSON order changed: exit=%d stdout=%q stderr=%q", code, out, diagnostics)
	}
}

func TestRunInitRejectsInvalidGoalAsUsage(t *testing.T) {
	for _, goal := range []string{"", "a\nb", "a\u2028b", strings.Repeat("a", 513)} {
		root := t.TempDir()
		code, out, diagnostics := runTestCommand(root, "init", goal)
		if code != 2 || out != "" || !strings.Contains(diagnostics, "E_USAGE") {
			t.Fatalf("init %q: exit=%d stdout=%q stderr=%q", goal, code, out, diagnostics)
		}
		if _, err := os.Stat(filepath.Join(root, ".mandala")); !os.IsNotExist(err) {
			t.Fatalf("invalid init created project directory: %v", err)
		}
	}
}
