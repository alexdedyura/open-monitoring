//go:build !windows

package stress

import "os"

// Off Windows the app is only expected to compile, not to ship, so the test
// file is an ordinary buffered one and the alignment requirement disappears
// with it.
func openDirect(path string) (*os.File, bool, error) {
	f, err := os.OpenFile(path, os.O_RDWR|os.O_CREATE|os.O_TRUNC, 0o644)
	return f, false, err
}

func alignedBuf(n int) []byte { return make([]byte, n) }
