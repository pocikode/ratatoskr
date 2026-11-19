package history

import (
	"context"
	"os"
	"testing"
	"testing/synctest"
	"time"

	"github.com/pocikode/ratatoskr/internal/store"
)

func TestWrittenReceiptAfterInsertBatch(t *testing.T) {
	t.Parallel()

	root, err := os.OpenRoot(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer root.Close()
	db, err := store.OpenProject(t.Context(), root)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	synctest.Test(t, func(t *testing.T) {
		c := New(Deps{DB: func() *store.DB { return db }})
		ctx, cancel := context.WithCancel(t.Context())
		defer cancel()
		id := c.NewPending(ctx, Entry{Req: Side{Raw: []byte("GET / HTTP/1.1\r\n\r\n")}})
		done := make(chan error, 1)
		go func() { done <- c.Written(ctx, id) }()
		synctest.Wait()

		// Deliver the receipt after the batch containing its insert commits.
		insert, receipt := <-c.in, <-c.in
		c.in <- insert
		go c.Run(ctx)
		time.Sleep(FlushInterval)
		synctest.Wait()
		start := time.Now()
		c.in <- receipt
		if err := <-done; err != nil || time.Since(start) != 0 {
			t.Fatalf("Written returned after %s: %v", time.Since(start), err)
		}
	})
}
