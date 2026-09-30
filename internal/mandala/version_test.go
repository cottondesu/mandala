package mandala

import (
	"bytes"
	"os"
	"path/filepath"
	"runtime/debug"
	"strings"
	"testing"
)

func TestModuleVersionFromBuildInfo(t *testing.T) {
	for _, tt := range []struct {
		name string
		info *debug.BuildInfo
		ok   bool
		want string
	}{
		{"unavailable", nil, false, "(devel)"},
		{"empty", &debug.BuildInfo{}, true, "(devel)"},
		{"development", &debug.BuildInfo{Main: debug.Module{Version: "(devel)"}}, true, "(devel)"},
		{"release", &debug.BuildInfo{Main: debug.Module{Version: "v0.3.0"}}, true, "v0.3.0"},
		{"dirty", &debug.BuildInfo{Main: debug.Module{Version: "v0.2.0+dirty"}}, true, "v0.2.0+dirty"},
		{"pseudo-version", &debug.BuildInfo{Main: debug.Module{Version: "v0.3.1-0.20260930010101-abcdef123456"}}, true, "v0.3.1-0.20260930010101-abcdef123456"},
		{"main module", &debug.BuildInfo{
			GoVersion: "go1.26",
			Main:      debug.Module{Version: "v0.3.0", Replace: &debug.Module{Version: "v9.0.0"}},
			Deps:      []*debug.Module{{Version: "v8.0.0"}},
		}, true, "v0.3.0"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := moduleVersionFromBuildInfo(tt.info, tt.ok); got != tt.want {
				t.Fatalf("build info: got %q, want %q", got, tt.want)
			}
		})
	}
}

func TestNormalizeModuleVersion(t *testing.T) {
	for _, tt := range []struct {
		name, raw, want string
	}{
		{"release", "v0.3.0", "v0.3.0"},
		{"dirty", "v0.2.0+dirty", "v0.2.0+dirty"},
		{"development", "(devel)", "(devel)"},
		{"empty", "", "(devel)"},
		{"pseudo-version", "v0.3.1-0.20260930010101-abcdef123456", "v0.3.1-0.20260930010101-abcdef123456"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := normalizeModuleVersion(tt.raw); got != tt.want {
				t.Fatalf("normalize %q: got %q, want %q", tt.raw, got, tt.want)
			}
		})
	}
}

func TestRunVersionWithControlledModuleVersion(t *testing.T) {
	for _, tt := range []struct {
		name, raw, want string
	}{
		{"release", "v0.3.0", "mandala v0.3.0\n"},
		{"dirty", "v0.2.0+dirty", "mandala v0.2.0+dirty\n"},
		{"development", "(devel)", "mandala (devel)\n"},
		{"empty", "", "mandala (devel)\n"},
		{"pseudo-version", "v0.3.1-0.20260930010101-abcdef123456", "mandala v0.3.1-0.20260930010101-abcdef123456\n"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			root := t.TempDir()
			var out, diagnostics bytes.Buffer
			code := run([]string{"--version"}, root, &out, &diagnostics, tt.raw)
			if code != 0 || out.String() != tt.want || diagnostics.Len() != 0 {
				t.Fatalf("version: exit=%d stdout=%q stderr=%q", code, out.String(), diagnostics.String())
			}
			entries, err := os.ReadDir(root)
			if err != nil || len(entries) != 0 {
				t.Fatalf("version modified empty directory: entries=%v err=%v", entries, err)
			}
		})
	}
}

func TestRunVersionRejectsCombinations(t *testing.T) {
	for _, args := range [][]string{
		{"--version", "status"},
		{"--project", ".", "--version"},
		{"--version", "--project", "."},
		{"--project=", "--version"},
		{"--version", "--project="},
		{"--version=false", "init", "unexpected"},
		{"--version", "--version=false", "init", "unexpected"},
		{"--version", "--help"},
		{"--version", "--version"},
		{"--version", "--"},
	} {
		t.Run(strings.Join(args, " "), func(t *testing.T) {
			t.Parallel()
			root := t.TempDir()
			code, out, diagnostics := runTestCommand(root, args...)
			if code != 2 || out != "" || !strings.Contains(diagnostics, "E_USAGE") {
				t.Fatalf("%v: exit=%d stdout=%q stderr=%q", args, code, out, diagnostics)
			}
			entries, err := os.ReadDir(root)
			if err != nil || len(entries) != 0 {
				t.Fatalf("invalid version invocation modified directory: entries=%v err=%v", entries, err)
			}
		})
	}
}

func TestRunVersionIgnoresBrokenState(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	dir := filepath.Join(root, ".mandala")
	if err := os.Mkdir(dir, 0700); err != nil {
		t.Fatal(err)
	}
	statePath := filepath.Join(dir, "state.json")
	broken := []byte("{broken")
	if err := os.WriteFile(statePath, broken, 0600); err != nil {
		t.Fatal(err)
	}
	var out, diagnostics bytes.Buffer
	code := run([]string{"--version"}, root, &out, &diagnostics, "v0.3.0")
	if code != 0 || out.String() != "mandala v0.3.0\n" || diagnostics.Len() != 0 {
		t.Fatalf("broken state: exit=%d stdout=%q stderr=%q", code, out.String(), diagnostics.String())
	}
	data, err := os.ReadFile(statePath)
	if err != nil || !bytes.Equal(data, broken) {
		t.Fatalf("version changed broken state: data=%q err=%v", data, err)
	}
}

func TestRunRootHelpIncludesVersion(t *testing.T) {
	t.Parallel()
	code, out, diagnostics := runTestCommand(t.TempDir(), "--help")
	if code != 0 || !strings.Contains(out, "mandala --version\n") || diagnostics != "" {
		t.Fatalf("root help: exit=%d stdout=%q stderr=%q", code, out, diagnostics)
	}
}

func TestRunVersionDoesNotChangeCommandSyntax(t *testing.T) {
	argsList := [][]string{nil, {"unknown"}, {"version"}, {"--project", "", "show"}}
	for _, command := range []string{"init", "add", "mark", "done", "status", "gaps", "show", "clean"} {
		argsList = append(argsList, []string{command, "--version"})
	}
	for _, args := range argsList {
		t.Run(strings.Join(args, " "), func(t *testing.T) {
			t.Parallel()
			code, out, diagnostics := runTestCommand(t.TempDir(), args...)
			if code != 2 || out != "" || !strings.Contains(diagnostics, "E_USAGE") {
				t.Fatalf("%v: exit=%d stdout=%q stderr=%q", args, code, out, diagnostics)
			}
		})
	}
}
