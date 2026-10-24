package main

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// invokeCLIStdin is invokeCLI with stdin content, for `batch --file -`.
func invokeCLIStdin(t *testing.T, binary, dir, stdin string, args ...string) (int, string, string) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, binary, args...)
	cmd.Dir = dir
	cmd.Stdin = strings.NewReader(stdin)
	cmd.Env = append(os.Environ(), "TERM=dumb")
	var out, diagnostics bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &diagnostics
	err := cmd.Run()
	if ctx.Err() != nil {
		t.Fatalf("CLI blocked: %v", ctx.Err())
	}
	if err == nil {
		return 0, out.String(), diagnostics.String()
	}
	if exit, ok := err.(*exec.ExitError); ok {
		return exit.ExitCode(), out.String(), diagnostics.String()
	}
	t.Fatalf("run CLI: %v", err)
	return 0, "", ""
}

func writeIntegrationBatchFile(t *testing.T, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "batch.json")
	if err := os.WriteFile(path, []byte(content), 0600); err != nil {
		t.Fatal(err)
	}
	return path
}

const integrationBatchInput = `{"batch_version":1,"operations":[` +
	`{"op":"add","id":"security"},` +
	`{"op":"add","id":"security.csrf"},` +
	`{"op":"mark","id":"security.csrf","status":"done"},` +
	`{"op":"add","id":"tests"}]}`

const integrationBatchJSONResult = `{"batch_result_version":1,"dry_run":false,"changed":true,"operations":4,` +
	`"required_gaps_before":0,"required_gaps_after":1,"changes":[` +
	`{"id":"security","before":null,"after":{"parent":"","status":"expanded","required":true}},` +
	`{"id":"security.csrf","before":null,"after":{"parent":"security","status":"done","required":true}},` +
	`{"id":"tests","before":null,"after":{"parent":"","status":"open","required":true}}]}` + "\n"

func TestCLIBatchApply(t *testing.T) {
	binary := buildCLI(t)
	root := t.TempDir()
	if code, _, diagnostics := invokeCLI(t, binary, root, "init", "goal"); code != 0 {
		t.Fatal(diagnostics)
	}
	ops := writeIntegrationBatchFile(t, integrationBatchInput)

	// Absolute input file path outside the project directory.
	code, out, diagnostics := invokeCLI(t, binary, root, "batch", "--file", ops)
	if code != 0 || out != "" || diagnostics != "" {
		t.Fatalf("apply: exit=%d stdout=%q stderr=%q", code, out, diagnostics)
	}
	statePath := filepath.Join(root, ".mandala", "state.json")
	state, err := os.ReadFile(statePath)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{`"id": "security"`, `"status": "expanded"`, `"id": "security.csrf"`, `"status": "done"`, `"id": "tests"`} {
		if !strings.Contains(string(state), want) {
			t.Fatalf("state missing %s:\n%s", want, state)
		}
	}

	// Semantic no-op via canceling marks: silent, no exit 1.
	noop := writeIntegrationBatchFile(t, `{"batch_version":1,"operations":[`+
		`{"op":"mark","id":"tests","status":"done"},{"op":"mark","id":"tests","status":"open"}]}`)
	code, out, diagnostics = invokeCLI(t, binary, root, "batch", "--file", noop)
	if code != 0 || out != "" || diagnostics != "" {
		t.Fatalf("no-op apply: exit=%d stdout=%q stderr=%q", code, out, diagnostics)
	}
}

func TestCLIBatchFreshProjectJSONResult(t *testing.T) {
	binary := buildCLI(t)
	root := t.TempDir()
	if code, _, diagnostics := invokeCLI(t, binary, root, "init", "goal"); code != 0 {
		t.Fatal(diagnostics)
	}
	ops := writeIntegrationBatchFile(t, integrationBatchInput)
	code, out, diagnostics := invokeCLI(t, binary, root, "batch", "--file", ops, "--json")
	if code != 0 || diagnostics != "" || out != integrationBatchJSONResult {
		t.Fatalf("apply JSON: exit=%d stdout=%q stderr=%q", code, out, diagnostics)
	}
}

