package storage

import (
	"fmt"
)

type migration struct {
	version int
	sql     string
}

var migrations = []migration{
	{
		version: 1,
		sql: `
CREATE TABLE IF NOT EXISTS schema_version (
    version INTEGER PRIMARY KEY,
    applied_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);
CREATE TABLE IF NOT EXISTS jobs (
    id TEXT PRIMARY KEY,
    type TEXT NOT NULL,
    status TEXT NOT NULL DEFAULT 'pending',
    model_id TEXT NOT NULL,
    request TEXT NOT NULL,
    result TEXT,
    error TEXT,
    progress REAL DEFAULT 0,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    started_at TIMESTAMP,
    completed_at TIMESTAMP,
    priority INTEGER DEFAULT 0
);
CREATE TABLE IF NOT EXISTS outputs (
    id TEXT PRIMARY KEY,
    job_id TEXT NOT NULL,
    type TEXT NOT NULL,
    path TEXT NOT NULL,
    mime_type TEXT,
    size_bytes INTEGER DEFAULT 0,
    duration_ms REAL,
    sample_rate INTEGER,
    channels INTEGER,
    metadata TEXT,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    FOREIGN KEY (job_id) REFERENCES jobs(id)
);`,
	},
	{
		version: 2,
		sql: `
ALTER TABLE jobs ADD COLUMN worker_id TEXT DEFAULT '';
ALTER TABLE jobs ADD COLUMN claimed_at TIMESTAMP;
ALTER TABLE jobs ADD COLUMN lease_until TIMESTAMP;
ALTER TABLE jobs ADD COLUMN attempt INTEGER DEFAULT 0;
ALTER TABLE jobs ADD COLUMN backend_name TEXT DEFAULT '';
ALTER TABLE jobs ADD COLUMN backend_version TEXT DEFAULT '';
ALTER TABLE jobs ADD COLUMN trace_id TEXT DEFAULT '';
ALTER TABLE jobs ADD COLUMN output_ref TEXT DEFAULT '';
ALTER TABLE jobs ADD COLUMN error_code TEXT DEFAULT '';
ALTER TABLE jobs ADD COLUMN error_message TEXT DEFAULT '';
ALTER TABLE jobs ADD COLUMN duration_ms INTEGER;`,
	},
}

func (db *DB) RunMigrations() error {
	if _, err := db.db.Exec(`CREATE TABLE IF NOT EXISTS schema_version (
		version INTEGER PRIMARY KEY,
		applied_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
	)`); err != nil {
		return fmt.Errorf("create schema_version table: %w", err)
	}

	var currentVersion int
	err := db.db.QueryRow(`SELECT COALESCE(MAX(version), 0) FROM schema_version`).Scan(&currentVersion)
	if err != nil {
		return fmt.Errorf("read current schema version: %w", err)
	}

	for _, m := range migrations {
		if m.version > currentVersion {
			tx, err := db.db.Begin()
			if err != nil {
				return fmt.Errorf("begin transaction for migration %d: %w", m.version, err)
			}

			if _, err := tx.Exec(m.sql); err != nil {
				tx.Rollback()
				return fmt.Errorf("execute migration %d: %w", m.version, err)
			}

			if _, err := tx.Exec(`INSERT INTO schema_version (version) VALUES (?)`, m.version); err != nil {
				tx.Rollback()
				return fmt.Errorf("record migration %d version: %w", m.version, err)
			}

			if err := tx.Commit(); err != nil {
				return fmt.Errorf("commit migration %d: %w", m.version, err)
			}
		}
	}

	return nil
}
