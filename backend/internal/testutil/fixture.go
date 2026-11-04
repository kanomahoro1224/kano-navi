package testutil

import (
	"context"
	"database/sql"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	_ "modernc.org/sqlite"
)

// NewSnapshot creates an empty snapshot with the real schema, read from the
// store's bootstrap.sql, so tests meet the same columns, defaults and
// constraints as a running service. It is read from disk because the store
// package imports this one in its tests. The schema's stream_identity index
// needs that function, which the store package registers when it is linked.
func NewSnapshot(t testing.TB) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "snapshot.sqlite")
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatalf("open fixture: %v", err)
	}
	defer db.Close()
	_, file, _, _ := runtime.Caller(0)
	schema, err := os.ReadFile(filepath.Join(filepath.Dir(file), "..", "store", "sqlite", "bootstrap.sql"))
	if err != nil {
		t.Fatalf("read schema: %v", err)
	}
	if _, err = db.Exec(string(schema)); err != nil {
		t.Fatalf("create fixture: %v", err)
	}
	return path
}

// Exec writes to a snapshot that a running handler may be reading. The busy
// timeout lets the write wait out a reader's shared lock; it is set on the
// connection rather than in a file: URI because t.TempDir keeps '#' and '%'
// from subtest names, which a URI would misread.
func Exec(t testing.TB, path, query string, args ...any) {
	t.Helper()
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatalf("open fixture for write: %v", err)
	}
	defer db.Close()
	ctx := context.Background()
	conn, err := db.Conn(ctx)
	if err != nil {
		t.Fatalf("open fixture for write: %v", err)
	}
	defer conn.Close()
	if _, err := conn.ExecContext(ctx, `PRAGMA busy_timeout = 5000`); err != nil {
		t.Fatalf("set fixture busy timeout: %v", err)
	}
	if _, err := conn.ExecContext(ctx, query, args...); err != nil {
		t.Fatalf("write fixture: %v", err)
	}
}
