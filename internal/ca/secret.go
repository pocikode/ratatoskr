package ca

import (
	"crypto/cipher"
	"crypto/rand"
	"errors"
	"fmt"
	"io"
	"os"

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

// sealer seals and opens blobs with XChaCha20-Poly1305 under secret.key. A sealed
// blob is nonce||ciphertext, with keyAAD as associated data.
type sealer struct {
	aead cipher.AEAD
}

// openSealer loads (or, on first start, creates) secret.key below root and
// returns its AEAD. loose is true when the key file had permissions wider than
// 0600; they were tightened before returning.
func openSealer(root *os.Root) (s *sealer, loose bool, err error) {
	key, loose, err := loadSecretKey(root)
	if err != nil {
		return nil, false, err
	}

	aead, err := chacha20poly1305.NewX(key)
	if err != nil {
		return nil, false, fmt.Errorf("ca: init secret cipher: %w", err)
	}

	return &sealer{aead: aead}, loose, nil
}

// loadSecretKey reads the 32-byte key file, creating it with a fresh random key
// when it is absent or empty.
func loadSecretKey(root *os.Root) (key []byte, loose bool, err error) {
	f, err := root.OpenFile(secretFile, os.O_RDWR|os.O_CREATE, 0o600)
	if err != nil {
		return nil, false, fmt.Errorf("ca: open %s: %w", secretFile, err)
	}
	defer f.Close()

	info, err := f.Stat()
	if err != nil {
		return nil, false, fmt.Errorf("ca: stat %s: %w", secretFile, err)
	}

	if secretTooOpen(info.Mode()) {
		// The key is useless without its narrow permissions: tighten instead of
		// refusing to start, but the caller reports it.
		if err := root.Chmod(secretFile, 0o600); err != nil {
			return nil, false, fmt.Errorf("ca: restrict %s to 0600: %w", secretFile, err)
		}

		loose = true
	}

	key = make([]byte, secretSize)

	if info.Size() == 0 {
		if _, err := rand.Read(key); err != nil {
			return nil, false, fmt.Errorf("ca: generate %s: %w", secretFile, err)
		}

		if _, err := f.Write(key); err != nil {
			return nil, false, fmt.Errorf("ca: write %s: %w", secretFile, err)
		}

		// The key must survive a crash before anything sealed with it is stored.
		if err := f.Sync(); err != nil {
			return nil, false, fmt.Errorf("ca: sync %s: %w", secretFile, err)
		}

		return key, loose, nil
	}

	if info.Size() != secretSize {
		return nil, false, fmt.Errorf("ca: %s: want %d bytes, got %d", secretFile, secretSize, info.Size())
	}

	if _, err := io.ReadFull(f, key); err != nil {
		return nil, false, fmt.Errorf("ca: read %s: %w", secretFile, err)
	}

	return key, loose, nil
}

// seal returns nonce||ciphertext for plaintext, bound to aad.
func (s *sealer) seal(plaintext, aad []byte) []byte {
	// A random nonce: a repeat would break XChaCha20-Poly1305. crypto/rand never
	// fails (it panics on an unrecoverable system error).
	nonce := make([]byte, s.aead.NonceSize())
	_, _ = rand.Read(nonce)

	out := s.aead.Seal(nonce, nonce, plaintext, aad)

	return out
}

// open reverses [sealer.seal]. A wrong key, a truncated blob or another aad all
// fail authentication.
func (s *sealer) open(blob, aad []byte) ([]byte, error) {
	if len(blob) < s.aead.NonceSize()+s.aead.Overhead() {
		return nil, errors.New("ca: sealed secret is too short")
	}

	nonce, ciphertext := blob[:s.aead.NonceSize()], blob[s.aead.NonceSize():]

	plaintext, err := s.aead.Open(nil, nonce, ciphertext, aad)
	if err != nil {
		return nil, fmt.Errorf("ca: open secret: %w", err)
	}

	return plaintext, nil
}
