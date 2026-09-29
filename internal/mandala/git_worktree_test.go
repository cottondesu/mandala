package mandala

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestInitNestedProjectIsIgnoredFromWorktreeRoot(t *testing.T) {
	root := initFixtureRepository(t, nil)
	project := filepath.Join(root, "packages", "foo")
	if err := os.MkdirAll(project, 0700); err != nil {
		t.Fatal(err)
	}

	if err := Init(project, newTestState(t)); err != nil {
		t.Fatal(err)
	}

	requireStateExists(t, project)
	if status := runFixtureGit(t, root, nil, "status", "--porcelain"); status != "" {
		t.Fatalf("nested project is visible: %q", status)
	}
}

func TestInitLinkedWorktreeUsesGitResolvedExclude(t *testing.T) {
	root := initFixtureRepository(t, nil)
	linked := filepath.Join(t.TempDir(), "linked")
	runFixtureGit(t, root, nil, "worktree", "add", "--quiet", "--detach", linked, "HEAD")
	gitfile, err := os.Lstat(filepath.Join(linked, ".git"))
	if err != nil || !gitfile.Mode().IsRegular() {
		t.Fatalf("linked worktree .git is not a gitfile: %v %v", gitfile, err)
	}
	excludePath := fixtureExcludePath(t, linked)
	if strings.HasPrefix(excludePath, filepath.Join(linked, ".git")) {
		t.Fatalf("exclude path was derived from worktree .git path: %q", excludePath)
	}

	if err := Init(linked, newTestState(t)); err != nil {
		t.Fatal(err)
	}

	requireStateExists(t, linked)
	if status := runFixtureGit(t, linked, nil, "status", "--porcelain"); status != "" {
		t.Fatalf("linked worktree is dirty: %q", status)
	}
	exclude, err := os.ReadFile(excludePath)
	if err != nil || !strings.Contains(string(exclude), ".mandala/\n") {
		t.Fatalf("resolved exclude was not updated: %v %q", err, exclude)
	}
}

func TestInitCreatesMissingExcludeFilePrivately(t *testing.T) {
	root := initFixtureRepository(t, nil)
	excludePath := fixtureExcludePath(t, root)
	if err := os.Remove(excludePath); err != nil {
		t.Fatal(err)
	}

	if err := Init(root, newTestState(t)); err != nil {
		t.Fatal(err)
	}

	info, err := os.Stat(excludePath)
	if err != nil {
		t.Fatal(err)
	}
	if runtime.GOOS != "windows" && info.Mode().Perm() != 0600 {
		t.Fatalf("exclude permissions = %o", info.Mode().Perm())
	}
	contents, err := os.ReadFile(excludePath)
	if err != nil || string(contents) != ".mandala/\n" {
		t.Fatalf("exclude = %q, err = %v", contents, err)
	}
}

func TestInitCreatesMissingGitInfoDirectory(t *testing.T) {
	root := t.TempDir()
	runFixtureGit(t, root, nil, "init", "--quiet", "--template=")
	infoPath := filepath.Join(root, ".git", "info")
	if _, err := os.Stat(infoPath); !os.IsNotExist(err) {
		t.Fatalf("template-free repository already has info directory: %v", err)
	}

	if err := Init(root, newTestState(t)); err != nil {
		t.Fatal(err)
	}

	requireStateExists(t, root)
	contents, err := os.ReadFile(filepath.Join(infoPath, "exclude"))
	if err != nil || string(contents) != ".mandala/\n" {
		t.Fatalf("exclude = %q, err = %v", contents, err)
	}
	if status := runFixtureGit(t, root, nil, "status", "--porcelain"); status != "" {
		t.Fatalf("git status is not clean: %q", status)
	}
}

func TestInitRollsBackCreatedGitInfoDirectory(t *testing.T) {
	root := t.TempDir()
	runFixtureGit(t, root, nil, "init", "--quiet", "--template=")
	if err := os.WriteFile(filepath.Join(root, ".gitignore"), []byte("!.mandala/\n!.mandala/state.json\n"), 0600); err != nil {
		t.Fatal(err)
	}
	runFixtureGit(t, root, nil, "add", "--", ".gitignore")
	runFixtureGit(t, root, nil, "commit", "--quiet", "-m", "add negation")
	infoPath := filepath.Join(root, ".git", "info")

	err := Init(root, newTestState(t))

	if err == nil || !strings.Contains(err.Error(), "repository-local ignore") {
		t.Fatalf("negation failure = %v", err)
	}
	if _, statErr := os.Stat(infoPath); !os.IsNotExist(statErr) {
		t.Fatalf("failed init left created info directory: %v", statErr)
	}
}

