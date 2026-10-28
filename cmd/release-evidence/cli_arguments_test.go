package main

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestCLIArguments(t *testing.T) {
	if len(os.Args) >= 4 && os.Args[2] == "--" && os.Args[3] == "release-evidence-cli-helper" {
		os.Args = append([]string{os.Args[0]}, os.Args[4:]...)
		main()
		return
	}

	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", t.TempDir())
	cases := []struct {
		name     string
		before   []string
		after    []string
		help     bool
		wantExit int
		wantText string
	}{
		{name: "trailing positional", after: []string{"unexpected"}, wantExit: 2, wantText: "unexpected positional arguments"},
		{name: "empty positional", after: []string{""}, wantExit: 2, wantText: "unexpected positional arguments"},
		{name: "after double dash", after: []string{"--", "unexpected"}, wantExit: 2, wantText: "unexpected positional arguments"},
		{name: "positional before flags", before: []string{"unexpected"}, wantExit: 2, wantText: "unexpected positional arguments"},
		{name: "help", help: true, wantExit: 0, wantText: "Usage:"},
		{name: "valid flags", wantExit: 1, wantText: "launching git to find the repo root"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cwd := t.TempDir()
			output := filepath.Join(cwd, "evidence")
			args := append([]string{}, tc.before...)
			args = append(args, "-output", output, "-candidate", strings.Repeat("a", 40),
				"-expected-go-minor", "1.27", "-allow-dirty", "-go", filepath.Join(cwd, "missing-go"))
			args = append(args, tc.after...)
			if tc.help {
				args = []string{"--help"}
			}
			cmd := exec.Command(executable, append([]string{"-test.run=^TestCLIArguments$", "--", "release-evidence-cli-helper"}, args...)...)
			cmd.Dir = cwd
			var stdout, stderr bytes.Buffer
			cmd.Stdout, cmd.Stderr = &stdout, &stderr
			err := cmd.Run()
			if cmd.ProcessState == nil {
				t.Fatalf("launch CLI subprocess: %v", err)
			}
			gotExit := cmd.ProcessState.ExitCode()
			t.Logf("exit=%d stdout=%q stderr=%q", gotExit, stdout.String(), stderr.String())
			if gotExit != tc.wantExit {
				t.Errorf("exit = %d, want %d", gotExit, tc.wantExit)
			}
			if !strings.Contains(stderr.String(), tc.wantText) {
				t.Errorf("stderr = %q, want %q", stderr.String(), tc.wantText)
			}
			if tc.wantExit == 2 && !strings.Contains(stderr.String(), "Usage:") {
				t.Errorf("stderr = %q, want usage", stderr.String())
			}
			if stdout.Len() != 0 {
				t.Errorf("stdout = %q, want empty", stdout.String())
			}
			if _, err := os.Stat(output); !os.IsNotExist(err) {
				t.Errorf("output directory was created or stat failed: %v", err)
			}
		})
	}
}
