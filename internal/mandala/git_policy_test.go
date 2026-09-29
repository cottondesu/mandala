package mandala

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestInitAddsDirectoryRuleWhenOnlyStateFileIsIgnored(t *testing.T) {
	root := initFixtureRepository(t, map[string]string{
		".gitignore": ".mandala/state.json\n",
	})
	directory := filepath.Join(root, ".mandala")
	if err := os.Mkdir(directory, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(directory, "user-note.txt"), []byte("keep"), 0600); err != nil {
		t.Fatal(err)
	}

	if err := Init(root, newTestState(t)); err != nil {
		t.Fatal(err)
	}

	exclude, err := os.ReadFile(fixtureExcludePath(t, root))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasSuffix(string(exclude), ".mandala/\n") {
		t.Fatalf("exclude does not cover the Mandala directory: %q", exclude)
	}
	if status := runFixtureGit(t, root, nil, "status", "--porcelain"); status != "" {
		t.Fatalf("Mandala directory contents are visible: %q", status)
	}
}

func TestInitRefusesTrackedStateMissingFromWorktree(t *testing.T) {
	root := initFixtureRepository(t, nil)
	statePath := filepath.Join(root, filepath.FromSlash(gitStatePath))
	if err := os.Mkdir(filepath.Dir(statePath), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(statePath, []byte("{\"schema_version\":1,\"goal\":\"old\",\"cells\":[]}\n"), 0600); err != nil {
		t.Fatal(err)
	}
	runFixtureGit(t, root, nil, "add", "--", gitStatePath)
	runFixtureGit(t, root, nil, "commit", "--quiet", "-m", "track state")
	if err := os.Remove(statePath); err != nil {
		t.Fatal(err)
	}
	excludePath := fixtureExcludePath(t, root)
	before, err := os.ReadFile(excludePath)
	if err != nil {
		t.Fatal(err)
	}

	err = Init(root, newTestState(t))

	if err == nil || !strings.Contains(err.Error(), "tracked") {
		t.Fatalf("tracked state error = %v", err)
	}
	if _, statErr := os.Stat(statePath); !os.IsNotExist(statErr) {
		t.Fatalf("tracked state was recreated: %v", statErr)
	}
	after, err := os.ReadFile(excludePath)
	if err != nil {
		t.Fatal(err)
	}
	if string(after) != string(before) {
		t.Fatalf("failed init changed exclude: before=%q after=%q", before, after)
	}
}

func TestInitRefusesCaseVariantTrackedStateMissingFromWorktree(t *testing.T) {
	t.Setenv("GIT_LITERAL_PATHSPECS", "1")
	root := initFixtureRepository(t, nil)
	const trackedPath = ".Mandala/state.json"
	statePath := filepath.Join(root, filepath.FromSlash(trackedPath))
	if err := os.Mkdir(filepath.Dir(statePath), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(statePath, []byte("{\"schema_version\":1,\"goal\":\"old\",\"cells\":[]}\n"), 0600); err != nil {
		t.Fatal(err)
	}
	runFixtureGit(t, root, nil, "add", "--", trackedPath)
	runFixtureGit(t, root, nil, "commit", "--quiet", "-m", "track case variant state")
	if err := os.Remove(statePath); err != nil {
		t.Fatal(err)
	}
	excludePath := fixtureExcludePath(t, root)
	before, err := os.ReadFile(excludePath)
	if err != nil {
		t.Fatal(err)
	}

	err = Init(root, newTestState(t))

	if err == nil || !strings.Contains(err.Error(), "tracked") {
		t.Fatalf("case variant tracked state error = %v", err)
	}
	if _, statErr := os.Stat(filepath.Join(root, filepath.FromSlash(gitStatePath))); !os.IsNotExist(statErr) {
		t.Fatalf("case variant tracked state was recreated: %v", statErr)
	}
	after, err := os.ReadFile(excludePath)
	if err != nil {
		t.Fatal(err)
	}
	if string(after) != string(before) {
		t.Fatalf("failed init changed exclude: before=%q after=%q", before, after)
	}
}

func TestInitDoesNotTrustGitConfigEnvironmentIgnore(t *testing.T) {
	root := initFixtureRepository(t, nil)
	ignore := filepath.Join(t.TempDir(), "ignore")
	if err := os.WriteFile(ignore, []byte(".mandala/\n"), 0600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("GIT_CONFIG_COUNT", "1")
	t.Setenv("GIT_CONFIG_KEY_0", "core.excludesfile")
	t.Setenv("GIT_CONFIG_VALUE_0", ignore)

	if err := Init(root, newTestState(t)); err != nil {
		t.Fatal(err)
	}

	exclude, err := os.ReadFile(fixtureExcludePath(t, root))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasSuffix(string(exclude), ".mandala/\n") {
		t.Fatalf("repository-local exclude was not updated: %q", exclude)
	}
}

func TestInitDoesNotTrustGlobalExcludesFile(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, "xdg"))
	ignore := filepath.Join(home, "global-ignore")
	if err := os.WriteFile(ignore, []byte(".mandala/\n"), 0600); err != nil {
		t.Fatal(err)
	}
	runFixtureGit(t, home, nil, "config", "--global", "core.excludesFile", ignore)

	root := t.TempDir()
	runFixtureGit(t, root, nil, "init", "--quiet")
	excludePath := fixtureExcludePath(t, root)
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
	if string(after) != string(before)+gitExcludeRule+"\n" {
		t.Fatalf("repository-local exclude was not updated: before=%q after=%q", before, after)
	}
}

func TestGitExcludeRollbackRejectsReplacedInfoDirectory(t *testing.T) {
	root := initFixtureRepository(t, nil)
	excludePath := fixtureExcludePath(t, root)
	before, err := os.ReadFile(excludePath)
	if err != nil {
		t.Fatal(err)
	}
	change, err := appendGitExcludeRule(excludePath)
	if err != nil {
		t.Fatal(err)
	}

	infoPath := filepath.Dir(excludePath)
	originalInfoPath := infoPath + "-original"
	if err := os.Rename(infoPath, originalInfoPath); err != nil {
		t.Fatal(err)
	}
	target := t.TempDir()
	if err := os.Symlink(target, infoPath); err != nil {
		t.Skipf("symlink unavailable: %v", err)
	}

	err = change.rollback()

	if err == nil || !strings.Contains(err.Error(), "safe directory") {
		t.Fatalf("rollback after info replacement error = %v", err)
	}
	if entries, readErr := os.ReadDir(target); readErr != nil || len(entries) != 0 {
		t.Fatalf("replacement target changed: entries=%v err=%v", entries, readErr)
	}
	after, readErr := os.ReadFile(filepath.Join(originalInfoPath, "exclude"))
	if readErr != nil {
		t.Fatal(readErr)
	}
	want := string(before) + gitExcludeRule + "\n"
	if string(after) != want {
		t.Fatalf("original exclude changed during rejected rollback: got=%q want=%q", after, want)
	}
}

func TestGitExcludeRollbackRemovesEmptyCreatedFile(t *testing.T) {
	root := initFixtureRepository(t, nil)
	excludePath := fixtureExcludePath(t, root)
	if err := os.Remove(excludePath); err != nil {
		t.Fatal(err)
	}
	file, err := os.OpenFile(excludePath, os.O_CREATE|os.O_EXCL|os.O_RDWR, 0600)
	if err != nil {
		t.Fatal(err)
	}
	identity, statErr := file.Stat()
	closeErr := file.Close()
	if statErr != nil || closeErr != nil {
		t.Fatal(errors.Join(statErr, closeErr))
	}
	change := gitExcludeChange{
		path:     excludePath,
		identity: identity,
		created:  true,
	}

	if err := change.rollback(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(excludePath); !os.IsNotExist(err) {
		t.Fatalf("empty created exclude was not removed: %v", err)
	}
}
