package secret_test

import (
	"bytes"
	"io/fs"
	"os"
	"sync"
	"testing"

	"github.com/pocikode/ratatoskr/internal/secret"
)

func TestConcurrentFirstStart(t *testing.T) {
	for _, empty := range []bool{false, true} {
		name := "missing"
		if empty {
			name = "empty"
		}
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			testConcurrentOpen(t, empty)
		})
	}
}

func testConcurrentOpen(t *testing.T, empty bool) {
	t.Helper()
	root, err := os.OpenRoot(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer root.Close()
	if empty {
		f, err := root.OpenFile(secret.File, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
		if err != nil {
			t.Fatal(err)
		}
		if err := f.Close(); err != nil {
			t.Fatal(err)
		}
	}
	const n = 32
	boxes := make(chan *secret.Box, n)
	var wg sync.WaitGroup
	for range n {
		wg.Go(func() {
			b, loose, err := secret.Open(root)
			if err != nil {
				t.Error(err)
				return
			}
			if loose {
				t.Error("fresh key has loose permissions")
			}
			boxes <- b
		})
	}
	wg.Wait()
	close(boxes)
	var first *secret.Box
	count := 0
	for b := range boxes {
		count++
		if first == nil {
			first = b
		}
		got, err := b.Open(first.Seal([]byte("value"), []byte("purpose")), []byte("purpose"))
		if err != nil || !bytes.Equal(got, []byte("value")) {
			t.Fatalf("openers disagree on key: %q, %v", got, err)
		}
	}
	if count != n {
		t.Fatalf("only %d openers succeeded", count)
	}
	entries, err := fs.ReadDir(root.FS(), ".")
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 || entries[0].Name() != "secret.key" {
		t.Fatalf("temporary key files left: %v", entries)
	}
}

func TestEmptyKeyPermissionsAndAuthentication(t *testing.T) {
	t.Parallel()
	root, err := os.OpenRoot(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer root.Close()
	f, err := root.OpenFile("secret.key", os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		t.Fatal(err)
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}
	if err := root.Chmod("secret.key", 0o644); err != nil {
		t.Fatal(err)
	}
	b, loose, err := secret.Open(root)
	if err != nil || !loose {
		t.Fatalf("open empty loose key = %v, %v", loose, err)
	}
	info, err := root.Stat("secret.key")
	if err != nil || info.Size() != 32 || info.Mode().Perm() != 0o600 {
		t.Fatalf("key info = %v, %v", info, err)
	}
	blob := b.Seal([]byte("value"), []byte("purpose"))
	if _, err := b.Open(blob, []byte("other")); err == nil {
		t.Fatal("wrong AAD accepted")
	}
	blob[len(blob)-1] ^= 1
	if _, err := b.Open(blob, []byte("purpose")); err == nil {
		t.Fatal("tampering accepted")
	}
	if _, err := b.Open(blob[:3], nil); err == nil {
		t.Fatal("truncated blob accepted")
	}
}
