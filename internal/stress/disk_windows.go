//go:build windows

package stress

import (
	"os"
	"unsafe"

	"golang.org/x/sys/windows"
)

// Windows will happily serve a "disk benchmark" entirely out of the standby
// cache, so the test file is opened with FILE_FLAG_NO_BUFFERING — every
// request then goes to the device — plus FILE_FLAG_WRITE_THROUGH so writes are
// not parked in the drive's volatile cache either.
//
// The price is that every transfer has to be sector aligned in length, in file
// offset and in the *address* of the buffer; see alignedBuf. If the flags are
// refused for any reason the caller falls back to a buffered file rather than
// giving up on the test.
const (
	fileFlagWriteThrough = 0x80000000
	fileFlagNoBuffering  = 0x20000000

	// 4 KiB covers both 512e and native 4K devices, and is a multiple of the
	// 512-byte sector the rest report.
	sectorAlign = 4096
)

func openDirect(path string) (*os.File, bool, error) {
	p, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return nil, false, err
	}

	h, err := windows.CreateFile(p,
		windows.GENERIC_READ|windows.GENERIC_WRITE,
		0, // exclusive: nothing else should be reading the file mid-test
		nil,
		windows.CREATE_ALWAYS,
		windows.FILE_ATTRIBUTE_NORMAL|fileFlagNoBuffering|fileFlagWriteThrough,
		0)
	if err == nil {
		return os.NewFile(uintptr(h), path), true, nil
	}

	f, err := os.OpenFile(path, os.O_RDWR|os.O_CREATE|os.O_TRUNC, 0o644)
	return f, false, err
}

// alignedBuf returns a buffer of n bytes whose first byte sits on a sector
// boundary, which unbuffered I/O requires. Go never moves heap objects, so an
// address checked once stays aligned for the buffer's whole life.
func alignedBuf(n int) []byte {
	raw := make([]byte, n+sectorAlign)
	off := (sectorAlign - int(uintptr(unsafe.Pointer(&raw[0]))%sectorAlign)) % sectorAlign
	return raw[off : off+n]
}
