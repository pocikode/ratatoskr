package project

import (
	"context"
	"database/sql"
	"fmt"
	"io"
	"io/fs"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"strings"

	_ "modernc.org/sqlite" // pure-Go SQLite driver ("sqlite"), also imported by internal/store
)

// copyData copies project src into the (empty) directory of dst: project.db as
// a consistent snapshot (VACUUM INTO), then every other file. The snapshot is
// taken over its own read-only connection because the store's read pool is
// query_only and its writer runs inside transactions, where VACUUM is illegal.
func (s *Service) copyData(ctx context.Context, src, dst int64) error {
	abs, err := filepath.Abs(s.root.Name())
	if err != nil {
		return err
	}

	srcDB := filepath.Join(abs, filepath.FromSlash(dirOf(src)), "project.db")
	if _, err := s.root.Stat(path.Join(dirOf(src), "project.db")); err == nil {
		dstDB := filepath.Join(abs, filepath.FromSlash(dirOf(dst)), "project.db")
		if err := vacuumInto(ctx, srcDB, dstDB); err != nil {
			return fmt.Errorf("snapshot database: %w", err)
		}
	}

	return s.copyFiles(dirOf(src), dirOf(dst))
}

func vacuumInto(ctx context.Context, from, to string) error {
	u := url.URL{Scheme: "file", Path: filepath.ToSlash(from), RawQuery: "mode=ro"}

	db, err := sql.Open("sqlite", u.String())
	if err != nil {
		return err
	}
	defer db.Close()

	db.SetMaxOpenConns(1)

	_, err = db.ExecContext(ctx, `VACUUM INTO ?`, to)

	return err
}

// copyFiles copies everything below srcDir except the SQLite files.
func (s *Service) copyFiles(srcDir, dstDir string) error {
	return fs.WalkDir(s.root.FS(), srcDir, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}

		rel := strings.TrimPrefix(strings.TrimPrefix(p, srcDir), "/")
		if rel == "" || isSQLiteFile(rel) {
			return nil
		}

		out := path.Join(dstDir, rel)

		switch {
		case d.IsDir():
			return s.root.MkdirAll(out, 0o700)
		case d.Type().IsRegular():
			return s.copyFile(p, out)
		default:
			return nil // symlinks and devices are not project data
		}
	})
}

func isSQLiteFile(rel string) bool {
	switch rel {
	case "project.db", "project.db-wal", "project.db-shm":
		return true
	}

	return false
}

func (s *Service) copyFile(from, to string) (err error) {
	in, err := s.root.Open(from)
	if err != nil {
		return err
	}
	defer in.Close()

	out, err := s.root.OpenFile(to, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return err
	}

	defer func() {
		if cerr := out.Close(); err == nil {
			err = cerr
		}
	}()

	_, err = io.Copy(out, in)

	return err
}
