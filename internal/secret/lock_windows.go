package secret

import (
	"os"

	"golang.org/x/sys/windows"
)

// lockSecretKey holds the initialization lock until f is closed. Locking past
// EOF also serializes initialization of empty legacy key files.
func lockSecretKey(f *os.File) error {
	return windows.LockFileEx(windows.Handle(f.Fd()), windows.LOCKFILE_EXCLUSIVE_LOCK, 0, 1, 0, &windows.Overlapped{})
}
