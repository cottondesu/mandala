package mandala

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

const (
	gitExcludeRule   = ".mandala/"
	gitStatePath     = ".mandala/state.json"
	gitStatePathspec = ":(icase,literal).mandala/state.json"
)

func ensureGitLocalExclude(root string) error {
	gitPath, err := exec.LookPath("git")
	if err != nil {
		return handleUnavailableGit(root, err)
	}
	inside, runErr := runGitCommand(gitPath, root, "rev-parse", "--is-inside-work-tree")
	if runErr != nil || inside.exitCode != 0 {
		marker, markerErr := hasGitMarker(root)
		if markerErr != nil {
			return gitExcludeFailure("inspect Git worktree marker: %v", markerErr)
		}
		if !marker {
			return nil
		}
		if runErr != nil {
			return gitExcludeFailure("detect Git worktree: %v", runErr)
		}
		return gitExcludeFailure("detect Git worktree: %s", gitDiagnostic(inside))
	}
	switch strings.TrimSpace(inside.stdout) {
	case "false":
		return nil
	case "true":
	default:
		return gitExcludeFailure("detect Git worktree: unexpected output")
	}

	tracked, err := gitTracksState(gitPath, root)
	if err != nil {
		return gitExcludeFailure("check whether state is tracked: %v", err)
	}
	if tracked {
		return gitExcludeFailure("%s is tracked by Git", gitStatePath)
	}

	ignored, err := gitIgnoresDirectory(gitPath, root)
	if err != nil {
		return gitExcludeFailure("check existing ignore rules: %v", err)
	}
	if ignored {
		return nil
	}

	resolved, err := runGitCommand(gitPath, root, "rev-parse", "--path-format=absolute", "--git-common-dir")
	if err != nil {
		return gitExcludeFailure("resolve repository-local exclude: %v", err)
	}
	if resolved.exitCode != 0 {
		return gitExcludeFailure("resolve repository-local exclude: %s", gitDiagnostic(resolved))
	}
	commonDirectory := strings.TrimRight(resolved.stdout, "\r\n")
	if commonDirectory == "" || !filepath.IsAbs(commonDirectory) || strings.ContainsAny(commonDirectory, "\r\n") {
		return gitExcludeFailure("resolve repository-local exclude: Git returned an invalid path")
	}
	excludePath := filepath.Join(commonDirectory, "info", "exclude")
	change, err := appendGitExcludeRule(excludePath)
	if err != nil {
		if rollbackErr := change.rollback(); rollbackErr != nil {
			return gitExcludeFailure("update repository-local exclude: %v; rollback skipped: %v", err, rollbackErr)
		}
		return gitExcludeFailure("update repository-local exclude: %v", err)
	}
	ignored, err = gitIgnoresDirectory(gitPath, root)
	if err != nil {
		if rollbackErr := change.rollback(); rollbackErr != nil {
			return gitExcludeFailure("verify repository-local ignore: %v; rollback skipped: %v", err, rollbackErr)
		}
		return gitExcludeFailure("verify repository-local ignore: %v", err)
	}
	if !ignored {
		if rollbackErr := change.rollback(); rollbackErr != nil {
			return gitExcludeFailure("verify repository-local ignore: %s remains visible to Git; rollback skipped: %v", gitExcludeRule, rollbackErr)
		}
		return gitExcludeFailure("verify repository-local ignore: %s remains visible to Git", gitExcludeRule)
	}
	return nil
}

func handleUnavailableGit(root string, lookupErr error) error {
	marker, err := hasGitMarker(root)
	if err != nil {
		return gitExcludeFailure("inspect Git worktree marker: %v", err)
	}
	if marker {
		return gitExcludeFailure("Git executable is unavailable: %v", lookupErr)
	}
	return nil
}

func hasGitMarker(root string) (bool, error) {
	for current := root; ; current = filepath.Dir(current) {
		_, err := os.Lstat(filepath.Join(current, ".git"))
		if err == nil {
			return true, nil
		}
		if !errors.Is(err, os.ErrNotExist) {
			return false, err
		}
		parent := filepath.Dir(current)
		if parent == current {
			return false, nil
		}
	}
}

func gitTracksState(gitPath, root string) (bool, error) {
	result, err := runGitCommand(gitPath, root, "ls-files", "--error-unmatch", "--", gitStatePathspec)
	if err != nil {
		return false, err
	}
	switch result.exitCode {
	case 0:
		return true, nil
	case 1:
		return false, nil
	default:
		return false, fmt.Errorf("%s", gitDiagnostic(result))
	}
}

func gitIgnoresDirectory(gitPath, root string) (bool, error) {
	result, err := runGitCommand(gitPath, root, "check-ignore", "--no-index", "-q", "--", gitExcludeRule)
	if err != nil {
		return false, err
	}
	switch result.exitCode {
	case 0:
		return true, nil
	case 1:
		return false, nil
	default:
		return false, fmt.Errorf("%s", gitDiagnostic(result))
	}
}

func gitExcludeFailure(format string, args ...any) error {
	return fmt.Errorf("could not establish repository-local ignore for .mandala: "+format, args...)
}
