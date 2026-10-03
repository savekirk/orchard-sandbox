package store

import (
	"encoding/json"
	"strings"

	"github.com/savekirk/orchard-sandbox/internal/model"
)

// LogRequest records a call to an Orchard endpoint.
func (s *Store) LogRequest(r *model.RequestLog) {
	headers, _ := json.Marshal(r.Headers)
	r.CreatedAt = s.timestamp()
	res, err := s.db.Exec(`INSERT INTO requests (run_id, method, path, operation, ref, headers, body, status, response_body, resp_code, duration_ms, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		r.RunID, r.Method, r.Path, r.Operation, r.Ref, string(headers), r.Body, r.Status, r.ResponseBody, r.RespCode, r.DurationMs, r.CreatedAt)
	if err != nil {
		return
	}
	r.ID, _ = res.LastInsertId()
	s.Publish(r.RunID, "requests")
}

// ListRequests returns a run's most recent requests. A non-empty ref limits results to that reference.
func (s *Store) ListRequests(runID, ref string, limit int) ([]model.RequestLog, error) {
	query := `SELECT id, run_id, method, path, operation, ref, headers, body, status, response_body, resp_code, duration_ms, created_at
		FROM requests WHERE run_id = ?`
	args := []any{runID}
	if ref != "" {
		query += " AND ref = ?"
		args = append(args, ref)
	}
	query += " ORDER BY id DESC LIMIT ?"
	args = append(args, limit)

	rows, err := s.db.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []model.RequestLog{}
	for rows.Next() {
		var r model.RequestLog
		var headers string
		if err := rows.Scan(&r.ID, &r.RunID, &r.Method, &r.Path, &r.Operation, &r.Ref, &headers, &r.Body, &r.Status,
			&r.ResponseBody, &r.RespCode, &r.DurationMs, &r.CreatedAt); err != nil {
			return nil, err
		}
		_ = json.Unmarshal([]byte(headers), &r.Headers)
		out = append(out, r)
	}
	return out, rows.Err()
}

// CountAttempts returns how many earlier requests used the same operation and reference.
func (s *Store) CountAttempts(runID, operation, ref string) int {
	var n int
	_ = s.db.QueryRow("SELECT count(*) FROM requests WHERE run_id = ? AND operation = ? AND ref = ?", runID, operation, ref).Scan(&n)
	return n
}

// Counts summarises a run's activity for the dashboard.
type Counts struct {
	Requests        int `json:"requests"`
	Transactions    int `json:"transactions"`
	Pending         int `json:"pending"`
	CallbacksFailed int `json:"callbacks_failed"`
	SMS             int `json:"sms"`
	Subscriptions   int `json:"subscriptions"`
}

// Counts returns activity totals for a run.
func (s *Store) Counts(runID string) Counts {
	var c Counts
	_ = s.db.QueryRow(`SELECT
		(SELECT count(*) FROM requests WHERE run_id = ?1),
		(SELECT count(*) FROM transactions WHERE run_id = ?1),
		(SELECT count(*) FROM transactions WHERE run_id = ?1 AND status = 'PENDING'),
		(SELECT count(*) FROM callbacks WHERE run_id = ?1 AND state = 'FAILED'),
		(SELECT count(*) FROM sms WHERE run_id = ?1),
		(SELECT count(*) FROM subscriptions WHERE run_id = ?1)`, runID).
		Scan(&c.Requests, &c.Transactions, &c.Pending, &c.CallbacksFailed, &c.SMS, &c.Subscriptions)
	return c
}

func (s *Store) insertSMS(q execer, msg *model.SMS) error {
	msg.CreatedAt = s.timestamp()
	res, err := q.Exec(`INSERT INTO sms (run_id, unique_id, sender_id, recipient, body, msg_type, pages, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)`, msg.RunID, msg.UniqueID, msg.SenderID, msg.Recipient, msg.Body, msg.MsgType, msg.Pages, msg.CreatedAt)
	if err != nil {
		return err
	}
	msg.ID, _ = res.LastInsertId()
	return nil
}

// RecordSMS stores a message without charging for it (e.g. OTPs the sandbox sends on Orchard's behalf).
func (s *Store) RecordSMS(msg *model.SMS) error {
	if err := s.insertSMS(s.db, msg); err != nil {
		return err
	}
	s.Publish(msg.RunID, "sms")
	return nil
}

// ListSMS returns a run's most recent messages.
func (s *Store) ListSMS(runID string, limit int) ([]model.SMS, error) {
	rows, err := s.db.Query(`SELECT id, run_id, unique_id, sender_id, recipient, body, msg_type, pages, created_at
		FROM sms WHERE run_id = ? ORDER BY id DESC LIMIT ?`, runID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []model.SMS{}
	for rows.Next() {
		var m model.SMS
		if err := rows.Scan(&m.ID, &m.RunID, &m.UniqueID, &m.SenderID, &m.Recipient, &m.Body, &m.MsgType, &m.Pages, &m.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, m)
	}
	return out, rows.Err()
}

// SaveCard creates or replaces a Ghana Card profile.
func (s *Store) SaveCard(runID string, c model.Card) error {
	_, err := s.db.Exec(`INSERT INTO cards (run_id, id_num, name, gender, verified, card_valid_start, card_valid_end) VALUES (?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT (run_id, id_num) DO UPDATE SET name = excluded.name, gender = excluded.gender, verified = excluded.verified,
		card_valid_start = excluded.card_valid_start, card_valid_end = excluded.card_valid_end`,
		runID, c.IDNum, c.Name, c.Gender, c.Verified, c.CardValidStart, c.CardValidEnd)
	if err == nil {
		s.Publish(runID, "cards")
	}
	return err
}

// GetCard returns a registered Ghana Card profile.
func (s *Store) GetCard(runID, idNum string) (*model.Card, error) {
	c := model.Card{IDNum: idNum}
	err := s.db.QueryRow("SELECT name, gender, verified, card_valid_start, card_valid_end FROM cards WHERE run_id = ? AND id_num = ?", runID, idNum).
		Scan(&c.Name, &c.Gender, &c.Verified, &c.CardValidStart, &c.CardValidEnd)
	if err != nil {
		return nil, notFound(err)
	}
	return &c, nil
}

// ListCards returns a run's registered Ghana Card profiles.
func (s *Store) ListCards(runID string) ([]model.Card, error) {
	rows, err := s.db.Query("SELECT id_num, name, gender, verified, card_valid_start, card_valid_end FROM cards WHERE run_id = ? ORDER BY id_num", runID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []model.Card{}
	for rows.Next() {
		var c model.Card
		if err := rows.Scan(&c.IDNum, &c.Name, &c.Gender, &c.Verified, &c.CardValidStart, &c.CardValidEnd); err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

// DeleteCard removes a Ghana Card profile.
func (s *Store) DeleteCard(runID, idNum string) error {
	return s.deleteOne(runID, "cards", "DELETE FROM cards WHERE run_id = ? AND id_num = ?", runID, idNum)
}

// SaveAccount creates or replaces an account inquiry profile.
func (s *Store) SaveAccount(runID string, a model.Account) error {
	if a.RespCode == "" {
		a.RespCode = model.CodeCompleted
	}
	a.BankCode = strings.ToUpper(a.BankCode)
	_, err := s.db.Exec(`INSERT INTO accounts (run_id, bank_code, account_number, account_name, resp_code) VALUES (?, ?, ?, ?, ?)
		ON CONFLICT (run_id, bank_code, account_number) DO UPDATE SET account_name = excluded.account_name, resp_code = excluded.resp_code`,
		runID, a.BankCode, a.AccountNumber, a.AccountName, a.RespCode)
	if err == nil {
		s.Publish(runID, "accounts")
	}
	return err
}

// GetAccount returns a registered account inquiry profile.
func (s *Store) GetAccount(runID, bankCode, accountNumber string) (*model.Account, error) {
	a := model.Account{BankCode: bankCode, AccountNumber: accountNumber}
	err := s.db.QueryRow("SELECT account_name, resp_code FROM accounts WHERE run_id = ? AND bank_code = ? AND account_number = ?", runID, bankCode, accountNumber).
		Scan(&a.AccountName, &a.RespCode)
	if err != nil {
		return nil, notFound(err)
	}
	return &a, nil
}

// ListAccounts returns a run's registered account inquiry profiles.
func (s *Store) ListAccounts(runID string) ([]model.Account, error) {
	rows, err := s.db.Query("SELECT bank_code, account_number, account_name, resp_code FROM accounts WHERE run_id = ? ORDER BY bank_code, account_number", runID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []model.Account{}
	for rows.Next() {
		var a model.Account
		if err := rows.Scan(&a.BankCode, &a.AccountNumber, &a.AccountName, &a.RespCode); err != nil {
			return nil, err
		}
		out = append(out, a)
	}
	return out, rows.Err()
}

// DeleteAccount removes an account inquiry profile.
func (s *Store) DeleteAccount(runID, bankCode, accountNumber string) error {
	return s.deleteOne(runID, "accounts", "DELETE FROM accounts WHERE run_id = ? AND bank_code = ? AND account_number = ?", runID, bankCode, accountNumber)
}

func (s *Store) deleteOne(runID, event, query string, args ...any) error {
	res, err := s.db.Exec(query, args...)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	s.Publish(runID, event)
	return nil
}
