// Package model holds Orchard wire types, sandbox domain types, and money helpers.
package model

import (
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
)

// CommonResponse is Orchard's standard {resp_code, resp_desc} body.
type CommonResponse struct {
	RespCode string `json:"resp_code"`
	RespDesc string `json:"resp_desc"`
}

// VerifyIDData is the identity record returned by /verifyID.
type VerifyIDData struct {
	Name           string `json:"name" yaml:"name"`
	Gender         string `json:"gender" yaml:"gender"`
	Verified       string `json:"verified" yaml:"verified"`
	CardValidStart string `json:"card_valid_start" yaml:"card_valid_start"`
	CardValidEnd   string `json:"card_valid_end" yaml:"card_valid_end"`
}

// VerifyIDResponse is the body returned by /verifyID.
type VerifyIDResponse struct {
	RespCode string        `json:"resp_code"`
	RespDesc string        `json:"resp_desc"`
	Data     *VerifyIDData `json:"data,omitempty"`
}

// AccountInquiryResponse is the body returned for trans_type AII on /sendRequest.
type AccountInquiryResponse struct {
	RespCode string `json:"resp_code"`
	RespDesc string `json:"resp_desc"`
	Name     string `json:"name"`
}

// Balances is the body returned by /check_wallet_balance, in GHS (sms_bal in units).
type Balances struct {
	SMSBal              float64 `json:"sms_bal"`
	PayoutBal           float64 `json:"payout_bal"`
	BillpayBal          float64 `json:"billpay_bal"`
	AvailableCollectBal float64 `json:"available_collect_bal"`
	AirtimeBal          float64 `json:"airtime_bal"`
	ActualCollectBal    float64 `json:"actual_collect_bal"`
}

// TransactionStatusResponse is the body returned by /checkTransaction.
type TransactionStatusResponse struct {
	TransStatus string `json:"trans_status"`
	TransRef    string `json:"trans_ref"`
	TransID     string `json:"trans_id"`
	Message     string `json:"message"`
}

// CallbackPayload is the body Orchard POSTs to a callback_url.
type CallbackPayload struct {
	TransID     string `json:"trans_id"`
	TransRef    string `json:"trans_ref"`
	TransStatus string `json:"trans_status"`
	Message     string `json:"message"`
}

// Wallet accounts held for every sandbox. SMS is counted in units, everything else in pesewas.
const (
	AccountAvailableCollect = "available_collect"
	AccountActualCollect    = "actual_collect"
	AccountPayout           = "payout"
	AccountBillpay          = "billpay"
	AccountAirtime          = "airtime"
	AccountSMS              = "sms"
)

// Accounts lists every wallet account in display order.
var Accounts = []string{AccountAvailableCollect, AccountActualCollect, AccountPayout, AccountBillpay, AccountAirtime, AccountSMS}

// Transaction statuses.
const (
	StatusPending    = "PENDING"
	StatusSuccessful = "SUCCESSFUL"
	StatusFailed     = "FAILED"
)

// Settlement modes for a sandbox.
const (
	SettleAuto   = "auto"
	SettleManual = "manual"
)

// Settings control how a sandbox behaves. Every run has its own copy.
type Settings struct {
	RequireAuth       bool   `json:"require_auth" yaml:"require_auth"`
	SettleMode        string `json:"settle_mode" yaml:"settle_mode"`
	SettleDelayMs     int    `json:"settle_delay_ms" yaml:"settle_delay_ms"`
	LatencyMs         int    `json:"latency_ms" yaml:"latency_ms"`
	CallbackAttempts  int    `json:"callback_attempts" yaml:"callback_attempts"`
	CallbackTimeoutMs int    `json:"callback_timeout_ms" yaml:"callback_timeout_ms"`
}

// Normalize fills invalid or missing values with defaults.
func (s Settings) Normalize() Settings {
	if s.SettleMode != SettleManual {
		s.SettleMode = SettleAuto
	}
	s.SettleDelayMs = max(s.SettleDelayMs, 0)
	s.LatencyMs = max(s.LatencyMs, 0)
	if s.CallbackAttempts <= 0 {
		s.CallbackAttempts = 3
	}
	if s.CallbackTimeoutMs <= 0 {
		s.CallbackTimeoutMs = 10000
	}
	return s
}

// Run is an isolated sandbox with its own credentials, balances and data.
type Run struct {
	RunID     string   `json:"run_id"`
	Name      string   `json:"name"`
	ClientKey string   `json:"client_key"`
	SecretKey string   `json:"secret_key"`
	ServiceID string   `json:"service_id"`
	Settings  Settings `json:"settings"`
	CreatedAt string   `json:"created_at"`
}

