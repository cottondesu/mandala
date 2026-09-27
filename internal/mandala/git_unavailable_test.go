package mandala

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestInitWithoutGitAllowsNonGitDirectory(t *testing.T) {
	root := t.TempDir()
	t.Setenv("PATH", t.TempDir())

	if err := Init(root, newTestState(t)); err != nil {
		t.Fatal(err)
	}

	requireStateExists(t, root)
}

func TestInitWithoutGitRejectsApparentWorktree(t *testing.T) {
	root := t.TempDir()
	if err := os.Mkdir(filepath.Join(root, ".git"), 0700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", t.TempDir())

	err := Init(root, newTestState(t))

	if err == nil || !strings.Contains(err.Error(), "repository-local ignore") {
		t.Fatalf("missing git error = %v", err)
	}
	if _, statErr := os.Stat(filepath.Join(root, ".mandala")); !os.IsNotExist(statErr) {
		t.Fatalf("failed init created project: %v", statErr)
	}
}

func TestInitRejectsBrokenGitMarkerWithoutCreatingProject(t *testing.T) {
	root := t.TempDir()
	if err := os.Mkdir(filepath.Join(root, ".git"), 0700); err != nil {
		t.Fatal(err)
	}

	err := Init(root, newTestState(t))

	if err == nil || !strings.Contains(err.Error(), "repository-local ignore") {
		t.Fatalf("broken repository error = %v", err)
	}
	if _, statErr := os.Stat(filepath.Join(root, ".mandala")); !os.IsNotExist(statErr) {
		t.Fatalf("failed init created project: %v", statErr)
	}
}
