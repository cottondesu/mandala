package mandala

import (
	"slices"
	"strings"
	"testing"
)

func TestSanitizeGitEnvironmentRemovesGitOverrides(t *testing.T) {
	// Given
	environment := []string{
		"PATH=/usr/bin",
		"HOME=/home/test",
		"XDG_CONFIG_HOME=/home/test/.config",
		"git_dir=/outside/repository",
		"GIT_WORK_TREE=/outside/worktree",
		"GIT_COMMON_DIR=/outside/common",
		"GIT_INDEX_FILE=/outside/index",
		"GIT_OBJECT_DIRECTORY=/outside/objects",
		"GIT_ALTERNATE_OBJECT_DIRECTORIES=/outside/alternate-objects",
		"GIT_LITERAL_PATHSPECS=1",
		"GIT_GLOB_PATHSPECS=1",
		"GIT_NOGLOB_PATHSPECS=1",
		"GIT_ICASE_PATHSPECS=1",
		"GIT_CONFIG=/outside/config",
		"GIT_CONFIG_COUNT=1",
		"GIT_CONFIG_GLOBAL=/outside/global-config",
		"GIT_CONFIG_NOSYSTEM=1",
		"GIT_CONFIG_PARAMETERS=core.excludesfile=/outside/ignore",
		"GIT_CONFIG_SYSTEM=/outside/system-config",
		"git_config_key_0=core.excludesfile",
		"GIT_CONFIG_VALUE_0=/outside/ignore",
		"GIT_CEILING_DIRECTORIES=/outside/ceiling",
		"GIT_DISCOVERY_ACROSS_FILESYSTEM=0",
	}

	// When
	got := sanitizeGitEnvironment(environment)

	// Then
	want := []string{
		"PATH=/usr/bin",
		"HOME=/home/test",
		"XDG_CONFIG_HOME=/home/test/.config",
	}
	if !slices.Equal(got, want) {
		t.Fatalf("sanitized environment = %q, want %q", got, want)
	}
}

func TestGitDiagnosticDoesNotExposeGitStderr(t *testing.T) {
	result := gitCommandResult{
		stderr:   "secret path\x1b[31m\n",
		exitCode: 128,
	}

	diagnostic := gitDiagnostic(result)

	if diagnostic != "git exited with status 128" || strings.Contains(diagnostic, "secret") || strings.Contains(diagnostic, "\x1b") {
		t.Fatalf("diagnostic = %q", diagnostic)
	}
}