// Transaction is a payment accepted by the sandbox.
type Transaction struct {
	RunID          string            `json:"run_id"`
	Exttrid        string            `json:"exttrid"`
	TransID        string            `json:"trans_id"`
	TransType      string            `json:"trans_type"`
	Channel        string            `json:"channel"`
	CustomerNumber string            `json:"customer_number"`
	NW             string            `json:"nw"`
	AmountPesewas  int64             `json:"amount_pesewas"`
	Amount         string            `json:"amount"`
	Reference      string            `json:"reference"`
	CallbackURL    string            `json:"callback_url"`
	Status         string            `json:"status"`
	TransStatus    string            `json:"trans_status"`
	Message        string            `json:"message"`
	Scenario       string            `json:"scenario"`
	SettleAt       int64             `json:"settle_at,omitempty"`
	Meta           map[string]string `json:"meta,omitempty"`
	CreatedAt      string            `json:"created_at"`
	UpdatedAt      string            `json:"updated_at"`
}

// Transaction channels describe how a payment entered the sandbox.
const (
	ChannelAPI       = "api"
	ChannelCheckout  = "checkout"
	ChannelGHIPSS    = "ghipss"
	ChannelAutoDebit = "auto_debit"
)

// RequestLog records one call to an Orchard endpoint and the sandbox's reply.
type RequestLog struct {
	ID           int64             `json:"id"`
	RunID        string            `json:"run_id"`
	Method       string            `json:"method"`
	Path         string            `json:"path"`
	Operation    string            `json:"operation"`
	Ref          string            `json:"ref"`
	Headers      map[string]string `json:"headers"`
	Body         string            `json:"body"`
	Status       int               `json:"status"`
	ResponseBody string            `json:"response_body"`
	RespCode     string            `json:"resp_code"`
	DurationMs   int64             `json:"duration_ms"`
	CreatedAt    string            `json:"created_at"`
}

// Callback is a queued webhook delivery to a merchant's callback_url.
type Callback struct {
	ID            string `json:"id"`
	RunID         string `json:"run_id"`
	Exttrid       string `json:"exttrid"`
	URL           string `json:"url"`
	Payload       string `json:"payload"`
	State         string `json:"state"`
	Attempts      int    `json:"attempts"`
	MaxAttempts   int    `json:"max_attempts"`
	NextAttemptAt int64  `json:"next_attempt_at,omitempty"`
	LastStatus    int    `json:"last_status"`
	LastError     string `json:"last_error,omitempty"`
	CreatedAt     string `json:"created_at"`
	DeliveredAt   string `json:"delivered_at,omitempty"`
}

// Callback delivery states.
const (
	CallbackQueued     = "QUEUED"
	CallbackDelivering = "DELIVERING"
	CallbackDelivered  = "DELIVERED"
	CallbackFailed     = "FAILED"
)

// CallbackAttempt records one HTTP delivery attempt.
type CallbackAttempt struct {
	ID           int64  `json:"id"`
	CallbackID   string `json:"callback_id"`
	HTTPStatus   int    `json:"http_status"`
	ResponseBody string `json:"response_body"`
	Error        string `json:"error,omitempty"`
	DurationMs   int64  `json:"duration_ms"`
	AttemptedAt  string `json:"attempted_at"`
}

// Card is a Ghana Card identity profile returned by /verifyID.
type Card struct {
	IDNum        string `json:"id_num" yaml:"id_num"`
	VerifyIDData `yaml:",inline"`
}

// Account is a name-lookup profile returned by account inquiry (AII).
// A RespCode other than 027 makes the lookup fail with that code.
type Account struct {
	BankCode      string `json:"bank_code" yaml:"bank_code"`
	AccountNumber string `json:"account_number" yaml:"account_number"`
	AccountName   string `json:"account_name" yaml:"account_name"`
	RespCode      string `json:"resp_code,omitempty" yaml:"resp_code"`
}

// SMS is a message accepted by /sendSms, or an OTP the sandbox sent to a customer.
type SMS struct {
	ID        int64  `json:"id"`
	RunID     string `json:"run_id"`
	UniqueID  string `json:"unique_id"`
	SenderID  string `json:"sender_id"`
	Recipient string `json:"recipient"`
	Body      string `json:"body"`
	MsgType   string `json:"msg_type"`
	Pages     int    `json:"pages"`
	CreatedAt string `json:"created_at"`
}

// Subscription is an auto debit mandate created by /autoDebit SUB.
type Subscription struct {
	RunID          string `json:"run_id"`
	UniqRefID      string `json:"uniq_ref_id"`
	ServiceID      string `json:"service_id"`
	CustomerNumber string `json:"customer_number"`
	NW             string `json:"nw"`
	AmountPesewas  int64  `json:"amount_pesewas"`
	Amount         string `json:"amount"`
	Cycle          string `json:"cycle"`
	StartDate      string `json:"start_date"`
	EndDate        string `json:"end_date"`
	Reference      string `json:"reference"`
	ReturnURL      string `json:"return_url"`
	Resumable      string `json:"resumable"`
	CycleSkip      string `json:"cycle_skip"`
	ApplyPenalty   string `json:"apply_penalty"`
	Status         string `json:"status"`
	OTP            string `json:"otp"`
	SubscribedAt   string `json:"subscribed_at"`
	ActivatedAt    string `json:"activated_at"`
	CancelledAt    string `json:"cancelled_at"`
}

