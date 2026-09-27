package mandala

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
)

type gitExcludeChange struct {
	path         string
	identity     os.FileInfo
	originalSize int64
	appended     []byte
	created      bool
}

func appendGitExcludeRule(path string) (change gitExcludeChange, returnErr error) {
	directory, name := filepath.Split(filepath.Clean(path))
	if directory == "" || name == "" || name == "." {
		return gitExcludeChange{}, fmt.Errorf("invalid exclude path %q", path)
	}
	root, err := os.OpenRoot(directory)
	if err != nil {
		return gitExcludeChange{}, fmt.Errorf("open exclude directory: %w", err)
	}
	defer func() { returnErr = errors.Join(returnErr, root.Close()) }()

	info, err := root.Lstat(name)
	created := errors.Is(err, os.ErrNotExist)
	flags := os.O_RDWR | os.O_APPEND
	if created {
		flags |= os.O_CREATE | os.O_EXCL
	} else if err != nil {
		return gitExcludeChange{}, fmt.Errorf("inspect exclude file: %w", err)
	} else if !info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0 {
		return gitExcludeChange{}, fmt.Errorf("exclude path is not a safe regular file")
	}
	file, err := root.OpenFile(name, flags, 0600)
	if err != nil {
		return gitExcludeChange{}, fmt.Errorf("open exclude file: %w", err)
	}
	defer func() { returnErr = errors.Join(returnErr, file.Close()) }()
	opened, err := file.Stat()
	if err != nil {
		return gitExcludeChange{}, fmt.Errorf("inspect opened exclude file: %w", err)
	}
	current, err := root.Lstat(name)
	if err != nil || !opened.Mode().IsRegular() || !current.Mode().IsRegular() || !os.SameFile(opened, current) {
		return gitExcludeChange{}, fmt.Errorf("exclude file changed while opening")
	}

	size := opened.Size()
	tailSize := min(size, int64(len(gitExcludeRule)+3))
	if tailSize > 0 {
		tail := make([]byte, tailSize)
		read, readErr := file.ReadAt(tail, size-tailSize)
		if readErr != nil && !errors.Is(readErr, io.EOF) {
			return gitExcludeChange{}, fmt.Errorf("inspect exclude contents: %w", readErr)
		}
		tail = bytes.TrimSuffix(tail[:read], []byte("\n"))
		tail = bytes.TrimSuffix(tail, []byte("\r"))
		if separator := bytes.LastIndexByte(tail, '\n'); separator >= 0 {
			tail = tail[separator+1:]
		}
		if bytes.Equal(tail, []byte(gitExcludeRule)) {
			return gitExcludeChange{}, nil
		}
	}
	addition := make([]byte, 0, len(gitExcludeRule)+2)
	if size > 0 {
		last := []byte{0}
		if _, err := file.ReadAt(last, size-1); err != nil {
			return gitExcludeChange{}, fmt.Errorf("inspect exclude terminator: %w", err)
		}
		if last[0] != '\n' {
			addition = append(addition, '\n')
		}
	}
	addition = append(addition, gitExcludeRule...)
	addition = append(addition, '\n')
	written, err := file.Write(addition)
	if err != nil {
		return gitExcludeChange{}, fmt.Errorf("write exclude rule: %w", err)
	}
	if written != len(addition) {
		return gitExcludeChange{}, fmt.Errorf("write exclude rule: %w", io.ErrShortWrite)
	}
	if err := file.Sync(); err != nil {
		return gitExcludeChange{}, fmt.Errorf("sync exclude file: %w", err)
	}
	current, err = root.Lstat(name)
	if err != nil || !current.Mode().IsRegular() || !os.SameFile(opened, current) {
		return gitExcludeChange{}, fmt.Errorf("exclude file changed while writing")
	}
	return gitExcludeChange{
		path:         path,
		identity:     opened,
		originalSize: size,
		appended:     addition,
		created:      created,
	}, nil
}

func (change gitExcludeChange) rollback() (returnErr error) {
	if len(change.appended) == 0 {
		return nil
	}
	directory, name := filepath.Split(filepath.Clean(change.path))
	root, err := os.OpenRoot(directory)
	if err != nil {
		return fmt.Errorf("open exclude directory for rollback: %w", err)
	}
	defer func() { returnErr = errors.Join(returnErr, root.Close()) }()
	current, err := root.Lstat(name)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil || !current.Mode().IsRegular() || !os.SameFile(change.identity, current) {
		return fmt.Errorf("exclude file identity changed after update")
	}
	file, err := root.OpenFile(name, os.O_RDWR, 0)
	if err != nil {
		return fmt.Errorf("open exclude file for rollback: %w", err)
	}
	closed := false
	defer func() {
		if !closed {
			returnErr = errors.Join(returnErr, file.Close())
		}
	}()
	opened, err := file.Stat()
	expectedSize := change.originalSize + int64(len(change.appended))
	if err != nil || !opened.Mode().IsRegular() || !os.SameFile(change.identity, opened) || opened.Size() != expectedSize {
		return fmt.Errorf("exclude file content changed after update")
	}
	appended := make([]byte, len(change.appended))
	read, err := file.ReadAt(appended, change.originalSize)
	if err != nil || read != len(appended) || !bytes.Equal(appended, change.appended) {
		return fmt.Errorf("exclude file content changed after update")
	}
	current, err = root.Lstat(name)
	if err != nil || !current.Mode().IsRegular() || !os.SameFile(change.identity, current) {
		return fmt.Errorf("exclude file identity changed before rollback")
	}
	if change.created {
		if err := file.Close(); err != nil {
			return fmt.Errorf("close created exclude before rollback: %w", err)
		}
		closed = true
		current, err = root.Lstat(name)
		if err != nil || !current.Mode().IsRegular() || !os.SameFile(change.identity, current) {
			return fmt.Errorf("created exclude file changed before rollback")
		}
		if err := root.Remove(name); err != nil {
			return fmt.Errorf("remove created exclude during rollback: %w", err)
		}
		return nil
	}
	if err := file.Truncate(change.originalSize); err != nil {
		return fmt.Errorf("truncate exclude during rollback: %w", err)
	}
	if err := file.Sync(); err != nil {
		return fmt.Errorf("sync exclude rollback: %w", err)
	}
	return nil
}
