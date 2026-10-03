package handler

import (
	"crypto/rand"
	"encoding/json"
	"fmt"
	"math/big"
	"strings"

	"github.com/savekirk/orchard-sandbox/internal/model"
	"github.com/savekirk/orchard-sandbox/internal/store"
)

const serviceName = "Sandbox Merchant"

var cycleNames = map[string]string{"DLY": "Daily", "WKL": "Weekly", "MON": "Monthly"}

// autoDebitReply is the body Orchard returns for auto debit operations.
type autoDebitReply struct {
	ServiceID   any    `json:"service_id"`
	ServiceName string `json:"service_name"`
	ActivatedAt string `json:"activated_at"`
	UniqRefID   string `json:"uniq_ref_id"`
	StatusCode  string `json:"status_code"`
	Operation   string `json:"operation"`
	Message     string `json:"message,omitempty"`
}

func newOTP() string {
	n, _ := rand.Int(rand.Reader, big.NewInt(100000))
	return fmt.Sprintf("%05d", n.Int64())
}

// AutoDebit handles POST /autoDebit (recurring debit mandates).
func (h *Handler) AutoDebit(c *call) {
	ref := c.str("uniq_ref_id")
	op := strings.ToUpper(c.str("operation"))
	switch {
	case ref == "":
		c.code(model.CodeMissingExttrid)
		return
	case len(ref) > 20:
		c.code(model.CodeExttridTooLong20)
		return
	case op == "":
		c.code(model.CodeIncompleteParams)
		return
	case !c.timestamp(false):
		return
	}
	if op == "SUB" {
		h.subscribe(c, ref)
		return
	}

	sub, err := h.st.GetSubscription(c.run.RunID, ref)
	if err != nil {
		c.code(model.CodeNoRecord)
		return
	}
	reply := autoDebitReply{
		ServiceID: c.fields["service_id"], ServiceName: serviceName, ActivatedAt: sub.ActivatedAt,
		UniqRefID: sub.UniqRefID, StatusCode: "000", Operation: op,
	}
	refuse := func(message string) {
		reply.StatusCode, reply.Message = "001", message
		c.reply(reply)
	}

	switch op {
	case "OTP":
		switch {
		case c.str("auth_code") == "":
			c.code(model.CodeIncompleteParams)
			return
		case sub.Status != model.SubscriptionPending:
			refuse("Subscription is not awaiting activation")
			return
		case c.str("auth_code") != sub.OTP:
			refuse("Invalid activation code")
			return
		}
		sub.Status, sub.ActivatedAt = model.SubscriptionActive, h.st.Now().UTC().Format("2006-01-02 15:04:05")
		reply.ActivatedAt = sub.ActivatedAt
		if payload, err := json.Marshal(reply); err == nil {
			_ = h.st.EnqueueCallback(c.run.RunID, sub.UniqRefID, sub.ReturnURL, string(payload), c.run.Settings)
		}
	case "OTR":
		if sub.Status != model.SubscriptionPending {
			refuse("Subscription is not awaiting activation")
			return
		}
		sub.OTP = newOTP()
		if err := h.st.UpdateSubscription(sub); err != nil {
			c.code("013")
			return
		}
		h.sendOTP(sub)
		c.code(model.CodeResendCompleted)
		return
	case "SUS":
		switch {
		case sub.Status != model.SubscriptionActive:
			refuse("Only active subscriptions can be suspended")
			return
		case sub.Resumable != "Y":
			refuse("Subscription is not resumable")
			return
		}
		sub.Status = model.SubscriptionSuspended
	case "RES":
		if sub.Status != model.SubscriptionSuspended {
			refuse("Only suspended subscriptions can be resumed")
			return
		}
		sub.Status = model.SubscriptionActive
	case "CAN":
		if sub.Status == model.SubscriptionCancelled {
			refuse("Subscription is already cancelled")
			return
		}
		sub.Status, sub.CancelledAt = model.SubscriptionCancelled, h.st.Now().UTC().Format("2006-01-02 15:04:05")
	case "STA":
		h.subscriptionStatus(c, sub)
		return
	default:
		c.code(model.CodeUndefinedTransType)
		return
	}
	if err := h.st.UpdateSubscription(sub); err != nil {
		c.code("013")
		return
	}
	c.reply(reply)
}