// Subscription statuses, as reported by the STA operation.
const (
	SubscriptionPending   = "Pending"
	SubscriptionActive    = "Active"
	SubscriptionSuspended = "Suspended"
	SubscriptionCancelled = "Cancelled"
)

// LedgerEntry is an immutable balance movement.
type LedgerEntry struct {
	ID           int64  `json:"id"`
	RunID        string `json:"run_id"`
	Account      string `json:"account"`
	Direction    string `json:"direction"`
	Amount       int64  `json:"amount"`
	BalanceAfter int64  `json:"balance_after"`
	Exttrid      string `json:"exttrid"`
	Note         string `json:"note"`
	CreatedAt    string `json:"created_at"`
}

// FailureRule injects a fault into matching Orchard requests.
type FailureRule struct {
	ID           string `json:"id"`
	RunID        string `json:"run_id"`
	Operation    string `json:"operation"`
	Exttrid      string `json:"exttrid"`
	Attempt      int    `json:"attempt"`
	Action       string `json:"action"`
	HTTPStatus   int    `json:"http_status,omitempty"`
	RespCode     string `json:"resp_code,omitempty"`
	DelayMs      int    `json:"delay_ms,omitempty"`
	Hold         bool   `json:"hold,omitempty"`
	ResponseBody string `json:"response_body,omitempty"`
	CreatedAt    string `json:"created_at"`
}

// Failure actions.
const (
	ActionHTTPStatus       = "http_status"
	ActionRespCode         = "resp_code"
	ActionDelay            = "delayed_response"
	ActionDrop             = "connection_failure"
	ActionDropAfterProcess = "connection_close_after_accept"
	ActionMalformedJSON    = "malformed_json"
	ActionMissingFields    = "missing_fields"
	ActionWrongTypes       = "wrong_types"
	ActionOversized        = "oversized_response"
	ActionRedirect         = "redirect"
)

// FailureActions lists every supported action.
var FailureActions = []string{
	ActionHTTPStatus, ActionRespCode, ActionDelay, ActionDrop, ActionDropAfterProcess,
	ActionMalformedJSON, ActionMissingFields, ActionWrongTypes, ActionOversized, ActionRedirect,
}

// Errors returned by ParseAmount, each mapping to an Orchard response code.
var (
	ErrAmountMissing  = errors.New("amount missing")
	ErrAmountInvalid  = errors.New("amount invalid")
	ErrAmountDecimals = errors.New("amount has more than two decimal places")
	ErrAmountTooLow   = errors.New("amount too low")
)

// AmountCode maps a ParseAmount error to its Orchard response code.
func AmountCode(err error) string {
	switch {
	case errors.Is(err, ErrAmountMissing):
		return CodeMissingAmount
	case errors.Is(err, ErrAmountDecimals):
		return CodeTwoDecimalPlaces
	case errors.Is(err, ErrAmountTooLow):
		return CodeAmountTooLow
	default:
		return CodeInvalidAmount
	}
}

// maxAmountMajor bounds amounts so pesewas never overflow int64.
const maxAmountMajor = 1_000_000_000_000

// ParseAmount converts a JSON amount (string or number) into exact pesewas.
func ParseAmount(val any) (int64, error) {
	var s string
	switch v := val.(type) {
	case nil:
		return 0, ErrAmountMissing
	case string:
		s = strings.TrimSpace(v)
	case json.Number:
		s = v.String()
	case float64:
		s = strconv.FormatFloat(v, 'f', -1, 64)
	default:
		return 0, ErrAmountInvalid
	}
	if s == "" {
		return 0, ErrAmountMissing
	}

	whole, frac, hasFrac := strings.Cut(s, ".")
	if whole == "" {
		whole = "0"
	}
	if !isDigits(whole) || (hasFrac && !isDigits(frac)) {
		return 0, ErrAmountInvalid
	}
	if len(frac) > 2 {
		return 0, ErrAmountDecimals
	}
	major, err := strconv.ParseInt(whole, 10, 64)
	if err != nil || major >= maxAmountMajor {
		return 0, ErrAmountInvalid
	}
	minor := int64(0)
	if frac != "" {
		minor, _ = strconv.ParseInt((frac + "0")[:2], 10, 64)
	}
	total := major*100 + minor
	if total <= 0 {
		return 0, ErrAmountTooLow
	}
	return total, nil
}

func isDigits(s string) bool {
	if s == "" {
		return false
	}
	for _, c := range s {
		if c < '0' || c > '9' {
			return false
		}
	}
	return true
}

// FormatAmount renders pesewas as a two-decimal GHS string.
func FormatAmount(pesewas int64) string {
	sign := ""
	if pesewas < 0 {
		sign, pesewas = "-", -pesewas
	}
	return fmt.Sprintf("%s%d.%02d", sign, pesewas/100, pesewas%100)
}

// Cedis converts pesewas to a GHS float for Orchard's balance response.
func Cedis(pesewas int64) float64 {
	return float64(pesewas) / 100
}
