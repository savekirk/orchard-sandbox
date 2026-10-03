package store

import (
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/savekirk/orchard-sandbox/internal/model"
)

// NewRun describes a sandbox to create. Empty credentials are generated.
type NewRun struct {
	RunID     string           `json:"run_id"`
	Name      string           `json:"name"`
	ClientKey string           `json:"client_key"`
	SecretKey string           `json:"secret_key"`
	ServiceID string           `json:"service_id"`
	Settings  *model.Settings  `json:"settings"`
	Balances  map[string]int64 `json:"-"`
}

// ErrRunExists is returned when a run ID or client key is already taken.
var ErrRunExists = errors.New("run_id or client_key already in use")

func randomHex(n int) string {
	b := make([]byte, n)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

// CreateRun provisions an isolated sandbox. Settings default to those of the default run.
func (s *Store) CreateRun(in NewRun) (*model.Run, error) {
	if in.RunID == "" {
		in.RunID = "run_" + randomHex(4)
	}
	if strings.ContainsAny(in.RunID, "/ ?#%") {
		return nil, fmt.Errorf("%w: run_id may not contain spaces or /?#%%", ErrInvalid)
	}
	if in.Name == "" {
		in.Name = in.RunID
	}
	if in.ClientKey == "" {
		in.ClientKey = "ck_" + randomHex(12)
	}
	if in.SecretKey == "" {
		in.SecretKey = "sk_" + randomHex(24)
	}

	var settings model.Settings
	balances := in.Balances
	if def, err := s.GetRun(DefaultRunID); err == nil {
		settings = def.Settings
		if in.ServiceID == "" {
			in.ServiceID = def.ServiceID
		}
		if balances == nil {
			balances, _ = s.openingBalances(DefaultRunID)
		}
	}
	if in.Settings != nil {
		settings = *in.Settings
	}
	if in.ServiceID == "" {
		in.ServiceID = "1234"
	}
	settings = settings.Normalize()

	settingsJSON, _ := json.Marshal(settings)
	balancesJSON, _ := json.Marshal(balances)

	tx, err := s.db.Begin()
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback() }()

	_, err = tx.Exec(`INSERT INTO runs (run_id, name, client_key, secret_key, service_id, settings, opening_balances, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		in.RunID, in.Name, in.ClientKey, in.SecretKey, in.ServiceID, string(settingsJSON), string(balancesJSON), s.timestamp())
	if err != nil {
		if strings.Contains(err.Error(), "UNIQUE") {
			return nil, ErrRunExists
		}
		return nil, err
	}
	if err := setBalances(tx, in.RunID, balances); err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	s.Publish(in.RunID, "runs")
	return s.GetRun(in.RunID)
}

func setBalances(tx *sql.Tx, runID string, balances map[string]int64) error {
	for _, account := range model.Accounts {
		if _, err := tx.Exec(`INSERT INTO balances (run_id, account, amount) VALUES (?, ?, ?)
			ON CONFLICT (run_id, account) DO UPDATE SET amount = excluded.amount`, runID, account, balances[account]); err != nil {
			return fmt.Errorf("set balance: %w", err)
		}
	}
	return nil
}

// EnsureDefaultRun creates the default run, or refreshes its credentials and settings from configuration.
func (s *Store) EnsureDefaultRun(in NewRun) (*model.Run, error) {
	in.RunID = DefaultRunID
	if in.Name == "" {
		in.Name = "Default sandbox"
	}
	if _, err := s.GetRun(DefaultRunID); errors.Is(err, ErrNotFound) {
		return s.CreateRun(in)
	}
	settings := model.Settings{}
	if in.Settings != nil {
		settings = *in.Settings
	}
	settingsJSON, _ := json.Marshal(settings.Normalize())
	balancesJSON, _ := json.Marshal(in.Balances)
	_, err := s.db.Exec(`UPDATE runs SET client_key = ?, secret_key = ?, service_id = ?, settings = ?, opening_balances = ? WHERE run_id = ?`,
		in.ClientKey, in.SecretKey, in.ServiceID, string(settingsJSON), string(balancesJSON), DefaultRunID)
	if err != nil {
		return nil, fmt.Errorf("refresh default run: %w", err)
	}
	return s.GetRun(DefaultRunID)
}

const runColumns = "run_id, name, client_key, secret_key, service_id, settings, created_at"

func scanRun(row interface{ Scan(...any) error }) (*model.Run, error) {
	var r model.Run
	var settings string
	if err := row.Scan(&r.RunID, &r.Name, &r.ClientKey, &r.SecretKey, &r.ServiceID, &settings, &r.CreatedAt); err != nil {
		return nil, notFound(err)
	}
	_ = json.Unmarshal([]byte(settings), &r.Settings)
	r.Settings = r.Settings.Normalize()
	return &r, nil
}

// GetRun returns a run by ID.
func (s *Store) GetRun(runID string) (*model.Run, error) {
	return scanRun(s.db.QueryRow("SELECT "+runColumns+" FROM runs WHERE run_id = ?", runID))
}

// RunByClientKey returns the run that owns a client key.
func (s *Store) RunByClientKey(clientKey string) (*model.Run, error) {
	return scanRun(s.db.QueryRow("SELECT "+runColumns+" FROM runs WHERE client_key = ?", clientKey))
}

// ListRuns returns every run, default first.
func (s *Store) ListRuns() ([]*model.Run, error) {
	rows, err := s.db.Query("SELECT " + runColumns + " FROM runs ORDER BY run_id != 'default', created_at")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	runs := []*model.Run{}
	for rows.Next() {
		r, err := scanRun(rows)
		if err != nil {
			return nil, err
		}
		runs = append(runs, r)
	}
	return runs, rows.Err()
}

// UpdateSettings replaces a run's settings.
func (s *Store) UpdateSettings(runID string, settings model.Settings) (*model.Run, error) {
	raw, _ := json.Marshal(settings.Normalize())
	res, err := s.db.Exec("UPDATE runs SET settings = ? WHERE run_id = ?", string(raw), runID)
	if err != nil {
		return nil, err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return nil, ErrNotFound
	}
	s.Publish(runID, "settings")
	return s.GetRun(runID)
}

// DeleteRun removes a run and all of its data. The default run cannot be deleted.
func (s *Store) DeleteRun(runID string) error {
	if runID == DefaultRunID {
		return fmt.Errorf("%w: the default run cannot be deleted; reset it instead", ErrInvalid)
	}
	res, err := s.db.Exec("DELETE FROM runs WHERE run_id = ?", runID)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	s.Publish(runID, "runs")
	return nil
}

// ResetRun clears a run's activity and restores its opening balances. Test data (cards, accounts) and failure rules are kept.
func (s *Store) ResetRun(runID string) error {
	if _, err := s.GetRun(runID); err != nil {
		return err
	}
	balances, err := s.openingBalances(runID)
	if err != nil {
		return err
	}
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	for _, table := range []string{"ledger", "refs", "transactions", "requests", "callbacks", "sms", "subscriptions"} {
		if _, err := tx.Exec("DELETE FROM "+table+" WHERE run_id = ?", runID); err != nil {
			return fmt.Errorf("reset %s: %w", table, err)
		}
	}
	if err := setBalances(tx, runID, balances); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return err
	}
	s.Publish(runID, "reset")
	return nil
}

func (s *Store) openingBalances(runID string) (map[string]int64, error) {
	var raw string
	if err := s.db.QueryRow("SELECT opening_balances FROM runs WHERE run_id = ?", runID).Scan(&raw); err != nil {
		return nil, notFound(err)
	}
	balances := map[string]int64{}
	_ = json.Unmarshal([]byte(raw), &balances)
	return balances, nil
}
