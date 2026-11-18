package ca

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/pocikode/ratatoskr/internal/store"
)

// errNoCA means instance.db has no ca row yet: first start.
var errNoCA = errors.New("ca: no stored root CA")

// loadRow returns the stored root certificate and its sealed key.
func loadRow(ctx context.Context, db *store.DB) (certPEM, keyEnc []byte, err error) {
	err = db.View(ctx, func(tx *sql.Tx) error {
		err := tx.QueryRowContext(ctx, `SELECT cert_pem, key_pem_enc FROM ca WHERE id = 1`).
			Scan(&certPEM, &keyEnc)
		if errors.Is(err, sql.ErrNoRows) {
			return errNoCA
		}

		return err
	})
	if err != nil {
		return nil, nil, fmt.Errorf("ca: read root CA: %w", err)
	}

	return certPEM, keyEnc, nil
}

// saveRow writes the single ca row (id = 1).
func saveRow(ctx context.Context, db *store.DB, certPEM, keyEnc []byte) error {
	err := db.Write(ctx, func(tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, `
			INSERT INTO ca (id, cert_pem, key_pem_enc) VALUES (1, ?, ?)
			ON CONFLICT (id) DO UPDATE SET
				cert_pem    = excluded.cert_pem,
				key_pem_enc = excluded.key_pem_enc`,
			string(certPEM), keyEnc)

		return err
	})
	if err != nil {
		return fmt.Errorf("ca: write root CA: %w", err)
	}

	return nil
}
