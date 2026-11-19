//go:build unix

package secret

import (
	"os"
	"syscall"
)

// lockSecretKey holds the initialization lock until f is closed.
func lockSecretKey(f *os.File) error {
	return syscall.Flock(int(f.Fd()), syscall.LOCK_EX)
}
