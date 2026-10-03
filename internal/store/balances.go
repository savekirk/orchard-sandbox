package store

import (
	"database/sql"
	"errors"
	"fmt"

	"github.com/savekirk/orchard-sandbox/internal/model"
)

// ErrInsufficientBalance is returned when a debit exceeds the account balance.
var ErrInsufficientBalance = errors.New("insufficient balance")

// Ledger directions.
const (
	Credit = "CREDIT"
	Debit  = "DEBIT"
)

// Balances returns a run's balances per account (pesewas; SMS in units).
func (s *Store) Balances(runID string) (map[string]int64, error) {
	rows, err := s.db.Query("SELECT account, amount FROM balances WHERE run_id = ?", runID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]int64{}
	for rows.Next() {
		var account string
		var amount int64
		if err := rows.Scan(&account, &amount); err != nil {
			return nil, err
		}
		out[account] = amount
	}
	return out, rows.Err()
}

// WalletBalances returns balances in Orchard's /check_wallet_balance shape.
func (s *Store) WalletBalances(runID string) (model.Balances, error) {
	b, err := s.Balances(runID)
	if err != nil {
		return model.Balances{}, err
	}
	return model.Balances{
		SMSBal:              float64(b[model.AccountSMS]),
		PayoutBal:           model.Cedis(b[model.AccountPayout]),
		BillpayBal:          model.Cedis(b[model.AccountBillpay]),
		AvailableCollectBal: model.Cedis(b[model.AccountAvailableCollect]),
		AirtimeBal:          model.Cedis(b[model.AccountAirtime]),
		ActualCollectBal:    model.Cedis(b[model.AccountActualCollect]),
	}, nil
}

// SetBalances adjusts accounts to the given amounts, recording the difference in the ledger.
func (s *Store) SetBalances(runID string, target map[string]int64) error {
	current, err := s.Balances(runID)
	if err != nil {
		return err
	}
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	for _, account := range model.Accounts {
		want, ok := target[account]
		if !ok || want < 0 || want == current[account] {
			continue
		}
		diff, direction := want-current[account], Credit
		if diff < 0 {
			diff, direction = -diff, Debit
		}
		if err := s.move(tx, runID, account, direction, diff, "", "Manual adjustment"); err != nil {
			return err
		}
	}
	if err := tx.Commit(); err != nil {
		return err
	}
	s.Publish(runID, "balances")
	return nil
}

// move changes one account balance inside tx and appends a ledger entry.
func (s *Store) move(tx *sql.Tx, runID, account, direction string, amount int64, exttrid, note string) error {
	if amount <= 0 {
		return nil
	}
	var balance int64
	if err := tx.QueryRow("SELECT amount FROM balances WHERE run_id = ? AND account = ?", runID, account).Scan(&balance); err != nil {
		return fmt.Errorf("read %s balance: %w", account, notFound(err))
	}
	switch direction {
	case Credit:
		balance += amount
	case Debit:
		if balance < amount {
			return ErrInsufficientBalance
		}
		balance -= amount
	default:
		return fmt.Errorf("unknown ledger direction %q", direction)
	}
	if _, err := tx.Exec("UPDATE balances SET amount = ? WHERE run_id = ? AND account = ?", balance, runID, account); err != nil {
		return err
	}
	_, err := tx.Exec(`INSERT INTO ledger (run_id, account, direction, amount, balance_after, exttrid, note, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)`, runID, account, direction, amount, balance, exttrid, note, s.timestamp())
	return err
}

// Ledger returns the most recent ledger entries for a run.
func (s *Store) Ledger(runID string, limit int) ([]model.LedgerEntry, error) {
	rows, err := s.db.Query(`SELECT id, run_id, account, direction, amount, balance_after, exttrid, note, created_at
		FROM ledger WHERE run_id = ? ORDER BY id DESC LIMIT ?`, runID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []model.LedgerEntry{}
	for rows.Next() {
		var e model.LedgerEntry
		if err := rows.Scan(&e.ID, &e.RunID, &e.Account, &e.Direction, &e.Amount, &e.BalanceAfter, &e.Exttrid, &e.Note, &e.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, e)
	}
	return out, rows.Err()
}

// ChargeSMS reserves the message's unique_id, deducts SMS units, and records the message atomically.
func (s *Store) ChargeSMS(msg *model.SMS) error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	if err := reserve(tx, msg.RunID, RefSMS, msg.UniqueID); err != nil {
		return err
	}
	if err := s.move(tx, msg.RunID, model.AccountSMS, Debit, int64(msg.Pages), msg.UniqueID, "SMS to "+msg.Recipient); err != nil {
		return err
	}
	if err := s.insertSMS(tx, msg); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return err
	}
	s.Publish(msg.RunID, "sms", "balances")
	return nil
}
