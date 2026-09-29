package mandala

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestInitNestedProjectIgnoresGitDiscoveryEnvironment(t *testing.T) {
	// Given
	root := initFixtureRepository(t, nil)
	project := filepath.Join(root, "packages", "foo")
	if err := os.MkdirAll(project, 0700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("GIT_CEILING_DIRECTORIES", root)
	t.Setenv("GIT_DISCOVERY_ACROSS_FILESYSTEM", "0")

	// When
	if err := Init(project, newTestState(t)); err != nil {
		t.Fatal(err)
	}

	// Then
	requireStateExists(t, project)
	exclude, err := os.ReadFile(fixtureExcludePath(t, root))
	if err != nil || !strings.HasSuffix(string(exclude), ".mandala/\n") {
		t.Fatalf("repository-local exclude = %q, err = %v", exclude, err)
	}
	if status := runFixtureGit(t, root, nil, "status", "--porcelain"); status != "" {
		t.Fatalf("nested project is visible: %q", status)
	}
}
