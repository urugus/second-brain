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
		migrateV3,
		migrateV4,
		migrateV5,
		migrateV6,
		migrateV7,
		migrateV8,
		migrateV9,
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

func migrateV3(tx *sql.Tx) error {
	statements := []string{
		`ALTER TABLE notes ADD COLUMN consolidated_at TEXT`,
		`CREATE INDEX IF NOT EXISTS idx_notes_unconsolidated ON notes(consolidated_at) WHERE consolidated_at IS NULL`,
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

func migrateV4(tx *sql.Tx) error {
	statements := []string{
		`ALTER TABLE notes ADD COLUMN strength REAL NOT NULL DEFAULT 0.30 CHECK (strength >= 0 AND strength <= 1)`,
		`ALTER TABLE notes ADD COLUMN decay_rate REAL NOT NULL DEFAULT 0.015 CHECK (decay_rate > 0 AND decay_rate <= 1)`,
		`ALTER TABLE notes ADD COLUMN salience REAL NOT NULL DEFAULT 0.50 CHECK (salience >= 0 AND salience <= 1)`,
		`ALTER TABLE notes ADD COLUMN recall_count INTEGER NOT NULL DEFAULT 0 CHECK (recall_count >= 0)`,
		`ALTER TABLE notes ADD COLUMN last_recalled_at TEXT`,
	}
	for _, stmt := range statements {
		if _, err := tx.Exec(stmt); err != nil {
			return fmt.Errorf("exec %q: %w", stmt[:40], err)
		}
	}
	return nil
}

func migrateV5(tx *sql.Tx) error {
	statements := []string{
		`CREATE TABLE IF NOT EXISTS memory_edges (
			id               INTEGER PRIMARY KEY AUTOINCREMENT,
			from_note_id     INTEGER NOT NULL REFERENCES notes(id) ON DELETE CASCADE,
			to_note_id       INTEGER NOT NULL REFERENCES notes(id) ON DELETE CASCADE,
			weight           REAL NOT NULL DEFAULT 0.10 CHECK (weight > 0 AND weight <= 1),
			evidence         TEXT NOT NULL DEFAULT '',
			reinforced_count INTEGER NOT NULL DEFAULT 1 CHECK (reinforced_count >= 1),
			created_at       TEXT NOT NULL,
			updated_at       TEXT NOT NULL,
			CHECK (from_note_id <> to_note_id),
			UNIQUE (from_note_id, to_note_id)
		)`,
		`CREATE INDEX IF NOT EXISTS idx_memory_edges_from ON memory_edges(from_note_id)`,
		`CREATE INDEX IF NOT EXISTS idx_memory_edges_to ON memory_edges(to_note_id)`,
	}
	for _, stmt := range statements {
		if _, err := tx.Exec(stmt); err != nil {
			return fmt.Errorf("exec %q: %w", stmt[:40], err)
		}
	}
	return nil
}

func migrateV7(tx *sql.Tx) error {
	statements := []string{
		`CREATE TABLE IF NOT EXISTS kb_note_map (
			kb_path    TEXT NOT NULL,
			note_id    INTEGER NOT NULL REFERENCES notes(id) ON DELETE CASCADE,
			created_at TEXT NOT NULL,
			UNIQUE(kb_path, note_id)
		)`,
		`CREATE INDEX IF NOT EXISTS idx_kb_note_map_path ON kb_note_map(kb_path)`,
		`CREATE INDEX IF NOT EXISTS idx_kb_note_map_note ON kb_note_map(note_id)`,
	}
	for _, stmt := range statements {
		if _, err := tx.Exec(stmt); err != nil {
			return fmt.Errorf("exec %q: %w", stmt[:40], err)
		}
	}
	return nil
}

func migrateV6(tx *sql.Tx) error {
	statements := []string{
		`CREATE TABLE IF NOT EXISTS prediction_error_log (
			id              INTEGER PRIMARY KEY AUTOINCREMENT,
			source          TEXT NOT NULL,
			metric          TEXT NOT NULL,
			predicted_value REAL NOT NULL,
			actual_value    REAL NOT NULL,
			error_value     REAL NOT NULL,
			priority_delta  INTEGER NOT NULL DEFAULT 0,
			created_at      TEXT NOT NULL
		)`,
		`CREATE INDEX IF NOT EXISTS idx_prediction_error_source_time ON prediction_error_log(source, created_at DESC)`,
	}
	for _, stmt := range statements {
		if _, err := tx.Exec(stmt); err != nil {
			return fmt.Errorf("exec %q: %w", stmt[:40], err)
		}
	}
	return nil
}

func migrateV8(tx *sql.Tx) error {
	_, err := tx.Exec(`CREATE INDEX IF NOT EXISTS idx_notes_created_at ON notes(created_at DESC)`)
	if err != nil {
		return fmt.Errorf("exec %q: %w", "CREATE INDEX IF NOT EXISTS idx_notes_created_at ON notes(created_at DESC)", err)
	}
	return nil
}

func migrateV9(tx *sql.Tx) error {
	statements := []string{
		`CREATE TABLE IF NOT EXISTS entities (
			id              INTEGER PRIMARY KEY AUTOINCREMENT,
			kind            TEXT NOT NULL
			                CHECK (kind IN ('person', 'concept', 'org', 'project', 'unknown')),
			canonical_name  TEXT NOT NULL,
			normalized_name TEXT NOT NULL,
			strength        REAL NOT NULL DEFAULT 0.20 CHECK (strength >= 0 AND strength <= 1),
			salience        REAL NOT NULL DEFAULT 0.50 CHECK (salience >= 0 AND salience <= 1),
			status          TEXT NOT NULL DEFAULT 'candidate'
			                CHECK (status IN ('candidate', 'confirmed', 'rejected', 'archived')),
			created_at      TEXT NOT NULL,
			updated_at      TEXT NOT NULL,
			UNIQUE(kind, normalized_name)
		)`,
		`CREATE INDEX IF NOT EXISTS idx_entities_status ON entities(status)`,
		`CREATE TABLE IF NOT EXISTS entity_aliases (
			id               INTEGER PRIMARY KEY AUTOINCREMENT,
			entity_id        INTEGER NOT NULL REFERENCES entities(id) ON DELETE CASCADE,
			alias            TEXT NOT NULL,
			normalized_alias TEXT NOT NULL,
			confidence       REAL NOT NULL DEFAULT 0.50 CHECK (confidence >= 0 AND confidence <= 1),
			source_note_id   INTEGER REFERENCES notes(id) ON DELETE SET NULL,
			created_at       TEXT NOT NULL,
			UNIQUE(entity_id, normalized_alias)
		)`,
		`CREATE INDEX IF NOT EXISTS idx_entity_aliases_norm ON entity_aliases(normalized_alias)`,
		`CREATE TABLE IF NOT EXISTS note_entities (
			note_id     INTEGER NOT NULL REFERENCES notes(id) ON DELETE CASCADE,
			entity_id   INTEGER NOT NULL REFERENCES entities(id) ON DELETE CASCADE,
			confidence  REAL NOT NULL DEFAULT 0.50 CHECK (confidence >= 0 AND confidence <= 1),
			evidence    TEXT NOT NULL DEFAULT '',
			source      TEXT NOT NULL DEFAULT '',
			created_at  TEXT NOT NULL,
			updated_at  TEXT NOT NULL,
			UNIQUE(note_id, entity_id)
		)`,
		`CREATE INDEX IF NOT EXISTS idx_note_entities_note ON note_entities(note_id)`,
		`CREATE INDEX IF NOT EXISTS idx_note_entities_entity ON note_entities(entity_id)`,
		`CREATE TABLE IF NOT EXISTS entity_edges (
			id               INTEGER PRIMARY KEY AUTOINCREMENT,
			from_entity_id   INTEGER NOT NULL REFERENCES entities(id) ON DELETE CASCADE,
			to_entity_id     INTEGER NOT NULL REFERENCES entities(id) ON DELETE CASCADE,
			relation_type    TEXT NOT NULL DEFAULT 'associated',
			weight           REAL NOT NULL DEFAULT 0.10 CHECK (weight > 0 AND weight <= 1),
			evidence         TEXT NOT NULL DEFAULT '',
			source_note_id   INTEGER REFERENCES notes(id) ON DELETE SET NULL,
			reinforced_count INTEGER NOT NULL DEFAULT 1 CHECK (reinforced_count >= 1),
			created_at       TEXT NOT NULL,
			updated_at       TEXT NOT NULL,
			CHECK (from_entity_id <> to_entity_id),
			UNIQUE(from_entity_id, to_entity_id, relation_type)
		)`,
		`CREATE INDEX IF NOT EXISTS idx_entity_edges_from ON entity_edges(from_entity_id)`,
		`CREATE INDEX IF NOT EXISTS idx_entity_edges_to ON entity_edges(to_entity_id)`,
	}
	for _, stmt := range statements {
		if _, err := tx.Exec(stmt); err != nil {
			return fmt.Errorf("exec %q: %w", stmt[:40], err)
		}
	}
	return nil
}
