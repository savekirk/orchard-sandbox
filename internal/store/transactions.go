package store

import (
	"crypto/rand"
	"database/sql"
	"encoding/json"
	"fmt"
	"math/big"

	"github.com/savekirk/orchard-sandbox/internal/model"
)

// FundingAccount is the merchant wallet a payment draws from, or "" for collections.
func FundingAccount(transType string) string {
	switch transType {
	case "MTC", "RMT":
		return model.AccountPayout
	case "ATP":
		return model.AccountAirtime
	case "BLP":
		return model.AccountBillpay
	}
	return ""
}

// NewTransID returns an 11-digit transaction ID like Orchard's.
func NewTransID() string {
	n, _ := rand.Int(rand.Reader, big.NewInt(89_999_999_999))
	return fmt.Sprintf("%d", n.Int64()+10_000_000_000)
}

// CreateTransaction reserves the exttrid, holds funds for payouts, and stores the pending transaction.
// It returns ErrDuplicate or ErrInsufficientBalance when the payment cannot be accepted.
func (s *Store) CreateTransaction(t *model.Transaction) error {
	now := s.timestamp()
	t.CreatedAt, t.UpdatedAt = now, now
	t.Status, t.TransStatus = model.StatusPending, model.CodeAccepted
	if t.Message == "" {
		t.Message = "PENDING"
	}
	if t.TransID == "" {
		t.TransID = NewTransID()
	}
	t.Amount = model.FormatAmount(t.AmountPesewas)
	meta, _ := json.Marshal(t.Meta)

	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()

	if err := reserve(tx, t.RunID, RefExttrid, t.Exttrid); err != nil {
		return err
	}
	if account := FundingAccount(t.TransType); account != "" {
		if err := s.move(tx, t.RunID, account, Debit, t.AmountPesewas, t.Exttrid, t.TransType+" to "+t.CustomerNumber); err != nil {
			return err
		}
	}
	_, err = tx.Exec(`INSERT INTO transactions (run_id, exttrid, trans_id, trans_type, channel, customer_number, nw, amount, reference,
		callback_url, status, trans_status, message, scenario, settle_at, meta, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		t.RunID, t.Exttrid, t.TransID, t.TransType, t.Channel, t.CustomerNumber, t.NW, t.AmountPesewas, t.Reference,
		t.CallbackURL, t.Status, t.TransStatus, t.Message, t.Scenario, nullable(t.SettleAt), string(meta), now, now)
	if err != nil {
		return fmt.Errorf("insert transaction: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return err
	}
	s.Publish(t.RunID, "transactions", "balances")
	return nil
}

func nullable(v int64) any {
	if v == 0 {
		return nil
	}
	return v
}

const txColumns = `run_id, exttrid, trans_id, trans_type, channel, customer_number, nw, amount, reference, callback_url,
	status, trans_status, message, scenario, settle_at, meta, created_at, updated_at`

func scanTransaction(row interface{ Scan(...any) error }) (*model.Transaction, error) {
	var t model.Transaction
	var settleAt sql.NullInt64
	var meta string
	err := row.Scan(&t.RunID, &t.Exttrid, &t.TransID, &t.TransType, &t.Channel, &t.CustomerNumber, &t.NW, &t.AmountPesewas,
		&t.Reference, &t.CallbackURL, &t.Status, &t.TransStatus, &t.Message, &t.Scenario, &settleAt, &meta, &t.CreatedAt, &t.UpdatedAt)
	if err != nil {
		return nil, notFound(err)
	}
	t.SettleAt = settleAt.Int64
	t.Amount = model.FormatAmount(t.AmountPesewas)
	_ = json.Unmarshal([]byte(meta), &t.Meta)
	return &t, nil
}

// GetTransaction returns a transaction by exttrid.
func (s *Store) GetTransaction(runID, exttrid string) (*model.Transaction, error) {
	return scanTransaction(s.db.QueryRow("SELECT "+txColumns+" FROM transactions WHERE run_id = ? AND exttrid = ?", runID, exttrid))
}

// ListTransactions returns a run's most recent transactions.
func (s *Store) ListTransactions(runID string, limit int) ([]*model.Transaction, error) {
	return s.queryTransactions("SELECT "+txColumns+" FROM transactions WHERE run_id = ? ORDER BY created_at DESC, rowid DESC LIMIT ?", runID, limit)
}

// SubscriptionDebits returns the debits made under an auto debit subscription.
func (s *Store) SubscriptionDebits(runID, uniqRefID string) ([]*model.Transaction, error) {
	return s.queryTransactions(`SELECT `+txColumns+` FROM transactions
		WHERE run_id = ? AND json_extract(meta, '$.uniq_ref_id') = ? ORDER BY rowid`, runID, uniqRefID)
}

func (s *Store) queryTransactions(query string, args ...any) ([]*model.Transaction, error) {
	rows, err := s.db.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []*model.Transaction{}
	for rows.Next() {
		t, err := scanTransaction(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, t)
	}
	return out, rows.Err()
}

// TransactionKey identifies a transaction.
type TransactionKey struct{ RunID, Exttrid string }

// DueTransactions returns pending transactions whose settle time has passed.
func (s *Store) DueTransactions(limit int) ([]TransactionKey, error) {
	rows, err := s.db.Query(`SELECT run_id, exttrid FROM transactions
		WHERE status = ? AND settle_at IS NOT NULL AND settle_at <= ? ORDER BY settle_at LIMIT ?`, model.StatusPending, s.millis(), limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []TransactionKey
	for rows.Next() {
		var k TransactionKey
		if err := rows.Scan(&k.RunID, &k.Exttrid); err != nil {
			return nil, err
		}
		out = append(out, k)
	}
	return out, rows.Err()
}

// SetPayer records who paid a hosted checkout and which scenario their number selects.
func (s *Store) SetPayer(runID, exttrid, customer, nw, scenario string) error {
	_, err := s.db.Exec(`UPDATE transactions SET customer_number = ?, nw = ?, scenario = ?, updated_at = ?
		WHERE run_id = ? AND exttrid = ? AND status = ?`, customer, nw, scenario, s.timestamp(), runID, exttrid, model.StatusPending)
	if err == nil {
		s.Publish(runID, "transactions")
	}
	return err
}

// Outcome is the final result of a pending transaction.
type Outcome struct {
	Success bool
	Message string
	// Notify queues a callback to the transaction's callback_url.
	Notify   bool
	Settings model.Settings
}

// Settle moves a pending transaction to its final state, applies balances, and queues the callback.
// Settling an already-final transaction is a no-op that returns it unchanged.
func (s *Store) Settle(runID, exttrid string, out Outcome) (*model.Transaction, error) {
	tx, err := s.db.Begin()
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback() }()

	t, err := scanTransaction(tx.QueryRow("SELECT "+txColumns+" FROM transactions WHERE run_id = ? AND exttrid = ?", runID, exttrid))
	if err != nil {
		return nil, err
	}
	if t.Status != model.StatusPending {
		return t, nil
	}

	t.Status, t.TransStatus = model.StatusFailed, model.TransStatusFailed
	if out.Success {
		t.Status, t.TransStatus = model.StatusSuccessful, model.TransStatusSuccess
	}
	t.Message = out.Message
	if t.Message == "" {
		t.Message = map[bool]string{true: "SUCCESS", false: "FAILED"}[out.Success]
	}
	t.UpdatedAt, t.SettleAt = s.timestamp(), 0

	funding := FundingAccount(t.TransType)
	switch {
	case out.Success && funding == "":
		for _, account := range []string{model.AccountAvailableCollect, model.AccountActualCollect} {
			if err := s.move(tx, runID, account, Credit, t.AmountPesewas, t.Exttrid, t.TransType+" from "+t.CustomerNumber); err != nil {
				return nil, err
			}
		}
	case !out.Success && funding != "":
		if err := s.move(tx, runID, funding, Credit, t.AmountPesewas, t.Exttrid, "Reversal of failed "+t.TransType); err != nil {
			return nil, err
		}
	}

	if _, err := tx.Exec(`UPDATE transactions SET status = ?, trans_status = ?, message = ?, settle_at = NULL, updated_at = ?
		WHERE run_id = ? AND exttrid = ?`, t.Status, t.TransStatus, t.Message, t.UpdatedAt, runID, exttrid); err != nil {
		return nil, err
	}

	events := []string{"transactions", "balances"}
	if out.Notify && t.CallbackURL != "" {
		payload, _ := json.Marshal(model.CallbackPayload{TransID: t.TransID, TransRef: t.Exttrid, TransStatus: t.TransStatus, Message: t.Message})
		if err := s.insertCallback(tx, runID, t.Exttrid, t.CallbackURL, string(payload), out.Settings); err != nil {
			return nil, err
		}
		events = append(events, "callbacks")
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	s.Publish(runID, events...)
	return t, nil
}
