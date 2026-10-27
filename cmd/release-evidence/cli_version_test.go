package main

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// runCLIHelper launches this test binary as the real CLI through the helper
// entry in TestCLIArguments (cli_arguments_test.go). PATH is emptied so a
// request that passes argument validation stops at "launching git" instead of
// running anything.
func runCLIHelper(t *testing.T, cwd string, args []string) (exit int, stdout, stderr string) {
	t.Helper()
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command(executable, append([]string{"-test.run=^TestCLIArguments$", "--", "release-evidence-cli-helper"}, args...)...)
	cmd.Dir = cwd
	cmd.Env = append(os.Environ(), "PATH="+t.TempDir())
	var out, errb bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &errb
	err = cmd.Run()
	if cmd.ProcessState == nil {
		t.Fatalf("launch CLI subprocess: %v", err)
	}
	return cmd.ProcessState.ExitCode(), out.String(), errb.String()
}

func TestCLIExpectedGoMinor(t *testing.T) {
	cases := []struct {
		name     string
		flagArgs []string // nil: flag omitted
		wantExit int
		wantText string
	}{
		{name: "omitted keeps BLOCKED path", wantExit: 1, wantText: "launching git to find the repo root"},
		{name: "minor", flagArgs: []string{"-expected-go-minor", "1.26"}, wantExit: 1, wantText: "launching git"},
		{name: "go prefix", flagArgs: []string{"-expected-go-minor", "go1.27"}, wantExit: 1, wantText: "launching git"},
		{name: "patch", flagArgs: []string{"-expected-go-minor=1.27.1"}, wantExit: 1, wantText: "launching git"},
		{name: "future minor", flagArgs: []string{"-expected-go-minor", "1.99"}, wantExit: 1, wantText: "launching git"},
		{name: "surrounding whitespace", flagArgs: []string{"-expected-go-minor", " go1.27.3\t"}, wantExit: 1, wantText: "launching git"},
		{name: "non-numeric patch", flagArgs: []string{"-expected-go-minor", "1.27.invalid"}, wantExit: 2, wantText: `"1.27.invalid"`},
		{name: "extra component", flagArgs: []string{"-expected-go-minor", "1.27.3.extra"}, wantExit: 2, wantText: `"1.27.3.extra"`},
		{name: "trailing dot", flagArgs: []string{"-expected-go-minor", "1.27."}, wantExit: 2, wantText: `"1.27."`},
		{name: "go only", flagArgs: []string{"-expected-go-minor", "go"}, wantExit: 2, wantText: `"go"`},
		{name: "major only", flagArgs: []string{"-expected-go-minor", "1"}, wantExit: 2, wantText: `"1"`},
		{name: "prerelease suffix", flagArgs: []string{"-expected-go-minor", "1.27-beta"}, wantExit: 2, wantText: `"1.27-beta"`},
		{name: "letter suffix", flagArgs: []string{"-expected-go-minor", "1.27abc"}, wantExit: 2, wantText: `"1.27abc"`},
		{name: "empty component", flagArgs: []string{"-expected-go-minor", "1..27"}, wantExit: 2, wantText: `"1..27"`},
		{name: "explicit empty", flagArgs: []string{"-expected-go-minor", ""}, wantExit: 2, wantText: `""`},
		{name: "explicit empty equals form", flagArgs: []string{"-expected-go-minor="}, wantExit: 2, wantText: `""`},
		{name: "explicit whitespace", flagArgs: []string{"-expected-go-minor", "  "}, wantExit: 2, wantText: `"  "`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cwd := t.TempDir()
			output := filepath.Join(cwd, "evidence")
			args := []string{"-output", output, "-candidate", strings.Repeat("a", 40), "-allow-dirty", "-go", filepath.Join(cwd, "missing-go")}
			args = append(args, tc.flagArgs...)
			exit, stdout, stderr := runCLIHelper(t, cwd, args)
			t.Logf("exit=%d stdout=%q stderr=%q", exit, stdout, stderr)
			if exit != tc.wantExit {
				t.Errorf("exit = %d, want %d", exit, tc.wantExit)
			}
			if !strings.Contains(stderr, tc.wantText) {
				t.Errorf("stderr = %q, want %q", stderr, tc.wantText)
			}
			if tc.wantExit == 2 {
				if !strings.Contains(stderr, "-expected-go-minor") {
					t.Errorf("stderr = %q, want flag name in diagnostic", stderr)
				}
				if strings.Contains(stderr, "launching git") {
					t.Errorf("stderr = %q: git was launched before validation", stderr)
				}
			}
			if stdout != "" {
				t.Errorf("stdout = %q, want empty", stdout)
			}
			if _, err := os.Stat(output); !os.IsNotExist(err) {
				t.Errorf("output directory was created or stat failed: %v", err)
			}
		})
	}
}
