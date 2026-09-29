package mandala

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"
)

const gitOutputLimit = 32 << 10

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

func runGitCommand(gitPath, root string, args ...string) (gitCommandResult, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	commandArgs := append([]string{"-c", "core.excludesFile=" + os.DevNull, "-C", root}, args...)
	cmd := exec.CommandContext(ctx, gitPath, commandArgs...)
	cmd.Env = sanitizeGitEnvironment(os.Environ())
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

func sanitizeGitEnvironment(environment []string) []string {
	sanitized := make([]string, 0, len(environment))
	for _, entry := range environment {
		key, _, _ := strings.Cut(entry, "=")
		key = strings.ToUpper(key)
		switch key {
		case "GIT_DIR", "GIT_WORK_TREE", "GIT_COMMON_DIR", "GIT_INDEX_FILE", "GIT_OBJECT_DIRECTORY", "GIT_ALTERNATE_OBJECT_DIRECTORIES",
			"GIT_LITERAL_PATHSPECS", "GIT_GLOB_PATHSPECS", "GIT_NOGLOB_PATHSPECS", "GIT_ICASE_PATHSPECS",
			"GIT_CONFIG", "GIT_CONFIG_COUNT", "GIT_CONFIG_GLOBAL", "GIT_CONFIG_NOSYSTEM", "GIT_CONFIG_PARAMETERS", "GIT_CONFIG_SYSTEM",
			"GIT_CEILING_DIRECTORIES", "GIT_DISCOVERY_ACROSS_FILESYSTEM":
			continue
		default:
			if strings.HasPrefix(key, "GIT_CONFIG_KEY_") || strings.HasPrefix(key, "GIT_CONFIG_VALUE_") {
				continue
			}
			sanitized = append(sanitized, entry)
		}
	}
	return sanitized
}

func gitDiagnostic(result gitCommandResult) string {
	return fmt.Sprintf("git exited with status %d", result.exitCode)
}