func (h *Handler) subscribe(c *call, ref string) {
	customer, nw := c.str("customer_number"), strings.ToUpper(c.str("nw"))
	cycle := strings.ToUpper(c.str("cycle"))
	switch {
	case customer == "":
		c.code(model.CodeMissingCustomerNumber)
		return
	case nw == "":
		c.code(model.CodeMissingNetwork)
		return
	case nw != "MTN" && nw != "VOD" && nw != "AIR":
		c.code(model.CodeUndefinedNetwork)
		return
	}
	pesewas, ok := c.amount(1)
	if !ok {
		return
	}
	if cycleNames[cycle] == "" || c.str("start_date") == "" || c.str("reference") == "" {
		c.code(model.CodeIncompleteParams)
		return
	}
	returnURL, ok := c.callbackURL("return_url")
	if !ok {
		return
	}
	yesNo := func(key, fallback string) string {
		if v := strings.ToUpper(c.str(key)); v != "" {
			return v[:1]
		}
		return fallback
	}

	sub := &model.Subscription{
		RunID: c.run.RunID, UniqRefID: ref, ServiceID: c.str("service_id"), CustomerNumber: customer, NW: nw,
		AmountPesewas: pesewas, Cycle: cycle, StartDate: c.str("start_date"), EndDate: c.str("end_date"),
		Reference: c.str("reference"), ReturnURL: returnURL, Resumable: yesNo("resumable", "N"),
		CycleSkip: yesNo("cycle_skip", "N"), ApplyPenalty: yesNo("apply_penalty", "N"), OTP: newOTP(),
	}
	if err := h.st.CreateSubscription(sub); err != nil {
		h.replyStoreError(c, err)
		return
	}
	h.sendOTP(sub)
	c.reply(autoDebitReply{ServiceID: c.fields["service_id"], ServiceName: serviceName, UniqRefID: ref, StatusCode: "000", Operation: "SUB"})
}

// sendOTP records the activation SMS Orchard sends to the customer.
func (h *Handler) sendOTP(sub *model.Subscription) {
	_ = h.st.RecordSMS(&model.SMS{
		RunID: sub.RunID, UniqueID: sub.UniqRefID, SenderID: "ORCHARD", Recipient: sub.CustomerNumber, MsgType: "T", Pages: 1,
		Body: fmt.Sprintf("%s: activate your %s auto debit of GHS %s for %s with code %s.",
			serviceName, strings.ToLower(cycleNames[sub.Cycle]), sub.Amount, sub.Reference, sub.OTP),
	})
}

func (h *Handler) subscriptionStatus(c *call, sub *model.Subscription) {
	debits, _ := h.st.SubscriptionDebits(sub.RunID, sub.UniqRefID)
	transactions := make([]map[string]string, 0, len(debits))
	prev := ""
	for _, d := range debits {
		transactions = append(transactions, map[string]string{
			"prev_schedule": prev, "processing_id": d.Exttrid, "trans_date": d.CreatedAt, "trans_id": d.TransID,
			"trans_msg": d.Message, "trans_ref": sub.Reference, "trans_status": d.TransStatus,
		})
		prev = d.CreatedAt
	}
	c.reply(map[string]any{
		"profile": map[string]any{
			"amount": sub.Amount, "callback_url": sub.ReturnURL, "cancel_date": sub.CancelledAt,
			"completed": sub.Status == model.SubscriptionCancelled, "customer_number": sub.CustomerNumber,
			"cycle": cycleNames[sub.Cycle], "cycle_skip": sub.CycleSkip, "end_date": sub.EndDate, "nw": sub.NW,
			"resumable": sub.Resumable, "service_id": c.fields["service_id"], "service_name": serviceName,
			"start_date": sub.StartDate, "status": sub.Status, "subscription_date": sub.SubscribedAt, "uniq_ref_id": sub.UniqRefID,
		},
		"transactions": transactions,
	})
}

// DebitSubscription takes one cycle's payment from an active mandate, as Orchard does on schedule.
func (h *Handler) DebitSubscription(run *model.Run, sub *model.Subscription) (*model.Transaction, error) {
	if sub.Status != model.SubscriptionActive {
		return nil, badRequest("only active subscriptions can be debited")
	}
	debits, _ := h.st.SubscriptionDebits(run.RunID, sub.UniqRefID)
	t := &model.Transaction{
		RunID: run.RunID, Exttrid: fmt.Sprintf("%s-%d", sub.UniqRefID, len(debits)+1), TransType: "AUD", Channel: model.ChannelAutoDebit,
		CustomerNumber: sub.CustomerNumber, NW: sub.NW, AmountPesewas: sub.AmountPesewas, Reference: sub.Reference,
		CallbackURL: sub.ReturnURL, Meta: map[string]string{"uniq_ref_id": sub.UniqRefID},
	}
	h.schedule(run, t)
	if err := h.st.CreateTransaction(t); err != nil {
		if err == store.ErrDuplicate {
			return nil, fmt.Errorf("debit reference %s already used", t.Exttrid)
		}
		return nil, err
	}
	return t, nil
}
