//go:build windows

package mandala

import (
	"os"
	"strings"
	"syscall"
)

// snapshotBatchInput returns the identity of the file at path, read from an
// open handle. os.Stat on Windows loads the file ID lazily from the path the
// first time os.SameFile needs it, so a file replaced at the same path
// afterwards would be compared with itself. File.Stat on a handle reads the ID
// eagerly. The handle stays open until release, which keeps the original file
// and its ID alive, so the ID cannot be reused by a replacement.
//
// The handle requests no access, which still permits metadata queries, and
// shares read, write, and delete, so it does not by itself conflict with other
// handles' access or sharing modes. Symlinks are followed, and FILE_FLAG_BACKUP_SEMANTICS lets directories open
// so they are rejected as non-regular instead of failing with access denied.
func snapshotBatchInput(path string) (os.FileInfo, func(), error) {
	name, err := syscall.UTF16PtrFromString(extendedBatchPath(path))
	if err != nil {
		return nil, nil, &os.PathError{Op: "open", Path: path, Err: err}
	}
	h, err := syscall.CreateFile(name, 0,
		syscall.FILE_SHARE_READ|syscall.FILE_SHARE_WRITE|syscall.FILE_SHARE_DELETE,
		nil, syscall.OPEN_EXISTING, syscall.FILE_FLAG_BACKUP_SEMANTICS, 0)
	if err != nil {
		return nil, nil, &os.PathError{Op: "open", Path: path, Err: err}
	}
	f := os.NewFile(uintptr(h), path)
	info, err := f.Stat()
	if err != nil {
		f.Close()
		return nil, nil, err
	}
	return info, func() { f.Close() }, nil
}

// openBatchInputFile opens path read-only. Windows has no FIFO to block on;
// the opened handle is compared with the snapshot by the caller.
func openBatchInputFile(path string) (*os.File, error) {
	return os.Open(path)
}

// extendedBatchPath prefixes long absolute paths so CreateFile accepts them
// the way os.Open does. Short and relative paths are returned unchanged.
func extendedBatchPath(path string) string {
	if len(path) < 248 || strings.HasPrefix(path, `\\?\`) || strings.HasPrefix(path, `\\.\`) {
		return path
	}
	if strings.HasPrefix(path, `\\`) {
		return `\\?\UNC\` + path[2:]
	}
	if len(path) >= 3 && path[1] == ':' && (path[2] == '\\' || path[2] == '/') {
		return `\\?\` + strings.ReplaceAll(path, "/", `\`)
	}
	return path
}
