package queue

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"strings"
	"time"

	"blackbox/shared/types"
	_ "modernc.org/sqlite"
)

const schema = `
CREATE TABLE IF NOT EXISTS pending (
    id        TEXT PRIMARY KEY,
    queued_at INTEGER NOT NULL,
    payload   TEXT NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_queued_at ON pending (queued_at);
`

// Queue is a persistent, ordered event queue backed by SQLite.
type Queue struct {
	db *sql.DB
}

// New opens (or creates) the SQLite database at dbPath, applies the schema,
// and configures WAL mode. Call Close when done.
func New(dbPath string) (*Queue, error) {
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return nil, fmt.Errorf("queue: open db: %w", err)
	}
	db.SetMaxOpenConns(1)

	if _, err := db.Exec("PRAGMA journal_mode=WAL; PRAGMA synchronous=NORMAL;"); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("queue: configure wal: %w", err)
	}
	if _, err := db.Exec(schema); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("queue: apply schema: %w", err)
	}
	return &Queue{db: db}, nil
}

// Push persists an entry to the queue. The entry must have a non-empty ID.
func (q *Queue) Push(entry types.Entry) error {
	if entry.ID == "" {
		return fmt.Errorf("queue: empty entry ID")
	}
	payload, err := json.Marshal(entry)
	if err != nil {
		return fmt.Errorf("queue: marshal entry: %w", err)
	}
	_, err = q.db.Exec(
		`INSERT OR IGNORE INTO pending (id, queued_at, payload) VALUES (?, ?, ?)`,
		entry.ID,
		time.Now().UnixMilli(),
		string(payload),
	)
	if err != nil {
		return fmt.Errorf("queue: insert: %w", err)
	}
	return nil
}

// Flush reads up to limit pending entries ordered oldest-first.
// limit must be positive; a non-positive value is rejected to prevent
// accidental unbounded reads (SQLite treats LIMIT -1 as no limit).
func (q *Queue) Flush(limit int) ([]types.Entry, error) {
	if limit <= 0 {
		return nil, fmt.Errorf("queue: flush limit must be positive, got %d", limit)
	}
	rows, err := q.db.Query(
		`SELECT id, payload FROM pending ORDER BY queued_at ASC LIMIT ?`,
		limit,
	)
	if err != nil {
		return nil, fmt.Errorf("queue: flush query: %w", err)
	}

	// Collect corrupt row IDs during iteration; delete after rows.Close() to
	// avoid competing for the single connection (SetMaxOpenConns(1)).
	var entries []types.Entry
	var corruptIDs []string
	for rows.Next() {
		var id, payload string
		if err := rows.Scan(&id, &payload); err != nil {
			if closeErr := rows.Close(); closeErr != nil {
				log.Printf("queue: rows close: %v", closeErr)
			}
			return nil, fmt.Errorf("queue: scan row: %w", err)
		}
		var entry types.Entry
		if err := json.Unmarshal([]byte(payload), &entry); err != nil {
			log.Printf("queue: corrupt row id=%s, will delete: %v", id, err)
			corruptIDs = append(corruptIDs, id)
			continue
		}
		entries = append(entries, entry)
	}
	rowsErr := rows.Err()
	if err := rows.Close(); err != nil {
		log.Printf("queue: rows close: %v", err)
	}

	// Delete corrupt rows now that the cursor is closed and the connection is free.
	for _, id := range corruptIDs {
		if _, delErr := q.db.Exec(`DELETE FROM pending WHERE id = ?`, id); delErr != nil {
			log.Printf("queue: failed to delete corrupt row id=%s: %v", id, delErr)
		}
	}
	return entries, rowsErr
}

// Delete removes entries by ID. Silently ignores IDs not in the table.
// The SQL string is constructed by repeating '?' placeholders — IDs are
// passed as bound parameters (args...) and never interpolated into the
// query string, so there is no SQL injection risk.
func (q *Queue) Delete(ids []string) error {
	if len(ids) == 0 {
		return nil
	}
	placeholders := strings.Repeat("?,", len(ids))
	placeholders = placeholders[:len(placeholders)-1]
	args := make([]interface{}, len(ids))
	for i, id := range ids {
		args[i] = id
	}
	_, err := q.db.Exec(
		`DELETE FROM pending WHERE id IN (`+placeholders+`)`,
		args...,
	)
	if err != nil {
		return fmt.Errorf("queue: delete: %w", err)
	}
	return nil
}

// SweepStale removes entries older than maxAge. Returns the number of rows deleted.
func (q *Queue) SweepStale(maxAge time.Duration) (int, error) {
	cutoff := time.Now().Add(-maxAge).UnixMilli()
	result, err := q.db.Exec(`DELETE FROM pending WHERE queued_at < ?`, cutoff)
	if err != nil {
		return 0, fmt.Errorf("queue: sweep stale: %w", err)
	}
	n, err := result.RowsAffected()
	if err != nil {
		return 0, fmt.Errorf("queue: sweep stale rows affected: %w", err)
	}
	return int(n), nil
}

// PushAt persists an entry with a specific timestamp. Used in tests only.
func (q *Queue) PushAt(entry types.Entry, at time.Time) error {
	if entry.ID == "" {
		return fmt.Errorf("queue: empty entry ID")
	}
	payload, err := json.Marshal(entry)
	if err != nil {
		return fmt.Errorf("queue: marshal entry: %w", err)
	}
	_, err = q.db.Exec(
		`INSERT OR IGNORE INTO pending (id, queued_at, payload) VALUES (?, ?, ?)`,
		entry.ID,
		at.UnixMilli(),
		string(payload),
	)
	if err != nil {
		return fmt.Errorf("queue: insert at: %w", err)
	}
	return nil
}

// Close releases the database connection.
func (q *Queue) Close() error {
	return q.db.Close()
}
