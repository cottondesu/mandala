package main

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestValidateSHA(t *testing.T) {
	cases := []struct {
		in   string
		want bool
	}{
		{strings.Repeat("a1", 20), true},
		{strings.Repeat("0f", 32), true}, // SHA-256 object id
		{strings.Repeat("a1", 19), false},
		{strings.Repeat("a1", 20) + "zz", false},
		{strings.Repeat("A1", 20), true}, // normalized to lower by the caller
		{"", false},
		{"HEAD", false},
	}
	for _, tc := range cases {
		if err := validateSHA(tc.in); (err == nil) != tc.want {
			t.Errorf("validateSHA(%q) error = %v, want ok=%v", tc.in, err, tc.want)
		}
	}
}

func TestMinorParsing(t *testing.T) {
	if got := minorOfGOVERSION("go1.27.1"); got != "1.27" {
		t.Errorf("minorOfGOVERSION(go1.27.1) = %q", got)
	}
	if got := minorOfGOVERSION("go1.26"); got != "1.26" {
		t.Errorf("minorOfGOVERSION(go1.26) = %q", got)
	}
	if got := minorOfGOVERSION("devel +abc def"); got != "" {
		t.Errorf("minorOfGOVERSION(devel) = %q, want empty", got)
	}
}

func TestParseExpectedMinor(t *testing.T) {
	accepted := map[string]string{
		"1.26": "1.26", "1.27": "1.27", "go1.26": "1.26", "go1.27": "1.27",
		"1.26.10": "1.26", "1.27.1": "1.27", "go1.27.3": "1.27",
		"  1.27  ": "1.27", "\tgo1.27.1\n": "1.27", "1.100": "1.100", "2.0": "2.0",
	}
	for in, want := range accepted {
		got, err := parseExpectedMinor(in)
		if err != nil || got != want {
			t.Errorf("parseExpectedMinor(%q) = %q, %v; want %q", in, got, err, want)
		}
	}
	for _, in := range []string{
		"", " ", "go", "1", "go1", "1.27.invalid", "1.27.3.extra", "1.27.", "1.27-beta",
		"1.27abc", "1..27", ".27", "gogo1.27", "go 1.27", "1. 27", "1.27 .3", "v1.27", "1.27rc1", "1.-27",
	} {
		if got, err := parseExpectedMinor(in); err == nil {
			t.Errorf("parseExpectedMinor(%q) = %q, want error", in, got)
		}
	}
}

func TestCreateOutputDirNeverOverwritesAndStaysOutsideSource(t *testing.T) {
	root := t.TempDir()

	// Inside the source root is rejected (exact root, nested, and parent-created paths).
	for _, bad := range []string{root, filepath.Join(root, "ev"), filepath.Join(root, "ev2")} {
		if _, err := createOutputDir(bad, root); err == nil || !strings.Contains(err.Error(), "inside the source root") {
			t.Errorf("createOutputDir(%q) err = %v, want inside-source-root rejection", bad, err)
		}
	}

	// Fresh directory outside the root is created.
	good := filepath.Join(t.TempDir(), "ev-1")
	if _, err := createOutputDir(good, root); err != nil {
		t.Fatalf("createOutputDir fresh: %v", err)
	}
	if fi, err := os.Stat(good); err != nil || !fi.IsDir() {
		t.Fatalf("output dir missing: %v", err)
	}

	// A rerun to the same directory must fail: earlier evidence is never overwritten.
	_, err := createOutputDir(good, root)
	if err == nil || !strings.Contains(err.Error(), "already exists") {
		t.Fatalf("createOutputDir duplicate err = %v, want already-exists rejection", err)
	}
}

func TestCreateOutputDirRejectsSymlinkAndMissingParent(t *testing.T) {
	root := t.TempDir()
	outside := t.TempDir()
	alias := filepath.Join(outside, "alias")
	if err := os.Symlink(root, alias); err != nil {
		t.Skipf("symlink unavailable: %v", err)
	}
	if _, err := createOutputDir(filepath.Join(root, "out"), alias); err == nil {
		t.Fatal("symlinked source root accepted")
	}
	if runtime.GOOS == "windows" {
		if _, err := createOutputDir(filepath.Join(strings.ToUpper(root), "out"), strings.ToLower(root)); err == nil {
			t.Fatal("case variant inside source accepted")
		}
	}
	if _, err := createOutputDir(filepath.Join(alias, "out"), root); err == nil {
		t.Fatal("symlink into source accepted")
	}
	if _, err := os.Stat(filepath.Join(root, "out")); !os.IsNotExist(err) {
		t.Fatalf("source mutated: %v", err)
	}
	if _, err := createOutputDir(filepath.Join(outside, "missing", "out"), root); err == nil {
		t.Fatal("missing parent accepted")
	}
	if _, err := os.Stat(filepath.Join(outside, "missing")); !os.IsNotExist(err) {
		t.Fatalf("parent created: %v", err)
	}
}
