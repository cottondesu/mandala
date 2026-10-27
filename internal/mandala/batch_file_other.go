//go:build !unix && !windows

package mandala

import "os"

// Other platforms keep the baseline behavior: os.Stat before os.Open, then
// the identity comparison in readBatchInputFile. No handle snapshot or
// nonblocking open is implemented here, so the replacement and FIFO
// guarantees of the Unix and Windows files do not apply.
func snapshotBatchInput(path string) (os.FileInfo, func(), error) {
	info, err := os.Stat(path)
	if err != nil {
		return nil, nil, err
	}
	return info, func() {}, nil
}

func openBatchInputFile(path string) (*os.File, error) {
	return os.Open(path)
}
