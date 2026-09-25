package mandala

import (
	"bytes"
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"unicode/utf8"
)

const maxStateBytes = 1 << 20

type wireState struct {
	SchemaVersion *int        `json:"schema_version"`
	Goal          *string     `json:"goal"`
	Cells         *[]wireCell `json:"cells"`
}

type wireCell struct {
	ID       *string `json:"id"`
	Parent   *string `json:"parent"`
	Status   *Status `json:"status"`
	Required *bool   `json:"required"`
}

func Load(root string) (State, error) {
	dir, err := openMandalaDirectory(root)
	if err != nil {
		return State{}, err
	}
	defer dir.Close()
	return loadFromDirectory(dir)
}

func loadFromDirectory(dir *os.Root) (State, error) {
	info, err := dir.Lstat("state.json")
	if errors.Is(err, os.ErrNotExist) {
		return State{}, problem("E_NO_PROJECT", "state.json is missing in %q", dir.Name())
	}
	if err != nil {
		return State{}, fmt.Errorf("inspect state: %w", err)
	}
	if !info.Mode().IsRegular() {
		return State{}, problem("E_STATE_INVALID", "state.json in %q is not a regular file", dir.Name())
	}
	if info.Size() > maxStateBytes {
		return State{}, problem("E_STATE_INVALID", "state exceeds 1 MiB")
	}
	f, err := dir.Open("state.json")
	if err != nil {
		return State{}, fmt.Errorf("open state: %w", err)
	}
	defer f.Close()
	opened, err := f.Stat()
	if err != nil {
		return State{}, fmt.Errorf("inspect opened state: %w", err)
	}
	if !opened.Mode().IsRegular() || !os.SameFile(info, opened) {
		return State{}, problem("E_STATE_INVALID", "state.json changed while opening")
	}
	data, err := io.ReadAll(io.LimitReader(f, maxStateBytes+1))
	if err != nil {
		return State{}, fmt.Errorf("read state: %w", err)
	}
	if len(data) > maxStateBytes || !utf8.Valid(data) {
		return State{}, problem("E_STATE_INVALID", "state is oversized or not UTF-8")
	}
	var raw wireState
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&raw); err != nil {
		return State{}, problem("E_STATE_INVALID", "invalid JSON: %v", err)
	}
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		return State{}, problem("E_STATE_INVALID", "state contains trailing JSON")
	}
	if err := checkUniqueJSONKeys(json.NewDecoder(bytes.NewReader(data))); err != nil {
		return State{}, problem("E_STATE_INVALID", "invalid JSON: %v", err)
	}
	if raw.SchemaVersion == nil || raw.Goal == nil || raw.Cells == nil {
		return State{}, problem("E_STATE_INVALID", "state is missing a required field")
	}
	s := State{SchemaVersion: *raw.SchemaVersion, Goal: *raw.Goal, Cells: make([]Cell, 0, len(*raw.Cells))}
	for i, cell := range *raw.Cells {
		if cell.ID == nil || cell.Parent == nil || cell.Status == nil || cell.Required == nil {
			return State{}, problem("E_STATE_INVALID", "cell %d is missing a required field", i)
		}
		s.Cells = append(s.Cells, Cell{ID: *cell.ID, Parent: *cell.Parent, Status: *cell.Status, Required: *cell.Required})
	}
	if err := s.Validate(); err != nil {
		return State{}, err
	}
	return s, nil
}

