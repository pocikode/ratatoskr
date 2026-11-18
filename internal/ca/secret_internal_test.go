package ca

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

// TestSealRoundTrip seals and opens a root key, and checks that tampering,
// truncation and another key are all rejected: the CA key at rest must fail
// loudly, never silently.
func TestSealRoundTrip(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()

	root, err := os.OpenRoot(dir)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = root.Close() })

	seal, loose, err := openSealer(root)
	if err != nil {
		t.Fatal(err)
	}

	if loose {
		t.Error("a freshly created secret.key was reported as too open")
	}

	plaintext := []byte("private key material")
	aad := []byte(keyAAD)

	blob := seal.seal(plaintext, aad)
	if bytes.Contains(blob, plaintext) {
		t.Fatal("sealed blob contains the plaintext")
	}

	got, err := seal.open(blob, aad)
	if err != nil {
		t.Fatalf("open: %v", err)
	}

	if !bytes.Equal(got, plaintext) {
		t.Fatalf("open = %q, want %q", got, plaintext)
	}

	// Another associated data value must not open it.
	if _, err := seal.open(blob, []byte("other")); err == nil {
		t.Error("open with the wrong AAD succeeded")
	}

	// A flipped byte must not open it.
	tampered := bytes.Clone(blob)
	tampered[len(tampered)-1] ^= 0xff

	if _, err := seal.open(tampered, aad); err == nil {
		t.Error("open of a tampered blob succeeded")
	}

	// A truncated blob must not open it.
	if _, err := seal.open(blob[:4], aad); err == nil {
		t.Error("open of a truncated blob succeeded")
	}

	// Another secret.key must not open it.
	otherDir := t.TempDir()

	otherRoot, err := os.OpenRoot(otherDir)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = otherRoot.Close() })

	other, _, err := openSealer(otherRoot)
	if err != nil {
		t.Fatal(err)
	}

	if _, err := other.open(blob, aad); err == nil {
		t.Error("open with another secret.key succeeded")
	}
}

func TestSecretKeyWrongSize(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()

	root, err := os.OpenRoot(dir)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = root.Close() })

	if err := os.WriteFile(filepath.Join(dir, secretFile), make([]byte, 16), 0o600); err != nil {
		t.Fatal(err)
	}

	if _, _, err := openSealer(root); err == nil {
		t.Fatal("openSealer accepted a 16-byte secret.key")
	}
}

func TestSealNonceIsRandom(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()

	root, err := os.OpenRoot(dir)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = root.Close() })

	seal, _, err := openSealer(root)
	if err != nil {
		t.Fatal(err)
	}

	plaintext := []byte("same plaintext")

	first := seal.seal(plaintext, nil)
	second := seal.seal(plaintext, nil)

	if bytes.Equal(first, second) {
		t.Fatal("two seals of the same plaintext are identical: the nonce is not random")
	}
}
