package mandala

import (
	"fmt"
	"os"
)

// readBatchInputFile reads a bounded batch input from the regular file at
// path. display is the user-supplied path used in diagnostics.
//
// The guarantee is bounded and platform specific. snapshotBatchInput captures
// the identity of the file at path before the data open, and the opened handle
// is compared with it, so a different file placed at the same path in between
// is rejected with E_IO. Both must be regular files. On Windows the snapshot
// comes from an open handle that is held until the comparison, so the original
// file ID cannot be reused. On Unix it is the stat device and inode with no
// handle held, so inode reuse is not prevented; the nonblocking open keeps a
// FIFO replacement from blocking. Other platforms keep only the baseline
// os.Stat then os.Open comparison, with neither guarantee.
//
// Content is read only through the opened handle, so a replacement after the
// open cannot change what is read. This does not make the content immutable:
// another process that can already write the file may still modify it while
// it is being read.
func readBatchInputFile(path, display string) ([]byte, error) {
	want, release, err := snapshotBatchInput(path)
	if err != nil {
		return nil, fmt.Errorf("inspect batch input: %w", err)
	}
	defer release()
	if !want.Mode().IsRegular() {
		return nil, problem("E_IO", "batch input %q is not a regular file", display)
	}
	f, err := batchOpenInputFile(path)
	if err != nil {
		return nil, fmt.Errorf("open batch input: %w", err)
	}
	defer f.Close()
	got, err := f.Stat()
	if err != nil {
		return nil, fmt.Errorf("inspect batch input: %w", err)
	}
	if !got.Mode().IsRegular() {
		return nil, problem("E_IO", "batch input %q is not a regular file", display)
	}
	if !os.SameFile(want, got) {
		return nil, problem("E_IO", "batch input %q changed while opening", display)
	}
	return readBoundedBatchInput(f)
}
