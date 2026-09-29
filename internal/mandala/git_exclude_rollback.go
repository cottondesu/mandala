package mandala

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
)

func (change gitExcludeChange) rollback() (returnErr error) {
	if change.directoryIdentity == nil && change.identity == nil {
		return nil
	}
	directory, err := openGitExcludeDirectory(change.path, false)
	if err != nil {
		if change.createdDirectory && errors.Is(err, os.ErrNotExist) {
			return nil
		}
		return fmt.Errorf("open exclude directory for rollback: %w", err)
	}
	if change.directoryIdentity != nil && !os.SameFile(change.directoryIdentity, directory.identity) {
		return errors.Join(fmt.Errorf("exclude directory identity changed after update"), directory.root.Close())
	}
	if change.directoryIdentity == nil {
		change.directoryIdentity = directory.identity
	}
	root := directory.root
	rootClosed := false
	defer func() {
		if !rootClosed {
			returnErr = errors.Join(returnErr, root.Close())
		}
	}()
	closeRoot := func() error {
		if rootClosed {
			return nil
		}
		rootClosed = true
		return root.Close()
	}
	if change.identity == nil {
		if err := closeRoot(); err != nil {
			return fmt.Errorf("close exclude directory before rollback: %w", err)
		}
		return change.rollbackCreatedDirectory()
	}
	const name = "exclude"
	current, err := root.Lstat(name)
	if errors.Is(err, os.ErrNotExist) {
		if err := closeRoot(); err != nil {
			return fmt.Errorf("close exclude directory before rollback: %w", err)
		}
		return change.rollbackCreatedDirectory()
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
		if err := closeRoot(); err != nil {
			return fmt.Errorf("close exclude directory before rollback: %w", err)
		}
		return change.rollbackCreatedDirectory()
	}
	if err := file.Truncate(change.originalSize); err != nil {
		return fmt.Errorf("truncate exclude during rollback: %w", err)
	}
	if err := file.Sync(); err != nil {
		return fmt.Errorf("sync exclude rollback: %w", err)
	}
	return nil
}

func (change gitExcludeChange) rollbackCreatedDirectory() (returnErr error) {
	if !change.createdDirectory {
		return nil
	}
	directory, err := openGitExcludeDirectory(change.path, false)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("open created exclude directory for rollback: %w", err)
	}
	if !os.SameFile(change.directoryIdentity, directory.identity) {
		return errors.Join(fmt.Errorf("created exclude directory identity changed before rollback"), directory.root.Close())
	}
	opened, err := directory.root.Open(".")
	if err != nil {
		return errors.Join(fmt.Errorf("inspect created exclude directory: %w", err), directory.root.Close())
	}
	entries, readErr := opened.ReadDir(1)
	closeFileErr := opened.Close()
	closeRootErr := directory.root.Close()
	if readErr != nil && !errors.Is(readErr, io.EOF) {
		return errors.Join(fmt.Errorf("inspect created exclude directory: %w", readErr), closeFileErr, closeRootErr)
	}
	if closeFileErr != nil || closeRootErr != nil {
		return errors.Join(closeFileErr, closeRootErr)
	}
	if len(entries) != 0 {
		return fmt.Errorf("created exclude directory contains concurrent data")
	}

	commonPath := filepath.Dir(filepath.Dir(filepath.Clean(change.path)))
	common, err := os.OpenRoot(commonPath)
	if err != nil {
		return fmt.Errorf("open Git common directory for rollback: %w", err)
	}
	defer func() { returnErr = errors.Join(returnErr, common.Close()) }()
	current, err := common.Lstat("info")
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil || !current.IsDir() || current.Mode()&os.ModeSymlink != 0 || !os.SameFile(change.directoryIdentity, current) {
		return fmt.Errorf("created exclude directory identity changed before removal")
	}
	if err := common.Remove("info"); err != nil {
		return fmt.Errorf("remove created exclude directory during rollback: %w", err)
	}
	return nil
}
