// Package store persists sandbox state in SQLite and publishes change events.
package store

import (
	"database/sql"
	"errors"
	"fmt"
	"sync"
	"time"

	_ "modernc.org/sqlite"
)

// DefaultRunID is the sandbox used when a request is not tied to another run.
const DefaultRunID = "default"

// schemaVersion is bumped whenever the schema changes incompatibly.
const schemaVersion = 1

// ErrNotFound is returned when a record does not exist.
var ErrNotFound = errors.New("not found")

// ErrDuplicate is returned when a reference was already used in a run.
var ErrDuplicate = errors.New("duplicate reference")

// ErrInvalid wraps errors caused by invalid input.
var ErrInvalid = errors.New("invalid request")

// Event notifies listeners (the dashboard) that a run's data changed.
type Event struct {
	Type  string `json:"type"`
	RunID string `json:"run_id"`
}

// Store is the SQLite-backed sandbox state.
type Store struct {
	db  *sql.DB
	now func() time.Time

	mu          sync.Mutex
	subscribers map[chan Event]struct{}
}

// Open opens (or creates) the database. An empty path keeps everything in memory.
func Open(path string) (*Store, error) {
	dsn := path
	if dsn == "" {
		dsn = ":memory:"
	}
	db, err := sql.Open("sqlite", dsn+"?_pragma=foreign_keys(1)&_pragma=busy_timeout(5000)")
	if err != nil {
		return nil, fmt.Errorf("open database: %w", err)
	}
	// One connection serialises writes and keeps an in-memory database alive.
	db.SetMaxOpenConns(1)
	db.SetConnMaxLifetime(0)
	db.SetConnMaxIdleTime(0)

	s := &Store{db: db, now: time.Now, subscribers: map[chan Event]struct{}{}}
	if err := s.migrate(); err != nil {
		_ = db.Close()
		return nil, err
	}
	return s, nil
}

// Close releases the database.
func (s *Store) Close() error { return s.db.Close() }

// SetClock replaces the store's clock (used by tests).
func (s *Store) SetClock(now func() time.Time) { s.now = now }

// Now returns the store's current time.
func (s *Store) Now() time.Time { return s.now() }

func (s *Store) timestamp() string { return s.now().UTC().Format("2006-01-02 15:04:05") }

func (s *Store) millis() int64 { return s.now().UnixMilli() }

func (s *Store) migrate() error {
	var version int
	if err := s.db.QueryRow("PRAGMA user_version").Scan(&version); err != nil {
		return fmt.Errorf("read schema version: %w", err)
	}
	if version == schemaVersion {
		return nil
	}
	var tables int
	_ = s.db.QueryRow("SELECT count(*) FROM sqlite_master WHERE type = 'table'").Scan(&tables)
	if tables > 0 {
		return fmt.Errorf("database was created by an incompatible version (schema %d, want %d): delete it or point ORCHARD_SANDBOX_DB at a new file", version, schemaVersion)
	}
	if _, err := s.db.Exec(schema); err != nil {
		return fmt.Errorf("create schema: %w", err)
	}
	_, err := s.db.Exec(fmt.Sprintf("PRAGMA user_version = %d", schemaVersion))
	return err
}

