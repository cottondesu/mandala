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
	path              string
	directoryIdentity os.FileInfo
	identity          os.FileInfo
	originalSize      int64
	appended          []byte
	created           bool
	createdDirectory  bool
}

type gitExcludeDirectory struct {
	root     *os.Root
	identity os.FileInfo
	created  bool
}

func openGitExcludeDirectory(path string, create bool) (result gitExcludeDirectory, returnErr error) {
	directory, name := filepath.Split(filepath.Clean(path))
	directory = filepath.Clean(directory)
	if name != "exclude" || filepath.Base(directory) != "info" {
		return gitExcludeDirectory{}, fmt.Errorf("invalid exclude path %q", path)
	}
	common, err := os.OpenRoot(filepath.Dir(directory))
	if err != nil {
		return gitExcludeDirectory{}, fmt.Errorf("open Git common directory: %w", err)
	}
	defer func() {
		returnErr = errors.Join(returnErr, common.Close())
		if returnErr != nil && result.root != nil {
			returnErr = errors.Join(returnErr, result.root.Close())
			result.root = nil
		}
	}()

	info, err := common.Lstat("info")
	created := false
	if errors.Is(err, os.ErrNotExist) && create {
		if err := common.Mkdir("info", 0700); err != nil {
			return gitExcludeDirectory{}, fmt.Errorf("create exclude directory: %w", err)
		}
		created = true
		info, err = common.Lstat("info")
	}
	if err != nil {
		return gitExcludeDirectory{}, fmt.Errorf("inspect exclude directory: %w", err)
	}
	if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return gitExcludeDirectory{}, fmt.Errorf("exclude parent is not a safe directory")
	}
	openedRoot, err := common.OpenRoot("info")
	if err != nil {
		return gitExcludeDirectory{}, fmt.Errorf("open exclude directory: %w", err)
	}
	opened, err := openedRoot.Stat(".")
	if err != nil || !os.SameFile(info, opened) {
		return gitExcludeDirectory{}, errors.Join(fmt.Errorf("exclude directory changed while opening"), openedRoot.Close())
	}
	result = gitExcludeDirectory{root: openedRoot, identity: opened, created: created}
	return result, nil
}

func appendGitExcludeRule(path string) (change gitExcludeChange, returnErr error) {
	directory, err := openGitExcludeDirectory(path, true)
	if err != nil {
		return gitExcludeChange{
			path:              path,
			directoryIdentity: directory.identity,
			createdDirectory:  directory.created,
		}, err
	}
	root := directory.root
	change = gitExcludeChange{
		path:              path,
		directoryIdentity: directory.identity,
		createdDirectory:  directory.created,
	}
	defer func() { returnErr = errors.Join(returnErr, root.Close()) }()
	const name = "exclude"

	info, err := root.Lstat(name)
	created := errors.Is(err, os.ErrNotExist)
	flags := os.O_RDWR | os.O_APPEND
	if created {
		flags |= os.O_CREATE | os.O_EXCL
	} else if err != nil {
		return change, fmt.Errorf("inspect exclude file: %w", err)
	} else if !info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0 {
		return change, fmt.Errorf("exclude path is not a safe regular file")
	}
	file, err := root.OpenFile(name, flags, 0600)
	if err != nil {
		return change, fmt.Errorf("open exclude file: %w", err)
	}
	defer func() { returnErr = errors.Join(returnErr, file.Close()) }()
	opened, err := file.Stat()
	if err != nil {
		return change, fmt.Errorf("inspect opened exclude file: %w", err)
	}
	current, err := root.Lstat(name)
	if err != nil || !opened.Mode().IsRegular() || !current.Mode().IsRegular() || !os.SameFile(opened, current) {
		return change, fmt.Errorf("exclude file changed while opening")
	}
	change.identity = opened
	change.created = created

	size := opened.Size()
	change.originalSize = size
	tailSize := min(size, int64(len(gitExcludeRule)+3))
	if tailSize > 0 {
		tail := make([]byte, tailSize)
		read, readErr := file.ReadAt(tail, size-tailSize)
		if readErr != nil && !errors.Is(readErr, io.EOF) {
			return change, fmt.Errorf("inspect exclude contents: %w", readErr)
		}
		tail = bytes.TrimSuffix(tail[:read], []byte("\n"))
		tail = bytes.TrimSuffix(tail, []byte("\r"))
		if separator := bytes.LastIndexByte(tail, '\n'); separator >= 0 {
			tail = tail[separator+1:]
		}
		if bytes.Equal(tail, []byte(gitExcludeRule)) {
			return change, nil
		}
	}
	addition := make([]byte, 0, len(gitExcludeRule)+2)
	if size > 0 {
		last := []byte{0}
		if _, err := file.ReadAt(last, size-1); err != nil {
			return change, fmt.Errorf("inspect exclude terminator: %w", err)
		}
		if last[0] != '\n' {
			addition = append(addition, '\n')
		}
	}
	addition = append(addition, gitExcludeRule...)
	addition = append(addition, '\n')
	written, err := file.Write(addition)
	if written > 0 && written <= len(addition) {
		change.appended = append([]byte(nil), addition[:written]...)
	}
	if err != nil {
		return change, fmt.Errorf("write exclude rule: %w", err)
	}
	if written != len(addition) {
		return change, fmt.Errorf("write exclude rule: %w", io.ErrShortWrite)
	}
	if err := file.Sync(); err != nil {
		return change, fmt.Errorf("sync exclude file: %w", err)
	}
	current, err = root.Lstat(name)
	if err != nil || !current.Mode().IsRegular() || !os.SameFile(opened, current) {
		return change, fmt.Errorf("exclude file changed while writing")
	}
	return change, nil
}
