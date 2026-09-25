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

func buildCLI(t *testing.T) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "mandala")
	cmd := exec.Command("go", "build", "-o", path, ".")
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("build CLI: %v: %s", err, output)
	}
	return path
}

func invokeCLI(t *testing.T, binary, dir string, args ...string) (int, string, string) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, binary, args...)
	cmd.Dir = dir
	cmd.Stdin = bytes.NewReader(nil)
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

func TestCLIWorkflowAndCleanup(t *testing.T) {
	binary := buildCLI(t)
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
		code, out, diagnostics := invokeCLI(t, binary, root, args...)
		if code != 0 || out != "" || diagnostics != "" {
			t.Fatalf("%v: exit=%d stdout=%q stderr=%q", args, code, out, diagnostics)
		}
	}
	for _, args := range [][]string{{"gaps"}, {"gaps", "--json"}, {"status"}, {"show"}} {
		code, out, diagnostics := invokeCLI(t, binary, root, args...)
		if code != 1 && (args[0] == "gaps" || args[0] == "status") {
			t.Fatalf("%v: exit=%d", args, code)
		}
		if args[0] == "show" && code != 0 {
			t.Fatalf("show: exit=%d", code)
		}
		if out == "" || diagnostics != "" {
			t.Fatalf("%v: stdout=%q stderr=%q", args, out, diagnostics)
		}
		if len(args) == 2 && !json.Valid([]byte(out)) {
			t.Fatalf("invalid JSON: %q", out)
		}
	}
	sub := filepath.Join(root, "internal", "auth")
	if err := os.MkdirAll(sub, 0700); err != nil {
		t.Fatal(err)
	}
	_, rootGaps, _ := invokeCLI(t, binary, root, "gaps", "--json")
	code, subGaps, diagnostics := invokeCLI(t, binary, sub, "gaps", "--json")
	if code != 1 || subGaps != rootGaps || diagnostics != "" {
		t.Fatalf("subdirectory discovery: exit=%d stdout=%q stderr=%q", code, subGaps, diagnostics)
	}
	for _, id := range []string{"security.token-storage", "tests", "compatibility"} {
		if code, _, diagnostics := invokeCLI(t, binary, root, "done", id); code != 0 {
			t.Fatalf("done %s: %s", id, diagnostics)
		}
	}
	code, out, _ := invokeCLI(t, binary, root, "gaps", "--required", "--json")
	if code != 0 || out != "{\"schema_version\":1,\"gaps\":[]}\n" {
		t.Fatalf("complete gaps: exit=%d stdout=%q", code, out)
	}
	statePath := filepath.Join(root, ".mandala", "state.json")
	if _, err := os.Stat(statePath); err != nil {
		t.Fatalf("completed state disappeared: %v", err)
	}
	for _, command := range []string{"status", "show"} {
		code, out, diagnostics := invokeCLI(t, binary, root, command)
		if code != 0 || out == "" || diagnostics != "" {
			t.Fatalf("%s after completion: exit=%d stdout=%q stderr=%q", command, code, out, diagnostics)
		}
	}
	note := filepath.Join(root, ".mandala", "user-note.txt")
	if err := os.WriteFile(note, []byte("keep"), 0600); err != nil {
		t.Fatal(err)
	}
	for range 2 {
		if code, out, diagnostics := invokeCLI(t, binary, root, "clean"); code != 0 || out != "" || diagnostics != "" {
			t.Fatalf("clean: exit=%d stdout=%q stderr=%q", code, out, diagnostics)
		}
	}
	if _, err := os.Stat(note); err != nil {
		t.Fatalf("clean removed user file: %v", err)
	}
	if _, err := os.Stat(statePath); !os.IsNotExist(err) {
		t.Fatalf("clean did not remove state: %v", err)
	}
}

func TestCLIProjectOverrideAndErrors(t *testing.T) {
	binary := buildCLI(t)
	root := t.TempDir()
	other := t.TempDir()
	if code, _, diagnostics := invokeCLI(t, binary, root, "init", "goal"); code != 0 {
		t.Fatal(diagnostics)
	}
	code, out, diagnostics := invokeCLI(t, binary, other, "--project", root, "show", "--json")
	if code != 0 || diagnostics != "" || !json.Valid([]byte(out)) {
		t.Fatalf("override: exit=%d stdout=%q stderr=%q", code, out, diagnostics)
	}
	code, out, diagnostics = invokeCLI(t, binary, other, "gaps", "--json")
	if code != 2 || out != "" || !strings.Contains(diagnostics, "E_NO_PROJECT") {
		t.Fatalf("missing project: exit=%d stdout=%q stderr=%q", code, out, diagnostics)
	}
	code, out, diagnostics = invokeCLI(t, binary, root, "add", "a", "--optional")
	if code != 2 || out != "" || !strings.Contains(diagnostics, "E_USAGE") {
		t.Fatalf("flag after argument: exit=%d stdout=%q stderr=%q", code, out, diagnostics)
	}
	code, out, diagnostics = invokeCLI(t, binary, root, "done", "unknown")
	if code != 2 || out != "" || !strings.Contains(diagnostics, "E_UNKNOWN_NODE") {
		t.Fatalf("unknown cell: exit=%d stdout=%q stderr=%q", code, out, diagnostics)
	}
	if err := os.WriteFile(filepath.Join(root, ".mandala", "state.json"), []byte("{broken"), 0600); err != nil {
		t.Fatal(err)
	}
	code, out, diagnostics = invokeCLI(t, binary, root, "gaps", "--json")
	if code != 2 || out != "" || !strings.Contains(diagnostics, "E_STATE_INVALID") {
		t.Fatalf("corrupt JSON: exit=%d stdout=%q stderr=%q", code, out, diagnostics)
	}
}
