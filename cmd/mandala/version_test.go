package main

import (
	"bytes"
	"debug/buildinfo"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCLIVersionWithoutProjectOrGit(t *testing.T) {
	binary := buildCLI(t)
	info, err := buildinfo.ReadFile(binary)
	if err != nil {
		t.Fatal(err)
	}
	version := info.Main.Version
	if version == "" || version == "(devel)" {
		version = "(devel)"
	}
	want := "mandala " + version + "\n"
	root := t.TempDir()
	t.Setenv("PATH", t.TempDir())
	code, out, diagnostics := invokeCLI(t, binary, root, "--version")
	if code != 0 || out != want || !strings.HasPrefix(out, "mandala ") || !strings.HasSuffix(out, "\n") || strings.Count(out, "\n") != 1 || diagnostics != "" {
		t.Fatalf("version: exit=%d stdout=%q stderr=%q", code, out, diagnostics)
	}
	entries, err := os.ReadDir(root)
	if err != nil || len(entries) != 0 {
		t.Fatalf("version modified empty directory: entries=%v err=%v", entries, err)
	}

	t.Run("broken state", func(t *testing.T) {
		dir := filepath.Join(root, ".mandala")
		if err := os.Mkdir(dir, 0700); err != nil {
			t.Fatal(err)
		}
		statePath := filepath.Join(dir, "state.json")
		broken := []byte("{broken")
		if err := os.WriteFile(statePath, broken, 0600); err != nil {
			t.Fatal(err)
		}
		code, brokenOut, diagnostics := invokeCLI(t, binary, root, "--version")
		if code != 0 || brokenOut != out || diagnostics != "" {
			t.Fatalf("broken state: exit=%d stdout=%q stderr=%q", code, brokenOut, diagnostics)
		}
		data, err := os.ReadFile(statePath)
		if err != nil || !bytes.Equal(data, broken) {
			t.Fatalf("version changed broken state: data=%q err=%v", data, err)
		}
	})
	for _, args := range [][]string{
		{"--version", "status"},
		{"--project", ".", "--version"},
		{"--version", "--project", "."},
		{"status", "--version"},
		{"version"},
		{"--version=false", "init", "unexpected"},
		{"--version", "--version=false", "init", "unexpected"},
		{"--version", "--help"},
		{"--version", "--version"},
		{"--version", "--"},
	} {
		t.Run(strings.Join(args, " "), func(t *testing.T) {
			code, out, diagnostics := invokeCLI(t, binary, root, args...)
			if code != 2 || out != "" || !strings.Contains(diagnostics, "E_USAGE") {
				t.Fatalf("%v: exit=%d stdout=%q stderr=%q", args, code, out, diagnostics)
			}
		})
	}
}
