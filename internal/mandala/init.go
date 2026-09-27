package mandala

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

func Init(root string, state State) error {
	if err := state.Validate(); err != nil {
		return err
	}
	root, err := canonicalDirectory(root)
	if err != nil {
		return err
	}
	for parent := filepath.Dir(root); parent != root; parent = filepath.Dir(parent) {
		_, err := os.Lstat(filepath.Join(parent, ".mandala"))
		if err == nil {
			return problem("E_STATE_INVALID", "cannot initialize below an existing .mandala boundary")
		}
		if err != nil && !errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("inspect ancestor project: %w", err)
		}
		if filepath.Dir(parent) == parent {
			break
		}
	}
	dir := filepath.Join(root, ".mandala")
	if err := validateInitTarget(dir); err != nil {
		return err
	}
	if err := ensureGitLocalExclude(root); err != nil {
		return err
	}
	if err := os.Mkdir(dir, 0700); err != nil && !errors.Is(err, os.ErrExist) {
		return fmt.Errorf("create project directory: %w", err)
	}
	project, err := openMandalaDirectory(root)
	if err != nil {
		return err
	}
	defer project.Close()
	return writeState(project, state, false)
}

func validateInitTarget(dir string) error {
	info, err := os.Lstat(dir)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("inspect project directory: %w", err)
	}
	if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return problem("E_STATE_INVALID", "%q is not a regular directory", dir)
	}
	statePath := filepath.Join(dir, "state.json")
	if _, err := os.Lstat(statePath); err == nil {
		return problem("E_STATE_INVALID", "state.json already exists in %q", dir)
	} else if !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("inspect state before init: %w", err)
	}
	return nil
}