func TestCLIBatchStdin(t *testing.T) {
	binary := buildCLI(t)
	root := t.TempDir()
	if code, _, diagnostics := invokeCLI(t, binary, root, "init", "goal"); code != 0 {
		t.Fatal(diagnostics)
	}
	code, out, diagnostics := invokeCLIStdin(t, binary, root, integrationBatchInput, "batch", "--file", "-", "--dry-run")
	if code != 0 || diagnostics != "" {
		t.Fatalf("stdin dry-run: exit=%d stderr=%q", code, diagnostics)
	}
	want := "batch dry-run: 4 operation(s), 3 changed cell(s)\n" +
		"required gaps: 0 -> 1\n" +
		"+ security [expanded, required]\n" +
		"+ security.csrf [done, required]\n" +
		"+ tests [open, required]\n"
	if out != want {
		t.Fatalf("stdin preview:\n got=%q\nwant=%q", out, want)
	}
}

func TestCLIBatchProjectOverrideAndRelativeFile(t *testing.T) {
	binary := buildCLI(t)
	root := t.TempDir()
	if code, _, diagnostics := invokeCLI(t, binary, root, "init", "goal"); code != 0 {
		t.Fatal(diagnostics)
	}
	workdir := t.TempDir()
	if err := os.WriteFile(filepath.Join(workdir, "ops.json"), []byte(integrationBatchInput), 0600); err != nil {
		t.Fatal(err)
	}
	// The state resolves via --project; the input file resolves via cwd.
	code, out, diagnostics := invokeCLI(t, binary, workdir, "--project", root, "batch", "--file", "ops.json")
	if code != 0 || out != "" || diagnostics != "" {
		t.Fatalf("override: exit=%d stdout=%q stderr=%q", code, out, diagnostics)
	}
	if _, err := os.Stat(filepath.Join(workdir, ".mandala")); !os.IsNotExist(err) {
		t.Fatalf("project must not be created in cwd: %v", err)
	}
	state, err := os.ReadFile(filepath.Join(root, ".mandala", "state.json"))
	if err != nil || !strings.Contains(string(state), `"id": "security"`) {
		t.Fatalf("override state: %v", err)
	}

	// Absolute input path from an unrelated directory, against a fresh
	// project, without --project.
	absolute := filepath.Join(t.TempDir(), "ops.json")
	if err := os.WriteFile(absolute, []byte(integrationBatchInput), 0600); err != nil {
		t.Fatal(err)
	}
	other := initCLIProject(t, binary)
	code, out, diagnostics = invokeCLI(t, binary, other, "batch", "--file", absolute, "--dry-run", "--json")
	if code != 0 || out == "" || diagnostics != "" {
		t.Fatalf("absolute path: exit=%d stdout=%q stderr=%q", code, out, diagnostics)
	}
}

func initCLIProject(t *testing.T, binary string) string {
	t.Helper()
	root := t.TempDir()
	if code, _, diagnostics := invokeCLI(t, binary, root, "init", "goal"); code != 0 {
		t.Fatal(diagnostics)
	}
	return root
}

func TestCLIBatchErrors(t *testing.T) {
	binary := buildCLI(t)
	root := initCLIProject(t, binary)
	bad := writeIntegrationBatchFile(t, `{"batch_version":1,"operations":[{"op":"add","id":"a.b"}]}`)
	code, out, diagnostics := invokeCLI(t, binary, root, "batch", "--file", bad, "--dry-run")
	if code != 2 || out != "" || !strings.Contains(diagnostics, "E_INVALID_PARENT") || !strings.Contains(diagnostics, "batch operation 1 (add a.b)") {
		t.Fatalf("operation failure: exit=%d stdout=%q stderr=%q", code, out, diagnostics)
	}
	invalid := writeIntegrationBatchFile(t, `{"batch_version":1,"operations":[]}`)
	code, out, diagnostics = invokeCLI(t, binary, root, "batch", "--file", invalid)
	if code != 2 || out != "" || !strings.Contains(diagnostics, "E_LIMIT") || !strings.Contains(diagnostics, "at least 1 operation") {
		t.Fatalf("input failure: exit=%d stdout=%q stderr=%q", code, out, diagnostics)
	}
	code, out, diagnostics = invokeCLI(t, binary, root, "batch")
	if code != 2 || out != "" || !strings.Contains(diagnostics, "E_USAGE") {
		t.Fatalf("missing file flag: exit=%d stdout=%q stderr=%q", code, out, diagnostics)
	}
}

