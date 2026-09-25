package mandala

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
)

func ProjectRoot(start, override string) (string, error) {
	root, err := findProject(start, override)
	if err != nil {
		return "", err
	}
	return root, nil
}

func findProject(start, override string) (string, error) {
	path := start
	if override != "" {
		path = override
		if !filepath.IsAbs(path) {
			path = filepath.Join(start, path)
		}
	}
	root, err := canonicalDirectory(path)
	if err != nil {
		return "", err
	}
	for {
		dir := filepath.Join(root, ".mandala")
		info, err := os.Lstat(dir)
		if err == nil {
			if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
				return "", problem("E_STATE_INVALID", "%q is not a regular directory", dir)
			}
			return root, nil
		}
		if !errors.Is(err, os.ErrNotExist) {
			return "", fmt.Errorf("inspect project directory: %w", err)
		}
		if override != "" || filepath.Dir(root) == root {
			break
		}
		root = filepath.Dir(root)
	}
	return "", problem("E_NO_PROJECT", "no .mandala project found")
}

func canonicalDirectory(path string) (string, error) {
	root, err := filepath.Abs(path)
	if err != nil {
		return "", fmt.Errorf("resolve project path: %w", err)
	}
	root, err = filepath.EvalSymlinks(root)
	if err != nil {
		return "", fmt.Errorf("resolve project path: %w", err)
	}
	info, err := os.Stat(root)
	if err != nil {
		return "", fmt.Errorf("inspect project path: %w", err)
	}
	if !info.IsDir() {
		return "", problem("E_USAGE", "project path %q is not a directory", path)
	}
	return root, nil
}

func Clean(start, override string) error {
	path := start
	if override != "" {
		path = override
		if !filepath.IsAbs(path) {
			path = filepath.Join(start, path)
		}
	}
	root, err := canonicalDirectory(path)
	if err != nil {
		return err
	}
	dir := filepath.Join(root, ".mandala")
	if _, err := os.Lstat(dir); errors.Is(err, os.ErrNotExist) {
		return nil
	} else if err != nil {
		return fmt.Errorf("inspect project directory: %w", err)
	}
	stateDir, err := openMandalaDirectory(root)
	if err != nil {
		return err
	}
	defer stateDir.Close()
	_, err = stateDir.Lstat("state.json")
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("inspect state: %w", err)
	}
	if _, err := loadFromDirectory(stateDir); err != nil {
		return err
	}
	if err := stateDir.Remove("state.json"); err != nil {
		return fmt.Errorf("remove state: %w", err)
	}
	directory, err := stateDir.Open(".")
	if err != nil {
		return fmt.Errorf("inspect project directory after cleanup: %w", err)
	}
	entries, readErr := directory.ReadDir(1)
	closeErr := directory.Close()
	if readErr != nil && !errors.Is(readErr, io.EOF) {
		return fmt.Errorf("inspect project directory after cleanup: %w", readErr)
	}
	if closeErr != nil {
		return fmt.Errorf("close project directory after cleanup: %w", closeErr)
	}
	if len(entries) == 0 {
		project, err := os.OpenRoot(root)
		if err != nil {
			return fmt.Errorf("open project for cleanup: %w", err)
		}
		defer project.Close()
		current, err := project.Lstat(".mandala")
		opened, statErr := stateDir.Stat(".")
		if err != nil || statErr != nil || !os.SameFile(current, opened) {
			return problem("E_STATE_INVALID", "project directory changed during cleanup")
		}
		if err := project.Remove(".mandala"); err != nil {
			return fmt.Errorf("remove empty project directory: %w", err)
		}
	}
	return nil
}