const schema = `
CREATE TABLE runs (
	run_id TEXT PRIMARY KEY,
	name TEXT NOT NULL,
	client_key TEXT NOT NULL UNIQUE,
	secret_key TEXT NOT NULL,
	service_id TEXT NOT NULL,
	settings TEXT NOT NULL,
	opening_balances TEXT NOT NULL,
	created_at TEXT NOT NULL
);

CREATE TABLE balances (
	run_id TEXT NOT NULL REFERENCES runs ON DELETE CASCADE,
	account TEXT NOT NULL,
	amount INTEGER NOT NULL,
	PRIMARY KEY (run_id, account)
);

CREATE TABLE ledger (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	run_id TEXT NOT NULL REFERENCES runs ON DELETE CASCADE,
	account TEXT NOT NULL,
	direction TEXT NOT NULL,
	amount INTEGER NOT NULL,
	balance_after INTEGER NOT NULL,
	exttrid TEXT NOT NULL,
	note TEXT NOT NULL,
	created_at TEXT NOT NULL
);
CREATE INDEX ledger_run ON ledger (run_id, id);

CREATE TABLE refs (
	run_id TEXT NOT NULL REFERENCES runs ON DELETE CASCADE,
	kind TEXT NOT NULL,
	ref TEXT NOT NULL,
	PRIMARY KEY (run_id, kind, ref)
);

CREATE TABLE transactions (
	run_id TEXT NOT NULL REFERENCES runs ON DELETE CASCADE,
	exttrid TEXT NOT NULL,
	trans_id TEXT NOT NULL,
	trans_type TEXT NOT NULL,
	channel TEXT NOT NULL,
	customer_number TEXT NOT NULL,
	nw TEXT NOT NULL,
	amount INTEGER NOT NULL,
	reference TEXT NOT NULL,
	callback_url TEXT NOT NULL,
	status TEXT NOT NULL,
	trans_status TEXT NOT NULL,
	message TEXT NOT NULL,
	scenario TEXT NOT NULL,
	settle_at INTEGER,
	meta TEXT NOT NULL,
	created_at TEXT NOT NULL,
	updated_at TEXT NOT NULL,
	PRIMARY KEY (run_id, exttrid)
);
CREATE INDEX transactions_due ON transactions (status, settle_at);

CREATE TABLE requests (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	run_id TEXT NOT NULL REFERENCES runs ON DELETE CASCADE,
	method TEXT NOT NULL,
	path TEXT NOT NULL,
	operation TEXT NOT NULL,
	ref TEXT NOT NULL,
	headers TEXT NOT NULL,
	body TEXT NOT NULL,
	status INTEGER NOT NULL,
	response_body TEXT NOT NULL,
	resp_code TEXT NOT NULL,
	duration_ms INTEGER NOT NULL,
	created_at TEXT NOT NULL
);
CREATE INDEX requests_ref ON requests (run_id, operation, ref);

CREATE TABLE callbacks (
	id TEXT PRIMARY KEY,
	run_id TEXT NOT NULL REFERENCES runs ON DELETE CASCADE,
	exttrid TEXT NOT NULL,
	url TEXT NOT NULL,
	payload TEXT NOT NULL,
	state TEXT NOT NULL,
	attempts INTEGER NOT NULL DEFAULT 0,
	max_attempts INTEGER NOT NULL,
	timeout_ms INTEGER NOT NULL,
	next_attempt_at INTEGER,
	last_status INTEGER NOT NULL DEFAULT 0,
	last_error TEXT NOT NULL DEFAULT '',
	created_at TEXT NOT NULL,
	delivered_at TEXT NOT NULL DEFAULT ''
);
CREATE INDEX callbacks_due ON callbacks (state, next_attempt_at);

CREATE TABLE callback_attempts (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	callback_id TEXT NOT NULL REFERENCES callbacks ON DELETE CASCADE,
	http_status INTEGER NOT NULL,
	response_body TEXT NOT NULL,
	error TEXT NOT NULL,
	duration_ms INTEGER NOT NULL,
	attempted_at TEXT NOT NULL
);

CREATE TABLE cards (
	run_id TEXT NOT NULL REFERENCES runs ON DELETE CASCADE,
	id_num TEXT NOT NULL,
	name TEXT NOT NULL,
	gender TEXT NOT NULL,
	verified TEXT NOT NULL,
	card_valid_start TEXT NOT NULL,
	card_valid_end TEXT NOT NULL,
	PRIMARY KEY (run_id, id_num)
);

CREATE TABLE accounts (
	run_id TEXT NOT NULL REFERENCES runs ON DELETE CASCADE,
	bank_code TEXT NOT NULL,
	account_number TEXT NOT NULL,
	account_name TEXT NOT NULL,
	resp_code TEXT NOT NULL,
	PRIMARY KEY (run_id, bank_code, account_number)
);

CREATE TABLE sms (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	run_id TEXT NOT NULL REFERENCES runs ON DELETE CASCADE,
	unique_id TEXT NOT NULL,
	sender_id TEXT NOT NULL,
	recipient TEXT NOT NULL,
	body TEXT NOT NULL,
	msg_type TEXT NOT NULL,
	pages INTEGER NOT NULL,
	created_at TEXT NOT NULL
);

CREATE TABLE subscriptions (
	run_id TEXT NOT NULL REFERENCES runs ON DELETE CASCADE,
	uniq_ref_id TEXT NOT NULL,
	service_id TEXT NOT NULL,
	customer_number TEXT NOT NULL,
	nw TEXT NOT NULL,
	amount INTEGER NOT NULL,
	cycle TEXT NOT NULL,
	start_date TEXT NOT NULL,
	end_date TEXT NOT NULL,
	reference TEXT NOT NULL,
	return_url TEXT NOT NULL,
	resumable TEXT NOT NULL,
	cycle_skip TEXT NOT NULL,
	apply_penalty TEXT NOT NULL,
	status TEXT NOT NULL,
	otp TEXT NOT NULL,
	subscribed_at TEXT NOT NULL,
	activated_at TEXT NOT NULL DEFAULT '',
	cancelled_at TEXT NOT NULL DEFAULT '',
	PRIMARY KEY (run_id, uniq_ref_id)
);

CREATE TABLE failure_rules (
	id TEXT PRIMARY KEY,
	run_id TEXT NOT NULL REFERENCES runs ON DELETE CASCADE,
	operation TEXT NOT NULL,
	exttrid TEXT NOT NULL,
	attempt INTEGER NOT NULL,
	action TEXT NOT NULL,
	http_status INTEGER NOT NULL,
	resp_code TEXT NOT NULL,
	delay_ms INTEGER NOT NULL,
	hold INTEGER NOT NULL,
	response_body TEXT NOT NULL,
	created_at TEXT NOT NULL
);
`

// Subscribe registers a listener for change events.
func (s *Store) Subscribe() (<-chan Event, func()) {
	ch := make(chan Event, 64)
	s.mu.Lock()
	s.subscribers[ch] = struct{}{}
	s.mu.Unlock()
	return ch, func() {
		s.mu.Lock()
		delete(s.subscribers, ch)
		s.mu.Unlock()
	}
}

// Publish notifies listeners without blocking; slow listeners miss events.
func (s *Store) Publish(runID string, types ...string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, t := range types {
		for ch := range s.subscribers {
			select {
			case ch <- Event{Type: t, RunID: runID}:
			default:
			}
		}
	}
}

// Reference namespaces. Orchard requires each identifier to be unique per service.
const (
	RefExttrid      = "exttrid"
	RefSMS          = "unique_id"
	RefSubscription = "uniq_ref_id"
)

type execer interface {
	Exec(query string, args ...any) (sql.Result, error)
}

// Reserve claims a reference within a run. It returns ErrDuplicate if the reference was used before.
func (s *Store) Reserve(runID, kind, ref string) error {
	return reserve(s.db, runID, kind, ref)
}

func reserve(q execer, runID, kind, ref string) error {
	res, err := q.Exec("INSERT OR IGNORE INTO refs (run_id, kind, ref) VALUES (?, ?, ?)", runID, kind, ref)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrDuplicate
	}
	return nil
}

func notFound(err error) error {
	if errors.Is(err, sql.ErrNoRows) {
		return ErrNotFound
	}
	return err
}
