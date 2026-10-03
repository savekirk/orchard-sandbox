package store

import (
	"database/sql"
	"time"

	"github.com/savekirk/orchard-sandbox/internal/model"
)

func (s *Store) insertCallback(q execer, runID, exttrid, url, payload string, settings model.Settings) error {
	settings = settings.Normalize()
	_, err := q.Exec(`INSERT INTO callbacks (id, run_id, exttrid, url, payload, state, max_attempts, timeout_ms, next_attempt_at, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		"cb_"+randomHex(6), runID, exttrid, url, payload, model.CallbackQueued, settings.CallbackAttempts, settings.CallbackTimeoutMs, s.millis(), s.timestamp())
	return err
}

// EnqueueCallback queues a webhook POST of payload to url.
func (s *Store) EnqueueCallback(runID, exttrid, url, payload string, settings model.Settings) error {
	if err := s.insertCallback(s.db, runID, exttrid, url, payload, settings); err != nil {
		return err
	}
	s.Publish(runID, "callbacks")
	return nil
}

// Delivery is a callback claimed for delivery.
type Delivery struct {
	model.Callback
	TimeoutMs int
}

// ClaimDueCallbacks marks due callbacks as DELIVERING and returns them.
func (s *Store) ClaimDueCallbacks(limit int) ([]Delivery, error) {
	rows, err := s.db.Query(`SELECT id, run_id, exttrid, url, payload, attempts, max_attempts, timeout_ms FROM callbacks
		WHERE state = ? AND next_attempt_at <= ? ORDER BY next_attempt_at LIMIT ?`, model.CallbackQueued, s.millis(), limit)
	if err != nil {
		return nil, err
	}
	var due []Delivery
	for rows.Next() {
		var d Delivery
		if err := rows.Scan(&d.ID, &d.RunID, &d.Exttrid, &d.URL, &d.Payload, &d.Attempts, &d.MaxAttempts, &d.TimeoutMs); err != nil {
			rows.Close()
			return nil, err
		}
		due = append(due, d)
	}
	rows.Close()

	claimed := due[:0]
	for _, d := range due {
		res, err := s.db.Exec("UPDATE callbacks SET state = ? WHERE id = ? AND state = ?", model.CallbackDelivering, d.ID, model.CallbackQueued)
		if err != nil {
			return nil, err
		}
		if n, _ := res.RowsAffected(); n == 1 {
			claimed = append(claimed, d)
		}
	}
	return claimed, nil
}

// RecordDelivery stores an attempt and moves the callback to DELIVERED, back to QUEUED (retry after backoff), or FAILED.
func (s *Store) RecordDelivery(d Delivery, attempt model.CallbackAttempt, backoff time.Duration) error {
	attempts := d.Attempts + 1
	delivered := attempt.HTTPStatus >= 200 && attempt.HTTPStatus < 300

	state, next, deliveredAt := model.CallbackQueued, sql.NullInt64{Int64: s.now().Add(backoff).UnixMilli(), Valid: true}, ""
	switch {
	case delivered:
		state, next, deliveredAt = model.CallbackDelivered, sql.NullInt64{}, s.timestamp()
	case attempts >= d.MaxAttempts:
		state, next = model.CallbackFailed, sql.NullInt64{}
	}

	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	if _, err := tx.Exec(`INSERT INTO callback_attempts (callback_id, http_status, response_body, error, duration_ms, attempted_at)
		VALUES (?, ?, ?, ?, ?, ?)`, d.ID, attempt.HTTPStatus, attempt.ResponseBody, attempt.Error, attempt.DurationMs, s.timestamp()); err != nil {
		return err
	}
	if _, err := tx.Exec(`UPDATE callbacks SET state = ?, attempts = ?, next_attempt_at = ?, last_status = ?, last_error = ?, delivered_at = ?
		WHERE id = ?`, state, attempts, next, attempt.HTTPStatus, attempt.Error, deliveredAt, d.ID); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return err
	}
	s.Publish(d.RunID, "callbacks")
	return nil
}

// RequeueStuckCallbacks returns callbacks interrupted mid-delivery (e.g. by a restart) to the queue.
func (s *Store) RequeueStuckCallbacks() error {
	_, err := s.db.Exec("UPDATE callbacks SET state = ? WHERE state = ?", model.CallbackQueued, model.CallbackDelivering)
	return err
}

// RetryCallback queues a callback for one more immediate attempt.
func (s *Store) RetryCallback(runID, id string) error {
	res, err := s.db.Exec(`UPDATE callbacks SET state = ?, next_attempt_at = ?, max_attempts = attempts + 1
		WHERE run_id = ? AND id = ? AND state != ?`, model.CallbackQueued, s.millis(), runID, id, model.CallbackDelivering)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	s.Publish(runID, "callbacks")
	return nil
}

const callbackColumns = "id, run_id, exttrid, url, payload, state, attempts, max_attempts, next_attempt_at, last_status, last_error, created_at, delivered_at"

// ListCallbacks returns a run's most recent callbacks.
func (s *Store) ListCallbacks(runID string, limit int) ([]model.Callback, error) {
	rows, err := s.db.Query("SELECT "+callbackColumns+" FROM callbacks WHERE run_id = ? ORDER BY created_at DESC, rowid DESC LIMIT ?", runID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []model.Callback{}
	for rows.Next() {
		var c model.Callback
		var next sql.NullInt64
		if err := rows.Scan(&c.ID, &c.RunID, &c.Exttrid, &c.URL, &c.Payload, &c.State, &c.Attempts, &c.MaxAttempts, &next,
			&c.LastStatus, &c.LastError, &c.CreatedAt, &c.DeliveredAt); err != nil {
			return nil, err
		}
		c.NextAttemptAt = next.Int64
		out = append(out, c)
	}
	return out, rows.Err()
}

// CallbackAttempts returns the delivery attempts of a callback.
func (s *Store) CallbackAttempts(runID, id string) ([]model.CallbackAttempt, error) {
	rows, err := s.db.Query(`SELECT a.id, a.callback_id, a.http_status, a.response_body, a.error, a.duration_ms, a.attempted_at
		FROM callback_attempts a JOIN callbacks c ON c.id = a.callback_id WHERE c.run_id = ? AND c.id = ? ORDER BY a.id`, runID, id)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []model.CallbackAttempt{}
	for rows.Next() {
		var a model.CallbackAttempt
		if err := rows.Scan(&a.ID, &a.CallbackID, &a.HTTPStatus, &a.ResponseBody, &a.Error, &a.DurationMs, &a.AttemptedAt); err != nil {
			return nil, err
		}
		out = append(out, a)
	}
	return out, rows.Err()
}
