package store

import (
	"github.com/savekirk/orchard-sandbox/internal/model"
)

// CreateSubscription reserves the uniq_ref_id and stores a pending auto debit mandate.
func (s *Store) CreateSubscription(sub *model.Subscription) error {
	sub.SubscribedAt = s.timestamp()
	sub.Status = model.SubscriptionPending
	sub.Amount = model.FormatAmount(sub.AmountPesewas)

	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	if err := reserve(tx, sub.RunID, RefSubscription, sub.UniqRefID); err != nil {
		return err
	}
	_, err = tx.Exec(`INSERT INTO subscriptions (run_id, uniq_ref_id, service_id, customer_number, nw, amount, cycle, start_date, end_date,
		reference, return_url, resumable, cycle_skip, apply_penalty, status, otp, subscribed_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		sub.RunID, sub.UniqRefID, sub.ServiceID, sub.CustomerNumber, sub.NW, sub.AmountPesewas, sub.Cycle, sub.StartDate, sub.EndDate,
		sub.Reference, sub.ReturnURL, sub.Resumable, sub.CycleSkip, sub.ApplyPenalty, sub.Status, sub.OTP, sub.SubscribedAt)
	if err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return err
	}
	s.Publish(sub.RunID, "subscriptions")
	return nil
}

const subscriptionColumns = `run_id, uniq_ref_id, service_id, customer_number, nw, amount, cycle, start_date, end_date, reference,
	return_url, resumable, cycle_skip, apply_penalty, status, otp, subscribed_at, activated_at, cancelled_at`

func scanSubscription(row interface{ Scan(...any) error }) (*model.Subscription, error) {
	var sub model.Subscription
	err := row.Scan(&sub.RunID, &sub.UniqRefID, &sub.ServiceID, &sub.CustomerNumber, &sub.NW, &sub.AmountPesewas, &sub.Cycle,
		&sub.StartDate, &sub.EndDate, &sub.Reference, &sub.ReturnURL, &sub.Resumable, &sub.CycleSkip, &sub.ApplyPenalty,
		&sub.Status, &sub.OTP, &sub.SubscribedAt, &sub.ActivatedAt, &sub.CancelledAt)
	if err != nil {
		return nil, notFound(err)
	}
	sub.Amount = model.FormatAmount(sub.AmountPesewas)
	return &sub, nil
}

// GetSubscription returns an auto debit mandate.
func (s *Store) GetSubscription(runID, uniqRefID string) (*model.Subscription, error) {
	return scanSubscription(s.db.QueryRow("SELECT "+subscriptionColumns+" FROM subscriptions WHERE run_id = ? AND uniq_ref_id = ?", runID, uniqRefID))
}

// ActiveSubscriptionFor returns an active mandate for a customer number.
func (s *Store) ActiveSubscriptionFor(runID, customerNumber string) (*model.Subscription, error) {
	return scanSubscription(s.db.QueryRow("SELECT "+subscriptionColumns+" FROM subscriptions WHERE run_id = ? AND customer_number = ? AND status = ? LIMIT 1",
		runID, customerNumber, model.SubscriptionActive))
}

// ListSubscriptions returns a run's auto debit mandates, newest first.
func (s *Store) ListSubscriptions(runID string) ([]*model.Subscription, error) {
	rows, err := s.db.Query("SELECT "+subscriptionColumns+" FROM subscriptions WHERE run_id = ? ORDER BY subscribed_at DESC, rowid DESC", runID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []*model.Subscription{}
	for rows.Next() {
		sub, err := scanSubscription(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, sub)
	}
	return out, rows.Err()
}

// UpdateSubscription saves a mandate's status, OTP, and lifecycle timestamps.
func (s *Store) UpdateSubscription(sub *model.Subscription) error {
	_, err := s.db.Exec(`UPDATE subscriptions SET status = ?, otp = ?, activated_at = ?, cancelled_at = ? WHERE run_id = ? AND uniq_ref_id = ?`,
		sub.Status, sub.OTP, sub.ActivatedAt, sub.CancelledAt, sub.RunID, sub.UniqRefID)
	if err == nil {
		s.Publish(sub.RunID, "subscriptions")
	}
	return err
}

// AddFailureRule stores a fault injection rule.
func (s *Store) AddFailureRule(r *model.FailureRule) error {
	r.ID = "rule_" + randomHex(4)
	r.CreatedAt = s.timestamp()
	if r.Operation == "" {
		r.Operation = "*"
	}
	if r.Exttrid == "" {
		r.Exttrid = "*"
	}
	_, err := s.db.Exec(`INSERT INTO failure_rules (id, run_id, operation, exttrid, attempt, action, http_status, resp_code, delay_ms, hold, response_body, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		r.ID, r.RunID, r.Operation, r.Exttrid, r.Attempt, r.Action, r.HTTPStatus, r.RespCode, r.DelayMs, r.Hold, r.ResponseBody, r.CreatedAt)
	if err == nil {
		s.Publish(r.RunID, "failures")
	}
	return err
}

// ListFailureRules returns a run's fault injection rules in creation order.
func (s *Store) ListFailureRules(runID string) ([]model.FailureRule, error) {
	rows, err := s.db.Query(`SELECT id, run_id, operation, exttrid, attempt, action, http_status, resp_code, delay_ms, hold, response_body, created_at
		FROM failure_rules WHERE run_id = ? ORDER BY rowid`, runID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []model.FailureRule{}
	for rows.Next() {
		var r model.FailureRule
		if err := rows.Scan(&r.ID, &r.RunID, &r.Operation, &r.Exttrid, &r.Attempt, &r.Action, &r.HTTPStatus, &r.RespCode,
			&r.DelayMs, &r.Hold, &r.ResponseBody, &r.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

// DeleteFailureRule removes a rule.
func (s *Store) DeleteFailureRule(runID, id string) error {
	return s.deleteOne(runID, "failures", "DELETE FROM failure_rules WHERE run_id = ? AND id = ?", runID, id)
}
