package mandala

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

const (
	gitExcludeRule = ".mandala/"
	gitStatePath   = ".mandala/state.json"
	gitOutputLimit = 32 << 10
)

type limitedOutput struct {
	data  []byte
	limit int
}

func (output *limitedOutput) Write(data []byte) (int, error) {
	remaining := output.limit - len(output.data)
	if remaining > 0 {
		if len(data) < remaining {
			remaining = len(data)
		}
		output.data = append(output.data, data[:remaining]...)
	}
	return len(data), nil
}

func (output *limitedOutput) String() string {
	return string(output.data)
}

type gitCommandResult struct {
	stdout   string
	stderr   string
	exitCode int
}

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

	ignored, err := gitIgnoresState(gitPath, root)
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
		return gitExcludeFailure("update repository-local exclude: %v", err)
	}
	ignored, err = gitIgnoresState(gitPath, root)
	if err != nil {
		if rollbackErr := change.rollback(); rollbackErr != nil {
			return gitExcludeFailure("verify repository-local ignore: %v; rollback skipped: %v", err, rollbackErr)
		}
		return gitExcludeFailure("verify repository-local ignore: %v", err)
	}
	if !ignored {
		if rollbackErr := change.rollback(); rollbackErr != nil {
			return gitExcludeFailure("verify repository-local ignore: %s remains visible to Git; rollback skipped: %v", gitStatePath, rollbackErr)
		}
		return gitExcludeFailure("verify repository-local ignore: %s remains visible to Git", gitStatePath)
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

func gitIgnoresState(gitPath, root string) (bool, error) {
	result, err := runGitCommand(gitPath, root, "check-ignore", "--no-index", "-q", "--", gitStatePath)
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

func runGitCommand(gitPath, root string, args ...string) (gitCommandResult, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	commandArgs := append([]string{"-C", root}, args...)
	cmd := exec.CommandContext(ctx, gitPath, commandArgs...)
	cmd.Env = make([]string, 0, len(os.Environ()))
	for _, entry := range os.Environ() {
		key, _, _ := strings.Cut(entry, "=")
		switch key {
		case "GIT_DIR", "GIT_WORK_TREE", "GIT_COMMON_DIR", "GIT_INDEX_FILE", "GIT_OBJECT_DIRECTORY", "GIT_ALTERNATE_OBJECT_DIRECTORIES":
			continue
		default:
			cmd.Env = append(cmd.Env, entry)
		}
	}
	stdout := limitedOutput{limit: gitOutputLimit}
	stderr := limitedOutput{limit: gitOutputLimit}
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()
	result := gitCommandResult{stdout: stdout.String(), stderr: stderr.String()}
	if err == nil {
		return result, nil
	}
	if ctx.Err() != nil {
		return result, fmt.Errorf("git command timed out: %w", ctx.Err())
	}
	var exitError *exec.ExitError
	if errors.As(err, &exitError) {
		result.exitCode = exitError.ExitCode()
		return result, nil
	}
	return result, fmt.Errorf("start git command: %w", err)
}

func gitDiagnostic(result gitCommandResult) string {
	diagnostic := strings.TrimSpace(result.stderr)
	if diagnostic == "" {
		return fmt.Sprintf("git exited with status %d", result.exitCode)
	}
	return diagnostic
}

func gitExcludeFailure(format string, args ...any) error {
	return fmt.Errorf("could not establish repository-local ignore for .mandala: "+format, args...)
}
