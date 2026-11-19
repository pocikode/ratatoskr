package replay_test

import (
	"database/sql"
	"os"
	"path/filepath"
	"testing"

	"github.com/pocikode/ratatoskr/internal/replay"
	"github.com/pocikode/ratatoskr/internal/store"
)

func TestMigrationPreservesEntries(t *testing.T) {
	t.Parallel()
	root, err := os.OpenRoot(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = root.Close() })
	legacy, err := sql.Open("sqlite", filepath.Join(root.Name(), "project.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = legacy.Close() })
	for _, file := range []string{"0001_init.sql", "0002_scope_sort_order.sql"} {
		migration, err := os.ReadFile(filepath.Join("..", "store", "migrations", "project", file))
		if err != nil {
			t.Fatal(err)
		}
		if _, err := legacy.ExecContext(t.Context(), string(migration)); err != nil {
			t.Fatal(err)
		}
	}
	_, err = legacy.ExecContext(t.Context(), `CREATE TABLE schema_version(version INTEGER PRIMARY KEY, name TEXT NOT NULL, applied_at INTEGER NOT NULL) STRICT;
 INSERT INTO schema_version VALUES(1, '0001_init.sql', 0),(2, '0002_scope_sort_order.sql', 0);
 INSERT INTO replay_collections(id,name) VALUES(5,'Existing');
 INSERT INTO replay_sessions(id,collection_id,kind,name,created_at) VALUES(6,5,'http','Existing',1234);
 INSERT INTO requests(id,source,host,port,tls,method,path,raw,len,created_at) VALUES(8,'replay','example.test',80,0,'GET','/',X'00ff',2,1234);
 INSERT INTO requests(id,source,host,port,tls,method,path,raw,len,created_at,truncated) VALUES(9,'proxy','example.test',80,0,'GET','/',X'00ff',2,1234,1);
 INSERT INTO replay_entries(id,session_id,request_id,created_at) VALUES(7,6,8,1234);`)
	if err != nil {
		t.Fatal(err)
	}
	if err := legacy.Close(); err != nil {
		t.Fatal(err)
	}
	db, err := store.OpenProject(t.Context(), root)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	for _, id := range []int64{8, 9} {
		var truncated bool
		if err := db.Reader().QueryRowContext(t.Context(), `SELECT request_truncated FROM requests WHERE id = ?`, id).Scan(&truncated); err != nil {
			t.Fatal(err)
		}
		if truncated != (id == 9) {
			t.Fatalf("request %d: backfilled truncation=%v", id, truncated)
		}
	}
	svc := replay.New(func() *store.DB { return db })
	page, err := svc.Entries(t.Context(), 6, 100, nil)
	if err != nil || page.Total != 1 || page.Items[0].ID != 7 || page.Items[0].RequestID != 8 || page.Items[0].Error != nil || page.Items[0].CreatedAt.UnixMilli() != 1234 {
		t.Fatalf("migrated entries = %+v %v", page, err)
	}
	entry, err := svc.AddEntry(t.Context(), 6, 8, "failure")
	if err != nil || entry.ID <= 7 {
		t.Fatalf("next entry = %+v %v", entry, err)
	}
	if err := svc.DeleteCollection(t.Context(), 5); err != nil {
		t.Fatal(err)
	}
	var count int
	if err := db.Reader().QueryRowContext(t.Context(), `SELECT count(*) FROM replay_entries`).Scan(&count); err != nil || count != 0 {
		t.Fatal(count, err)
	}
	if err := db.Reader().QueryRowContext(t.Context(), `SELECT count(*) FROM requests`).Scan(&count); err != nil || count != 2 {
		t.Fatal(count, err)
	}
}
