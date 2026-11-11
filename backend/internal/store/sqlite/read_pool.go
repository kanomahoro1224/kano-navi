package sqlite

import (
	"context"
	"database/sql"
	"fmt"
)

const publicReadConnections = 4

// openReadPool enables WAL on the writable runtime database and opens a
// separate read-only pool for public projections. The writer remains the
// Store's single connection, so mutations keep their existing single-flight
// behavior while independent public reads can run concurrently.
func (s *Store) openReadPool(ctx context.Context) error {
	if s == nil || s.db == nil || s.databasePath == "" {
		return ErrUnavailable
	}
	if _, err := s.db.ExecContext(ctx, "PRAGMA journal_mode=WAL"); err != nil {
		return fmt.Errorf("%w: WAL could not be enabled", ErrUnavailable)
	}
	reader, err := sql.Open("sqlite", "file:"+s.databasePath+"?mode=ro&_pragma=busy_timeout(5000)")
	if err != nil {
		return fmt.Errorf("%w: read pool could not be opened", ErrUnavailable)
	}
	reader.SetMaxOpenConns(publicReadConnections)
	reader.SetMaxIdleConns(publicReadConnections)
	if err = reader.PingContext(ctx); err != nil {
		_ = reader.Close()
		return fmt.Errorf("%w: read pool could not be read", ErrUnavailable)
	}
	s.readDB = reader
	return nil
}

// publicReader returns a shallow Store view whose reads use the read-only
// pool. Clearing readDB on the view prevents nested public methods from
// cloning it again. Writes always use the original Store and its one writer
// connection.
func (s *Store) publicReader() *Store {
	if s == nil || s.readDB == nil || s.db == s.readDB {
		return s
	}
	view := *s
	view.db = s.readDB
	view.readDB = nil
	view.writable = false
	view.workflowWritable = false
	return &view
}
