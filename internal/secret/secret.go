// Package secret seals instance secrets with XChaCha20-Poly1305 under secret.key.
package secret

import (
	"crypto/cipher"
	"crypto/rand"
	"errors"
	"fmt"
	"io"
	"os"
	"syscall"

	"golang.org/x/crypto/chacha20poly1305"
)

const (
	// secretFile holds the instance-wide key that seals secrets at rest
	// (docs/03-architecture.md §8). It lives next to instance.db so a backup of
	// the data dir stays usable and a copy without the key does not.
	secretFile = "secret.key"
	// secretSize is the XChaCha20-Poly1305 key size.
	secretSize = chacha20poly1305.KeySize
)

// secretTooOpen reports whether key file permissions grant access beyond the
// owner.
func secretTooOpen(mode os.FileMode) bool { return mode.Perm()&0o077 != 0 }

// Box seals and opens blobs with XChaCha20-Poly1305 under secret.key. A sealed
// blob is nonce||ciphertext, bound to caller-supplied associated data.
type Box struct {
	aead cipher.AEAD
}

// Open loads (or, on first start, creates) secret.key below root and
// returns its AEAD. loose is true when the key file had permissions wider than
// 0600; they were tightened before returning.
func Open(root *os.Root) (s *Box, loose bool, err error) {
	key, loose, err := loadSecretKey(root)
	if err != nil {
		return nil, false, err
	}

	aead, err := chacha20poly1305.NewX(key)
	if err != nil {
		return nil, false, fmt.Errorf("secret: init secret cipher: %w", err)
	}

	return &Box{aead: aead}, loose, nil
}

// loadSecretKey serializes initialization across processes, including readers
// that arrive between exclusive creation and the first write.
func loadSecretKey(root *os.Root) ([]byte, bool, error) {
	if root == nil {
		return nil, false, errors.New("secret: nil root")
	}
	f, err := root.OpenFile(secretFile, os.O_RDWR|os.O_CREATE|os.O_EXCL, 0o600)
	if errors.Is(err, os.ErrExist) {
		f, err = root.OpenFile(secretFile, os.O_RDWR, 0)
	}
	if err != nil {
		return nil, false, fmt.Errorf("secret: open key: %w", err)
	}
	defer f.Close()
	// Closing the descriptor releases the lock, including on an error.
	if err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX); err != nil {
		return nil, false, fmt.Errorf("secret: lock key: %w", err)
	}
	info, err := f.Stat()
	if err != nil {
		return nil, false, err
	}
	loose := secretTooOpen(info.Mode())
	if loose {
		if err := f.Chmod(0o600); err != nil {
			return nil, false, err
		}
	}
	key := make([]byte, secretSize)
	if info.Size() == 0 {
		_, _ = rand.Read(key)
		if _, err := f.Write(key); err != nil {
			return nil, false, fmt.Errorf("secret: write key: %w", err)
		}
		if err := f.Sync(); err != nil {
			return nil, false, fmt.Errorf("secret: sync key: %w", err)
		}
		return key, loose, nil
	}
	if info.Size() != secretSize {
		return nil, false, fmt.Errorf("secret: secret.key: want %d bytes, got %d", secretSize, info.Size())
	}
	if _, err := io.ReadFull(f, key); err != nil {
		return nil, false, fmt.Errorf("secret: read key: %w", err)
	}
	return key, loose, nil
}

// Seal returns nonce||ciphertext for plaintext, bound to aad.
func (s *Box) Seal(plaintext, aad []byte) []byte {
	// A random nonce: a repeat would break XChaCha20-Poly1305. crypto/rand never
	// fails (it panics on an unrecoverable system error).
	nonce := make([]byte, s.aead.NonceSize())
	_, _ = rand.Read(nonce)

	out := s.aead.Seal(nonce, nonce, plaintext, aad)

	return out
}

// Open reverses [Box.Seal]. A wrong key, a truncated blob or another aad all
// fail authentication.
func (s *Box) Open(blob, aad []byte) ([]byte, error) {
	if len(blob) < s.aead.NonceSize()+s.aead.Overhead() {
		return nil, errors.New("secret: sealed secret is too short")
	}

	nonce, ciphertext := blob[:s.aead.NonceSize()], blob[s.aead.NonceSize():]

	plaintext, err := s.aead.Open(nil, nonce, ciphertext, aad)
	if err != nil {
		return nil, fmt.Errorf("secret: open secret: %w", err)
	}

	return plaintext, nil
}
