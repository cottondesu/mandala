package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestCLIStatusJSONWorkflow(t *testing.T) {
	binary := buildCLI(t)
	root := t.TempDir()
	for _, args := range [][]string{
		{"init", "Implement OAuth"}, {"add", "security"}, {"add", "tests"}, {"add", "compatibility"},
		{"add", "security.csrf"}, {"done", "security.csrf"},
		{"add", "--optional", "docs"}, {"add", "--optional", "performance"}, {"done", "performance"},
	} {
		code, out, diagnostics := invokeCLI(t, binary, root, args...)
		if code != 0 || out != "" || diagnostics != "" {
			t.Fatalf("%v: exit=%d stdout=%q stderr=%q", args, code, out, diagnostics)
		}
	}
	path := filepath.Join(root, ".mandala", "state.json")
	before, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	beforeInfo, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	want := `{"schema_version":1,"goal":"Implement OAuth","cells":6,"groups":1,"required":{"open":2,"done":1,"na":0},"optional":{"open":1,"done":1,"na":0},"required_gaps":2}` + "\n"
	for range 3 {
		code, out, diagnostics := invokeCLI(t, binary, root, "status", "--json")
		if code != 1 || out != want || diagnostics != "" {
			t.Fatalf("open gaps: exit=%d stdout=%q stderr=%q", code, out, diagnostics)
		}
	}
	after, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	afterInfo, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(after) != string(before) || afterInfo.Size() != beforeInfo.Size() || !afterInfo.ModTime().Equal(beforeInfo.ModTime()) || afterInfo.Mode() != beforeInfo.Mode() {
		t.Fatal("binary status modified state")
	}
	for _, args := range [][]string{{"status"}, {"gaps", "--json"}, {"show", "--json"}} {
		code, out, diagnostics := invokeCLI(t, binary, root, args...)
		expectedCode := 1
		if args[0] == "show" {
			expectedCode = 0
		}
		if code != expectedCode || out == "" || diagnostics != "" || (len(args) == 2 && !json.Valid([]byte(out))) {
			t.Fatalf("%v: exit=%d stdout=%q stderr=%q", args, code, out, diagnostics)
		}
	}
	for _, id := range []string{"tests", "compatibility"} {
		if code, out, diagnostics := invokeCLI(t, binary, root, "done", id); code != 0 || out != "" || diagnostics != "" {
			t.Fatalf("done %s: exit=%d stdout=%q stderr=%q", id, code, out, diagnostics)
		}
	}
	want = `{"schema_version":1,"goal":"Implement OAuth","cells":6,"groups":1,"required":{"open":0,"done":3,"na":0},"optional":{"open":1,"done":1,"na":0},"required_gaps":0}` + "\n"
	code, out, diagnostics := invokeCLI(t, binary, root, "status", "--json")
	if code != 0 || out != want || diagnostics != "" {
		t.Fatalf("resolved gaps: exit=%d stdout=%q stderr=%q", code, out, diagnostics)
	}
}