func TestInitRejectsSymlinkedGitInfoDirectory(t *testing.T) {
	root := t.TempDir()
	runFixtureGit(t, root, nil, "init", "--quiet", "--template=")
	target := t.TempDir()
	if err := os.Symlink(target, filepath.Join(root, ".git", "info")); err != nil {
		t.Skipf("symlink unavailable: %v", err)
	}

	err := Init(root, newTestState(t))

	if err == nil || !strings.Contains(err.Error(), "safe directory") {
		t.Fatalf("unsafe info directory error = %v", err)
	}
	if _, statErr := os.Stat(filepath.Join(root, ".mandala")); !os.IsNotExist(statErr) {
		t.Fatalf("failed init created project: %v", statErr)
	}
	if entries, readErr := os.ReadDir(target); readErr != nil || len(entries) != 0 {
		t.Fatalf("symlink target changed: entries=%v err=%v", entries, readErr)
	}
}

func TestInitRejectsUnsafeExcludeWithoutCreatingProject(t *testing.T) {
	root := initFixtureRepository(t, nil)
	excludePath := fixtureExcludePath(t, root)
	target := filepath.Join(t.TempDir(), "target")
	if err := os.WriteFile(target, []byte("keep"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(excludePath); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(target, excludePath); err != nil {
		t.Skipf("symlink unavailable: %v", err)
	}

	err := Init(root, newTestState(t))

	if err == nil || !strings.Contains(err.Error(), "repository-local ignore") {
		t.Fatalf("unsafe exclude error = %v", err)
	}
	if _, statErr := os.Stat(filepath.Join(root, ".mandala")); !os.IsNotExist(statErr) {
		t.Fatalf("failed init created project: %v", statErr)
	}
	contents, readErr := os.ReadFile(target)
	if readErr != nil || string(contents) != "keep" {
		t.Fatalf("symlink target changed: %q %v", contents, readErr)
	}
}

func TestInitRollsBackExcludeWhenPostWriteVerificationFails(t *testing.T) {
	root := initFixtureRepository(t, map[string]string{
		".gitignore": "!.mandala/\n!.mandala/state.json\n",
	})
	excludePath := fixtureExcludePath(t, root)
	before, err := os.ReadFile(excludePath)
	if err != nil {
		t.Fatal(err)
	}

	code, stdout, diagnostics := runTestCommand(root, "init", "goal")

	if code != 2 || stdout != "" || !strings.Contains(diagnostics, "E_IO") || !strings.Contains(diagnostics, "repository-local ignore") {
		t.Fatalf("negation failure: exit=%d stdout=%q stderr=%q", code, stdout, diagnostics)
	}
	if _, statErr := os.Stat(filepath.Join(root, ".mandala")); !os.IsNotExist(statErr) {
		t.Fatalf("failed init created visible project: %v", statErr)
	}
	after, err := os.ReadFile(excludePath)
	if err != nil {
		t.Fatal(err)
	}
	if string(after) != string(before) {
		t.Fatalf("failed init left exclude change: before=%q after=%q", before, after)
	}
}

func TestGitExcludeRollbackPreservesConcurrentAppend(t *testing.T) {
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
	concurrent := []byte("# concurrent change\n")
	file, err := os.OpenFile(excludePath, os.O_WRONLY|os.O_APPEND, 0)
	if err != nil {
		t.Fatal(err)
	}
	if _, writeErr := file.Write(concurrent); writeErr != nil {
		closeErr := file.Close()
		t.Fatal(errors.Join(writeErr, closeErr))
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}

	err = change.rollback()

	if err == nil {
		t.Fatal("rollback ignored concurrent modification")
	}
	after, err := os.ReadFile(excludePath)
	if err != nil {
		t.Fatal(err)
	}
	want := string(before) + gitExcludeRule + "\n" + string(concurrent)
	if string(after) != want {
		t.Fatalf("concurrent content changed: got=%q want=%q", after, want)
	}
}
