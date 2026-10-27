//go:build unix

package mandala

import (
	"os"
	"syscall"
)

// snapshotBatchInput returns the identity of the file at path. Unix stat
// reports device and inode eagerly, so the result stays valid without holding
// a handle. Symlinks are followed.
func snapshotBatchInput(path string) (os.FileInfo, func(), error) {
	info, err := os.Stat(path)
	if err != nil {
		return nil, nil, err
	}
	return info, func() {}, nil
}

// openBatchInputFile opens path read-only without blocking. O_NONBLOCK makes
// opening a FIFO that replaced the file return immediately instead of waiting
// for a writer; the caller then rejects it because the handle is not regular.
func openBatchInputFile(path string) (*os.File, error) {
	return os.OpenFile(path, os.O_RDONLY|syscall.O_NONBLOCK, 0)
}