func TestCLIBatchHelpAndVersion(t *testing.T) {
	binary := buildCLI(t)
	dir := t.TempDir()
	code, out, diagnostics := invokeCLI(t, binary, dir, "batch", "--help")
	want := "Usage: mandala [--project DIR] batch --file <file|-> [--dry-run] [--json]\n"
	if code != 0 || out != want || diagnostics != "" {
		t.Fatalf("batch help: exit=%d stdout=%q stderr=%q", code, out, diagnostics)
	}
	code, out, diagnostics = invokeCLI(t, binary, dir, "--help")
	if code != 0 || !strings.Contains(out, "batch") || diagnostics != "" {
		t.Fatalf("root help lists batch: exit=%d stdout=%q stderr=%q", code, out, diagnostics)
	}
}

// TestCLIBatchExistingCommandsUnchanged guards the v0.4.1 command contracts.
func TestCLIBatchExistingCommandsUnchanged(t *testing.T) {
	binary := buildCLI(t)
	root := initCLIProject(t, binary)
	for _, args := range [][]string{
		{"add", "security"},
		{"add", "security.csrf"},
		{"add", "tests"},
		{"done", "security.csrf"},
		{"status"},
	} {
		code, out, diagnostics := invokeCLI(t, binary, root, args...)
		if code != 0 && args[0] != "status" {
			t.Fatalf("%v: exit=%d stderr=%q", args, code, diagnostics)
		}
		if args[0] == "status" && code != 1 {
			t.Fatalf("status exit changed: %d", code)
		}
		if diagnostics != "" || (out == "" && args[0] == "status") {
			t.Fatalf("%v: stdout=%q stderr=%q", args, out, diagnostics)
		}
	}
}

func TestCLIBatchREADMEExample(t *testing.T) {
	doc, err := os.ReadFile(filepath.Join("..", "..", "README.md"))
	if err != nil {
		t.Fatal(err)
	}
	_, section, ok := strings.Cut(string(doc), "### Batch output")
	if !ok {
		t.Fatal("README batch output section is missing")
	}
	_, textBlock, ok := strings.Cut(section, "```text\n")
	if !ok {
		t.Fatal("README text example is missing")
	}
	wantText, _, ok := strings.Cut(textBlock, "```")
	if !ok {
		t.Fatal("README text fence is not closed")
	}
	_, jsonBlock, ok := strings.Cut(section, "```json\n")
	if !ok {
		t.Fatal("README JSON example is missing")
	}
	wantJSON, _, ok := strings.Cut(jsonBlock, "```")
	if !ok {
		t.Fatal("README JSON fence is not closed")
	}
	var compact bytes.Buffer
	if err := json.Compact(&compact, []byte(wantJSON)); err != nil {
		t.Fatal(err)
	}
	binary := buildCLI(t)
	root := initCLIProject(t, binary)
	if code, _, diagnostics := invokeCLI(t, binary, root, "add", "security"); code != 0 {
		t.Fatal(diagnostics)
	}
	input := `{"batch_version":1,"operations":[{"op":"add","id":"security.csrf"},{"op":"add","id":"tests"}]}`
	for _, asJSON := range []bool{false, true} {
		args := []string{"batch", "--file", "-", "--dry-run"}
		want := wantText
		if asJSON {
			args = append(args, "--json")
			want = compact.String() + "\n"
		}
		code, out, diagnostics := invokeCLIStdin(t, binary, root, input, args...)
		if code != 0 || diagnostics != "" || out != want {
			t.Fatalf("README example json=%v: exit=%d stderr=%q\ngot=%q\nwant=%q", asJSON, code, diagnostics, out, want)
		}
	}
}
