package store

import (
	"database/sql"
	"fmt"
	"os"
	"time"

	_ "modernc.org/sqlite"
)

// migrations run in order. user_version records how many have run.
var migrations = []string{`
CREATE TABLE request (
	provider TEXT NOT NULL, key TEXT NOT NULL, account TEXT NOT NULL, session TEXT NOT NULL,
	agent TEXT NOT NULL, turn TEXT NOT NULL, model TEXT NOT NULL,
	input INTEGER NOT NULL, cached_input INTEGER NOT NULL, cache_write INTEGER NOT NULL,
	output INTEGER NOT NULL, reasoning INTEGER NOT NULL, cost_micros INTEGER,
	compaction INTEGER NOT NULL, block TEXT NOT NULL, rex_session TEXT NOT NULL,
	source TEXT NOT NULL, at INTEGER NOT NULL,
	PRIMARY KEY (provider, key));
CREATE INDEX request_at ON request (at);
CREATE INDEX request_session ON request (provider, session, at);
CREATE TABLE compaction (
	id INTEGER PRIMARY KEY, provider TEXT NOT NULL, key TEXT NOT NULL, account TEXT NOT NULL,
	session TEXT NOT NULL, turn TEXT NOT NULL, trigger TEXT NOT NULL,
	pre_tokens INTEGER, post_tokens INTEGER, duration_ms INTEGER,
	request_key TEXT NOT NULL, source TEXT NOT NULL, at INTEGER NOT NULL);
CREATE UNIQUE INDEX compaction_key ON compaction (provider, key) WHERE key != '';
CREATE INDEX compaction_session ON compaction (provider, session, at);
CREATE TABLE latest (
	provider TEXT NOT NULL, account TEXT NOT NULL, limit_id TEXT NOT NULL, minutes INTEGER NOT NULL,
	percent REAL NOT NULL, resets_at INTEGER NOT NULL, observed_at INTEGER NOT NULL,
	PRIMARY KEY (provider, account, limit_id, minutes));
CREATE TABLE credits (
	provider TEXT NOT NULL, account TEXT NOT NULL, limit_id TEXT NOT NULL,
	has_credits INTEGER NOT NULL, unlimited INTEGER NOT NULL, balance TEXT NOT NULL,
	observed_at INTEGER NOT NULL,
	PRIMARY KEY (provider, account, limit_id));
CREATE TABLE binding (
	id INTEGER PRIMARY KEY, host TEXT NOT NULL, block TEXT NOT NULL, rex_session TEXT NOT NULL,
	provider TEXT NOT NULL, session TEXT NOT NULL, source TEXT NOT NULL,
	start INTEGER NOT NULL, "end" INTEGER);
CREATE INDEX binding_session ON binding (provider, session, start);
CREATE TABLE cursor (
	path TEXT PRIMARY KEY, inode INTEGER NOT NULL, "offset" INTEGER NOT NULL, size INTEGER NOT NULL,
	mod_time INTEGER NOT NULL, head_hash TEXT NOT NULL, tail_hash TEXT NOT NULL, state TEXT NOT NULL);
`, `
CREATE INDEX compaction_request ON compaction (provider, request_key) WHERE request_key != '';
`}

// Open opens or creates the store and applies migrations.
// The file is created owner-only; SQLite gives its WAL files the same mode.
func Open(path string) (*Store, error) {
	f, err := os.OpenFile(path, os.O_RDWR|os.O_CREATE, 0o600)
	if err != nil {
		return nil, err
	}
	f.Close()
	db, err := sql.Open("sqlite", "file:"+path+"?_pragma=journal_mode(WAL)&_pragma=busy_timeout(5000)&_pragma=synchronous(NORMAL)")
	if err != nil {
		return nil, err
	}
	// One connection serializes writers; readers are cheap at this size.
	db.SetMaxOpenConns(1)
	if err := migrate(db); err != nil {
		db.Close()
		return nil, err
	}
	rdb, err := sql.Open("sqlite", "file:"+path+"?mode=ro&_pragma=busy_timeout(5000)")
	if err != nil {
		db.Close()
		return nil, err
	}
	rdb.SetMaxOpenConns(readers)
	return &Store{db: db, rdb: rdb}, nil
}

func migrate(db *sql.DB) error {
	var version int
	if err := db.QueryRow(`PRAGMA user_version`).Scan(&version); err != nil {
		return err
	}
	for i := version; i < len(migrations); i++ {
		tx, err := db.Begin()
		if err != nil {
			return err
		}
		if _, err := tx.Exec(migrations[i]); err != nil {
			tx.Rollback()
			return fmt.Errorf("migration %d: %w", i+1, err)
		}
		if _, err := tx.Exec(fmt.Sprintf(`PRAGMA user_version = %d`, i+1)); err != nil {
			tx.Rollback()
			return err
		}
		if err := tx.Commit(); err != nil {
			return err
		}
	}
	return nil
}

func nanos(t time.Time) int64 { return t.UnixNano() }

func fromNanos(n int64) time.Time { return time.Unix(0, n).UTC() }

func boolInt(b bool) int {
	if b {
		return 1
	}
	return 0
}
