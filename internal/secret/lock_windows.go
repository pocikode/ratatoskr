package secret

import (
	"os"
	"syscall"
	"unsafe"
)

var lockFileEx = syscall.NewLazyDLL("kernel32.dll").NewProc("LockFileEx")

// lockSecretKey holds the initialization lock until f is closed. Locking past
// EOF also serializes initialization of empty legacy key files.
func lockSecretKey(f *os.File) error {
	const exclusiveLock = 0x00000002 // LOCKFILE_EXCLUSIVE_LOCK
	var offset syscall.Overlapped
	ok, _, err := lockFileEx.Call(f.Fd(), exclusiveLock, 0, 1, 0, uintptr(unsafe.Pointer(&offset))) //nolint:gosec // T-037: LockFileEx takes *OVERLAPPED and has no public standard-library wrapper.
	if ok == 0 {
		return err
	}
	return nil
}
