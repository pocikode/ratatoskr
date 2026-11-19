package ca

import (
	"os"

	"github.com/pocikode/ratatoskr/internal/secret"
)

// Keep the package-local CA test API while sharing the secret box.
const secretFile = secret.File

type sealer struct{ box *secret.Box }

func openSealer(root *os.Root) (*sealer, bool, error) {
	box, loose, err := secret.Open(root)
	if err != nil {
		return nil, false, err
	}
	return &sealer{box: box}, loose, nil
}
func (s *sealer) seal(plain, aad []byte) []byte         { return s.box.Seal(plain, aad) }
func (s *sealer) open(blob, aad []byte) ([]byte, error) { return s.box.Open(blob, aad) }
