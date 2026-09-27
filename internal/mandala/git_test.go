package mandala

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestInitGitRootEstablishesRepositoryLocalIgnore(t *testing.T) {
	root := initFixtureRepository(t, nil)
	excludePath := fixtureExcludePath(t, root)
	beforeStatus := runFixtureGit(t, root, nil, "status", "--porcelain")
	beforeIgnore, err := os.ReadFile(filepath.Join(root, ".gitignore"))
	if err != nil && !os.IsNotExist(err) {
		t.Fatal(err)
	}

	code, stdout, diagnostics := runTestCommand(root, "init", "goal")

	if code != 0 || stdout != "" || diagnostics != "" {
		t.Fatalf("init: exit=%d stdout=%q stderr=%q", code, stdout, diagnostics)
	}
	requireStateExists(t, root)
	afterStatus := runFixtureGit(t, root, nil, "status", "--porcelain")
	if afterStatus != beforeStatus {
		t.Fatalf("status changed: before=%q after=%q", beforeStatus, afterStatus)
	}
	afterIgnore, err := os.ReadFile(filepath.Join(root, ".gitignore"))
	if err != nil && !os.IsNotExist(err) {
		t.Fatal(err)
	}
	if string(afterIgnore) != string(beforeIgnore) {
		t.Fatalf("project .gitignore changed: before=%q after=%q", beforeIgnore, afterIgnore)
	}
	exclude, err := os.ReadFile(excludePath)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasSuffix(string(exclude), ".mandala/\n") {
		t.Fatalf("exclude does not end in canonical rule: %q", exclude)
	}
}

func TestInitLeavesExistingIgnoreMetadataUnchanged(t *testing.T) {
	for _, test := range []struct {
		name            string
		tracked         map[string]string
		excludeContents string
	}{
		{name: "project gitignore", tracked: map[string]string{".gitignore": ".mandala/\n"}},
		{name: "repository exclude", excludeContents: "# keep\n.mandala/\n"},
	} {
		t.Run(test.name, func(t *testing.T) {
			root := initFixtureRepository(t, test.tracked)
			excludePath := fixtureExcludePath(t, root)
			if test.excludeContents != "" {
				if err := os.WriteFile(excludePath, []byte(test.excludeContents), 0600); err != nil {
					t.Fatal(err)
				}
			}
			before, err := os.ReadFile(excludePath)
			if err != nil {
				t.Fatal(err)
			}

			if err := Init(root, newTestState(t)); err != nil {
				t.Fatal(err)
			}

			after, err := os.ReadFile(excludePath)
			if err != nil {
				t.Fatal(err)
			}
			if string(after) != string(before) {
				t.Fatalf("exclude changed: before=%q after=%q", before, after)
			}
			requireStateExists(t, root)
			if status := runFixtureGit(t, root, nil, "status", "--porcelain"); status != "" {
				t.Fatalf("git status is not clean: %q", status)
			}
		})
	}
}

func TestInitPreservesExcludeContentAndAddsSeparator(t *testing.T) {
	root := initFixtureRepository(t, nil)
	excludePath := fixtureExcludePath(t, root)
	const before = "# existing\nother"
	if err := os.WriteFile(excludePath, []byte(before), 0600); err != nil {
		t.Fatal(err)
	}

	if err := Init(root, newTestState(t)); err != nil {
		t.Fatal(err)
	}

	after, err := os.ReadFile(excludePath)
	if err != nil {
		t.Fatal(err)
	}
	if string(after) != before+"\n.mandala/\n" {
		t.Fatalf("exclude = %q", after)
	}
}

func TestInitAppendsAfterRepositoryExcludeNegation(t *testing.T) {
	root := initFixtureRepository(t, nil)
	excludePath := fixtureExcludePath(t, root)
	const before = ".mandala/\n!.mandala/\n!.mandala/state.json\n"
	if err := os.WriteFile(excludePath, []byte(before), 0600); err != nil {
		t.Fatal(err)
	}

	if err := Init(root, newTestState(t)); err != nil {
		t.Fatal(err)
	}

	after, err := os.ReadFile(excludePath)
	if err != nil {
		t.Fatal(err)
	}
	if string(after) != before+".mandala/\n" {
		t.Fatalf("exclude = %q", after)
	}
	if status := runFixtureGit(t, root, nil, "status", "--porcelain"); status != "" {
		t.Fatalf("git status is not clean: %q", status)
	}
}

func TestInitPreservesExistingMandalaContents(t *testing.T) {
	root := initFixtureRepository(t, nil)
	directory := filepath.Join(root, ".mandala")
	if err := os.Mkdir(directory, 0700); err != nil {
		t.Fatal(err)
	}
	note := filepath.Join(directory, "user-note.txt")
	if err := os.WriteFile(note, []byte("keep"), 0600); err != nil {
		t.Fatal(err)
	}

	if err := Init(root, newTestState(t)); err != nil {
		t.Fatal(err)
	}

	contents, err := os.ReadFile(note)
	if err != nil || string(contents) != "keep" {
		t.Fatalf("existing content changed: %q %v", contents, err)
	}
	requireStateExists(t, root)
	if status := runFixtureGit(t, root, nil, "status", "--porcelain"); status != "" {
		t.Fatalf("git status is not clean: %q", status)
	}
}

func TestInitCleanInitDoesNotDuplicateExcludeRule(t *testing.T) {
	root := initFixtureRepository(t, nil)
	excludePath := fixtureExcludePath(t, root)
	if err := Init(root, newTestState(t)); err != nil {
		t.Fatal(err)
	}
	before, err := os.ReadFile(excludePath)
	if err != nil {
		t.Fatal(err)
	}
	if err := Clean(root, ""); err != nil {
		t.Fatal(err)
	}
	if err := Init(root, newTestState(t)); err != nil {
		t.Fatal(err)
	}
	after, err := os.ReadFile(excludePath)
	if err != nil {
		t.Fatal(err)
	}
	if string(after) != string(before) {
		t.Fatalf("exclude changed on repeated init: before=%q after=%q", before, after)
	}
	if strings.Count(string(after), ".mandala/\n") != 1 {
		t.Fatalf("canonical rule count = %d", strings.Count(string(after), ".mandala/\n"))
	}
	if err := Clean(root, ""); err != nil {
		t.Fatal(err)
	}
	final, err := os.ReadFile(excludePath)
	if err != nil || string(final) != string(after) {
		t.Fatalf("clean changed exclude: %v before=%q after=%q", err, after, final)
	}
}
