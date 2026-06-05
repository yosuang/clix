package store

import (
	"database/sql"
	"os"
	"path/filepath"

	_ "github.com/mattn/go-sqlite3"
	"github.com/yosuang/clix/internal/protocol"
)

type SQLite struct {
	db *sql.DB
}

func Open(path string) (*SQLite, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, protocol.NewError(protocol.StorageError, err.Error())
	}
	db, err := sql.Open("sqlite3", path)
	if err != nil {
		return nil, protocol.NewError(protocol.StorageError, err.Error())
	}
	db.SetMaxOpenConns(1)
	store := &SQLite{db: db}
	if err := store.migrate(); err != nil {
		_ = db.Close()
		return nil, protocol.NewError(protocol.StorageError, err.Error())
	}
	return store, nil
}

func (s *SQLite) Close() error {
	return s.db.Close()
}

func (s *SQLite) migrate() error {
	if _, err := s.db.Exec(`PRAGMA foreign_keys = ON`); err != nil {
		return err
	}
	_, err := s.db.Exec(`
CREATE TABLE IF NOT EXISTS runs (
  id TEXT PRIMARY KEY,
  tool_name TEXT NOT NULL,
  effect TEXT NOT NULL,
  tool_fingerprint TEXT NOT NULL,
  tool_source_path TEXT NOT NULL,
  input_json BLOB NOT NULL,
  status TEXT NOT NULL,
  requested_at TEXT NOT NULL,
  approved_at TEXT,
  started_at TEXT,
  finished_at TEXT,
  exit_code INTEGER,
  error_code TEXT,
  error_message TEXT
);

CREATE INDEX IF NOT EXISTS idx_runs_status_requested_at
ON runs(status, requested_at);
`)
	return err
}
