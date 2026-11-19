package ca

import (
	"bytes"
	"os"
	"testing"
	"time"

	"golang.org/x/crypto/chacha20poly1305"

	"github.com/pocikode/ratatoskr/internal/store"
)

func TestOpenCAWithLegacyCiphertext(t *testing.T) {
	t.Parallel()
	root, err := os.OpenRoot(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer root.Close()
	inst, err := store.OpenInstance(t.Context(), root)
	if err != nil {
		t.Fatal(err)
	}
	defer inst.Close()
	key := bytes.Repeat([]byte{42}, 32)
	f, err := root.OpenFile("secret.key", os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.Write(key); err != nil {
		t.Fatal(err)
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}
	oldRoot, err := generateRoot(time.Now())
	if err != nil {
		t.Fatal(err)
	}
	der, err := marshalKeyDER(oldRoot.key)
	if err != nil {
		t.Fatal(err)
	}
	// Reproduce the pre-extraction nonce || ciphertext and unchanged CA AAD.
	oldCipher, err := chacha20poly1305.NewX(key)
	if err != nil {
		t.Fatal(err)
	}
	nonce := bytes.Repeat([]byte{7}, chacha20poly1305.NonceSizeX)
	enc := oldCipher.Seal(nonce, nonce, der, []byte("ratatoskr/ca/root-key/v1"))
	if err := saveRow(t.Context(), inst, oldRoot.pem, enc); err != nil {
		t.Fatal(err)
	}
	svc, err := Open(t.Context(), Deps{Root: root, Instance: inst})
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(svc.CertPEM(), oldRoot.pem) {
		t.Fatal("legacy CA was replaced")
	}
	_, exported, err := svc.Export()
	want, marshalErr := marshalKeyPEM(oldRoot.key)
	if err != nil || marshalErr != nil || !bytes.Equal(exported, want) {
		t.Fatalf("legacy private key changed: %v, %v", err, marshalErr)
	}
}