func checkUniqueJSONKeys(decoder *json.Decoder) error {
	token, err := decoder.Token()
	if err != nil {
		return err
	}
	delimiter, ok := token.(json.Delim)
	if !ok {
		return nil
	}
	switch delimiter {
	case '{':
		seen := make(map[string]bool)
		for decoder.More() {
			keyToken, err := decoder.Token()
			if err != nil {
				return err
			}
			key, ok := keyToken.(string)
			if !ok {
				return fmt.Errorf("expected JSON field name")
			}
			switch key {
			case "schema_version", "goal", "cells", "id", "parent", "status", "required":
			default:
				return fmt.Errorf("unknown or noncanonical JSON field %q", key)
			}
			if seen[key] {
				return fmt.Errorf("duplicate JSON field %q", key)
			}
			seen[key] = true
			if err := checkUniqueJSONKeys(decoder); err != nil {
				return err
			}
		}
	case '[':
		for decoder.More() {
			if err := checkUniqueJSONKeys(decoder); err != nil {
				return err
			}
		}
	default:
		return fmt.Errorf("unexpected JSON delimiter %q", delimiter)
	}
	_, err = decoder.Token()
	return err
}

func Init(root string, s State) error {
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
	if err := os.Mkdir(dir, 0700); err != nil && !errors.Is(err, os.ErrExist) {
		return fmt.Errorf("create project directory: %w", err)
	}
	project, err := openMandalaDirectory(root)
	if err != nil {
		return err
	}
	defer project.Close()
	return writeState(project, s, false)
}

func Save(root string, s State) error {
	dir, err := openMandalaDirectory(root)
	if err != nil {
		return err
	}
	defer dir.Close()
	return writeState(dir, s, true)
}

func openMandalaDirectory(root string) (*os.Root, error) {
	project, err := os.OpenRoot(root)
	if err != nil {
		return nil, fmt.Errorf("open project directory: %w", err)
	}
	defer project.Close()
	info, err := project.Lstat(".mandala")
	if errors.Is(err, os.ErrNotExist) {
		return nil, problem("E_NO_PROJECT", "project directory %q is missing", filepath.Join(root, ".mandala"))
	}
	if err != nil {
		return nil, fmt.Errorf("inspect project directory: %w", err)
	}
	if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return nil, problem("E_STATE_INVALID", "%q is not a regular directory", filepath.Join(root, ".mandala"))
	}
	dir, err := project.OpenRoot(".mandala")
	if err != nil {
		return nil, fmt.Errorf("open project state directory: %w", err)
	}
	opened, err := dir.Stat(".")
	if err != nil || !os.SameFile(info, opened) {
		dir.Close()
		return nil, problem("E_STATE_INVALID", "project directory changed while opening")
	}
	return dir, nil
}

func writeState(dir *os.Root, s State, replace bool) error {
	if err := s.Validate(); err != nil {
		return err
	}
	s.Cells = s.SortedCells()
	data, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return fmt.Errorf("encode state: %w", err)
	}
	data = append(data, '\n')
	if len(data) > maxStateBytes {
		return problem("E_STATE_INVALID", "state exceeds 1 MiB")
	}
	info, err := dir.Lstat("state.json")
	if replace {
		if errors.Is(err, os.ErrNotExist) {
			return problem("E_NO_PROJECT", "state.json is missing in %q", dir.Name())
		}
		if err != nil {
			return fmt.Errorf("inspect state before write: %w", err)
		}
		if !info.Mode().IsRegular() {
			return problem("E_STATE_INVALID", "state.json in %q is not a regular file", dir.Name())
		}
	} else if err == nil {
		return problem("E_STATE_INVALID", "state.json already exists in %q", dir.Name())
	} else if !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("inspect state before init: %w", err)
	}
	name := ".state-" + rand.Text()
	f, err := dir.OpenFile(name, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		return fmt.Errorf("create temporary state: %w", err)
	}
	defer dir.Remove(name)
	if _, err := f.Write(data); err != nil {
		f.Close()
		return fmt.Errorf("write temporary state: %w", err)
	}
	if err := f.Sync(); err != nil {
		f.Close()
		return fmt.Errorf("sync temporary state: %w", err)
	}
	if err := f.Close(); err != nil {
		return fmt.Errorf("close temporary state: %w", err)
	}
	if err := dir.Rename(name, "state.json"); err != nil {
		return fmt.Errorf("replace state: %w", err)
	}
	return nil
}
