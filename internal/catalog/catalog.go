package catalog

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"

	_ "modernc.org/sqlite" // registers the "sqlite" database/sql driver
)

// DB wraps a *sql.DB connected to the insights catalog SQLite database.
type DB struct {
	*sql.DB
}

// Open opens (creating if necessary) the SQLite catalog database at path,
// ensuring its parent directory exists - path is expected to live on the
// same PVC as the stored SBOMs, e.g. "/data/catalog.sqlite" - applies the
// schema, and returns a ready-to-use *DB.
func Open(ctx context.Context, path string) (*DB, error) {
	if dir := filepath.Dir(path); dir != "" && dir != "." {
		if err := os.MkdirAll(dir, 0o750); err != nil {
			return nil, fmt.Errorf("catalog: unable to create directory for %s: %w", path, err)
		}
	}

	sqlDB, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, fmt.Errorf("catalog: unable to open %s: %w", path, err)
	}

	// SQLite only supports a single writer at a time regardless of
	// connection pooling. Keeping this at 1 means contention surfaces as a
	// busy_timeout wait (see pragma below) rather than spurious "database is
	// locked" errors from Go's connection pool opening a second connection.
	sqlDB.SetMaxOpenConns(1)

	pragmas := []string{
		"PRAGMA foreign_keys = ON;",
		"PRAGMA journal_mode = WAL;",
		"PRAGMA busy_timeout = 5000;",
	}
	for _, p := range pragmas {
		if _, err := sqlDB.ExecContext(ctx, p); err != nil {
			_ = sqlDB.Close()
			return nil, fmt.Errorf("catalog: unable to set pragma %q: %w", p, err)
		}
	}

	if err := migrate(ctx, sqlDB); err != nil {
		_ = sqlDB.Close()
		return nil, err
	}

	return &DB{DB: sqlDB}, nil
}

// Close closes the underlying database connection.
func (db *DB) Close() error {
	return db.DB.Close()
}
