package store

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"

	_ "modernc.org/sqlite"
)

type Store struct {
	db *sql.DB
}

func Open(dbPath string) (*Store, error) {
	if err := os.MkdirAll(filepath.Dir(dbPath), 0o755); err != nil {
		return nil, fmt.Errorf("create db directory: %w", err)
	}

	db, err := sql.Open("sqlite", dbPath+"?_pragma=foreign_keys(1)&_pragma=journal_mode(WAL)")
	if err != nil {
		return nil, fmt.Errorf("open database: %w", err)
	}

	if err := db.Ping(); err != nil {
		db.Close()
		return nil, fmt.Errorf("ping database: %w", err)
	}

	s := &Store{db: db}
	if err := s.migrate(); err != nil {
		db.Close()
		return nil, fmt.Errorf("migrate: %w", err)
	}

	return s, nil
}

func (s *Store) Close() error {
	return s.db.Close()
}

func (s *Store) migrate() error {
	if _, err := s.db.Exec(`CREATE TABLE IF NOT EXISTS schema_version (version INTEGER NOT NULL)`); err != nil {
		return fmt.Errorf("create schema_version: %w", err)
	}

	var version int
	row := s.db.QueryRow(`SELECT COALESCE(MAX(version), 0) FROM schema_version`)
	if err := row.Scan(&version); err != nil {
		return fmt.Errorf("get schema version: %w", err)
	}

	migrations := []func(tx *sql.Tx) error{
		migrateV1,
		migrateV2,
	}

	for i := version; i < len(migrations); i++ {
		tx, err := s.db.Begin()
		if err != nil {
			return fmt.Errorf("begin migration %d: %w", i+1, err)
		}

		if err := migrations[i](tx); err != nil {
			tx.Rollback()
			return fmt.Errorf("migration %d: %w", i+1, err)
		}

		if _, err := tx.Exec(`DELETE FROM schema_version`); err != nil {
			tx.Rollback()
			return fmt.Errorf("update schema version: %w", err)
		}
		if _, err := tx.Exec(`INSERT INTO schema_version (version) VALUES (?)`, i+1); err != nil {
			tx.Rollback()
			return fmt.Errorf("insert schema version: %w", err)
		}

		if err := tx.Commit(); err != nil {
			return fmt.Errorf("commit migration %d: %w", i+1, err)
		}
	}

	return nil
}

func migrateV1(tx *sql.Tx) error {
	statements := []string{
		`CREATE TABLE IF NOT EXISTS sessions (
			id         INTEGER PRIMARY KEY AUTOINCREMENT,
			title      TEXT NOT NULL,
			goal       TEXT NOT NULL DEFAULT '',
			status     TEXT NOT NULL DEFAULT 'active'
			           CHECK (status IN ('active', 'completed', 'abandoned')),
			started_at TEXT NOT NULL,
			ended_at   TEXT,
			summary    TEXT NOT NULL DEFAULT '',
			created_at TEXT NOT NULL,
			updated_at TEXT NOT NULL
		)`,
		`CREATE TABLE IF NOT EXISTS tasks (
			id          INTEGER PRIMARY KEY AUTOINCREMENT,
			session_id  INTEGER REFERENCES sessions(id),
			title       TEXT NOT NULL,
			description TEXT NOT NULL DEFAULT '',
			status      TEXT NOT NULL DEFAULT 'todo'
			            CHECK (status IN ('todo', 'in_progress', 'done', 'cancelled')),
			priority    INTEGER NOT NULL DEFAULT 0,
			created_at  TEXT NOT NULL,
			updated_at  TEXT NOT NULL
		)`,
		`CREATE INDEX IF NOT EXISTS idx_tasks_session ON tasks(session_id)`,
		`CREATE INDEX IF NOT EXISTS idx_tasks_status ON tasks(status)`,
		`CREATE TABLE IF NOT EXISTS notes (
			id         INTEGER PRIMARY KEY AUTOINCREMENT,
			session_id INTEGER REFERENCES sessions(id),
			content    TEXT NOT NULL,
			tags       TEXT NOT NULL DEFAULT '',
			source     TEXT NOT NULL DEFAULT '',
			created_at TEXT NOT NULL,
			updated_at TEXT NOT NULL
		)`,
		`CREATE INDEX IF NOT EXISTS idx_notes_session ON notes(session_id)`,
		`CREATE TABLE IF NOT EXISTS events (
			id         INTEGER PRIMARY KEY AUTOINCREMENT,
			session_id INTEGER REFERENCES sessions(id),
			type       TEXT NOT NULL,
			payload    TEXT NOT NULL,
			created_at TEXT NOT NULL
		)`,
		`CREATE INDEX IF NOT EXISTS idx_events_session ON events(session_id)`,
		`CREATE INDEX IF NOT EXISTS idx_events_type ON events(type)`,
		`CREATE TABLE IF NOT EXISTS consolidation_log (
			id               INTEGER PRIMARY KEY AUTOINCREMENT,
			session_id       INTEGER REFERENCES sessions(id),
			agent            TEXT NOT NULL DEFAULT '',
			input_summary    TEXT NOT NULL DEFAULT '',
			output_summary   TEXT NOT NULL DEFAULT '',
			kb_files_updated TEXT NOT NULL DEFAULT '',
			status           TEXT NOT NULL DEFAULT 'pending'
			                 CHECK (status IN ('pending', 'running', 'completed', 'failed')),
			created_at       TEXT NOT NULL
		)`,
	}

	for _, stmt := range statements {
		if _, err := tx.Exec(stmt); err != nil {
			return fmt.Errorf("exec %q: %w", stmt[:40], err)
		}
	}
	return nil
}

func migrateV2(tx *sql.Tx) error {
	_, err := tx.Exec(`CREATE TABLE IF NOT EXISTS sync_log (
		id               INTEGER PRIMARY KEY AUTOINCREMENT,
		agent            TEXT NOT NULL DEFAULT '',
		prompt_used      TEXT NOT NULL DEFAULT '',
		output_summary   TEXT NOT NULL DEFAULT '',
		notes_added      INTEGER NOT NULL DEFAULT 0,
		tasks_added      INTEGER NOT NULL DEFAULT 0,
		kb_files_updated TEXT NOT NULL DEFAULT '',
		duration_ms      INTEGER NOT NULL DEFAULT 0,
		status           TEXT NOT NULL DEFAULT 'pending'
		                 CHECK (status IN ('pending', 'running', 'completed', 'failed')),
		error_message    TEXT NOT NULL DEFAULT '',
		created_at       TEXT NOT NULL
	)`)
	return err
}
