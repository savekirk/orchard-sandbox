package handler

import (
	"errors"
	"fmt"
	"slices"
	"strings"

	"github.com/savekirk/orchard-sandbox/internal/model"
	"github.com/savekirk/orchard-sandbox/internal/store"
)

// Networks accepted per transaction type. A network outside every list is undefined (090);
// a known network that the transaction type cannot use is invalid (029).
var (
	allNetworks     = []string{"MTN", "VOD", "AIR", "GLO", "BNK", "MAS", "VIS"}
	paymentNetworks = map[string][]string{
		"CTM": {"MTN", "VOD", "AIR", "MAS", "VIS"},
		"AUD": {"MTN", "VOD", "AIR"},
		"MTC": {"MTN", "VOD", "AIR", "BNK"},
		"RMT": {"MTN", "VOD", "AIR", "BNK"},
		"ATP": {"MTN", "VOD", "AIR", "GLO"},
	}
	remittanceFields = []string{"sender_name", "sender_number", "recipient_name", "recipient_address", "transf_amount",
		"ctry_origin_code", "sender_gender", "recipient_gender", "transf_curr_code"}
)

// SendRequest handles POST /sendRequest.
func (h *Handler) SendRequest(c *call) {
	transType := strings.ToUpper(c.str("trans_type"))
	exttrid := c.str("exttrid")
	switch {
	case transType == "":
		c.code(model.CodeMissingTransType)
	case exttrid == "":
		c.code(model.CodeMissingExttrid)
	case len(exttrid) > 30:
		c.code(model.CodeExttridTooLong30)
	case !c.timestamp(true):
	case transType == "AII":
		h.accountInquiry(c, exttrid)
	case transType == "BLP" || paymentNetworks[transType] != nil:
		h.payment(c, transType, exttrid)
	default:
		c.code(model.CodeUndefinedTransType)
	}
}

// bankCode validates bank_code, replying on failure.
func (c *call) bankCode() (string, bool) {
	code := strings.ToUpper(c.str("bank_code"))
	if code == "" {
		c.code(model.CodeMissingBankCode)
		return "", false
	}
	if _, ok := model.BankCodes[code]; !ok {
		c.code(model.CodeUndefinedBankCode)
		return "", false
	}
	return code, true
}

func (h *Handler) accountInquiry(c *call, exttrid string) {
	customer := c.str("customer_number")
	nw := strings.ToUpper(c.str("nw"))
	switch {
	case customer == "":
		c.code(model.CodeMissingCustomerNumber)
		return
	case nw == "":
		c.code(model.CodeMissingNetwork)
		return
	case nw != "BNK":
		c.code(model.CodeInvalidNetwork)
		return
	}
	bankCode, ok := c.bankCode()
	if !ok {
		return
	}
	if err := h.st.Reserve(c.run.RunID, store.RefExttrid, exttrid); err != nil {
		h.replyStoreError(c, err)
		return
	}

	if account, err := h.st.GetAccount(c.run.RunID, bankCode, customer); err == nil {
		if account.RespCode != model.CodeCompleted {
			c.code(account.RespCode)
			return
		}
		c.reply(model.AccountInquiryResponse{RespCode: model.CodeCompleted, RespDesc: model.Describe(model.CodeCompleted), Name: account.AccountName})
		return
	}
	if model.ScenarioFor(customer) == model.ScenarioInvalidAccount {
		c.code(model.CodeNoRecord)
		return
	}
	c.reply(model.AccountInquiryResponse{
		RespCode: model.CodeCompleted, RespDesc: model.Describe(model.CodeCompleted), Name: model.SimulatedAccountName(customer),
	})
}

func (h *Handler) payment(c *call, transType, exttrid string) {
	minimum := int64(1)
	if transType == "ATP" {
		minimum = 20 // Orchard's minimum airtime top-up is GHS 0.20
	}
	pesewas, ok := c.amount(minimum)
	if !ok {
		return
	}

	landingPage := c.str("landing_page_url")
	if landingPage == "" {
		landingPage = c.str("landing_page")
	}
	ghipss := transType == "CTM" && landingPage != ""

	customer := c.str("customer_number")
	if transType == "BLP" {
		if customer = c.str("account_number"); customer == "" {
			customer = c.str("acount_number") // spelling used in Orchard's own bill payment sample
		}
		if customer == "" {
			c.code(model.CodeMissingAccountNumber)
			return
		}
	} else if customer == "" && !ghipss {
		c.code(model.CodeMissingCustomerNumber)
		return
	}

	nw := strings.ToUpper(c.str("nw"))
	if nw == "" {
		c.code(model.CodeMissingNetwork)
		return
	}
	if allowed := paymentNetworks[transType]; allowed != nil && !slices.Contains(allowed, nw) {
		if slices.Contains(allNetworks, nw) {
			c.code(model.CodeInvalidNetwork)
		} else {
			c.code(model.CodeUndefinedNetwork)
		}
		return
	}

	callbackURL, ok := c.callbackURL("callback_url")
	if !ok {
		return
	}

	meta := map[string]string{}
	if nw == "BNK" {
		bankCode, ok := c.bankCode()
		if !ok {
			return
		}
		meta["bank_code"] = bankCode
		if c.str("recipient_name") == "" {
			c.code(model.CodeIncompleteParams)
			return
		}
	}
	if transType == "RMT" {
		for _, field := range remittanceFields {
			if c.str(field) == "" {
				c.code(model.CodeIncompleteParams)
				return
			}
			meta[field] = c.str(field)
		}
	}
	for _, field := range []string{"recipient_name", "nickname"} {
		if v := c.str(field); v != "" {
			meta[field] = v
		}
	}

	if transType == "AUD" {
		sub, err := h.st.ActiveSubscriptionFor(c.run.RunID, customer)
		if err != nil {
			c.code(model.CodeServiceNotOpen)
			return
		}
		meta["uniq_ref_id"] = sub.UniqRefID
	}

	t := &model.Transaction{
		RunID: c.run.RunID, Exttrid: exttrid, TransType: transType, Channel: model.ChannelAPI,
		CustomerNumber: customer, NW: nw, AmountPesewas: pesewas, Reference: c.str("reference"),
		CallbackURL: callbackURL, Meta: meta,
	}
	if ghipss {
		t.Channel = model.ChannelGHIPSS
		meta["landing_page"] = landingPage
	} else {
		h.schedule(c.run, t)
	}
	if err := h.st.CreateTransaction(t); err != nil {
		h.replyStoreError(c, err)
		return
	}

	if ghipss {
		c.reply(map[string]any{
			"resp_code": "000",
			"form_url":  h.baseURL(c.r) + ghipssFormPath,
			"form_details": map[string]any{
				"MerID": c.run.ServiceID, "AcqID": "315", "OrderID": checkoutCode(t.RunID, t.Exttrid),
				"MerRespURL":  h.baseURL(c.r) + checkoutPath,
				"PurchaseAmt": fmt.Sprintf("%012d", t.AmountPesewas), "PurchaseCurrency": 936, "PurchaseCurrencyExponent": 2,
				"CaptureFlag": "A", "Signature": "c2FuZGJveA==", "SignatureMethod": "SHA1", "Version": "1.0.0",
			},
		})
		return
	}
	c.code(model.CodeAccepted)
}

// schedule picks a payment's scenario and, in auto mode, when it settles.
func (h *Handler) schedule(run *model.Run, t *model.Transaction) {
	sc := model.ScenarioFor(t.CustomerNumber)
	t.Scenario = sc.Key
	if run.Settings.SettleMode == model.SettleAuto && !sc.NeverSettle {
		t.SettleAt = h.st.Now().UnixMilli() + int64(run.Settings.SettleDelayMs)
	}
}

// replyStoreError maps store failures to Orchard response codes.
func (h *Handler) replyStoreError(c *call, err error) {
	switch {
	case errors.Is(err, store.ErrDuplicate):
		c.code(model.CodeDuplicate)
	case errors.Is(err, store.ErrInsufficientBalance):
		switch strings.ToUpper(c.str("trans_type")) {
		case "ATP":
			c.code(model.CodeInsufficientAirtime)
		case "BLP":
			c.code(model.CodeInsufficientBillpay)
		case "SMS":
			c.code(model.CodeInsufficientSMS)
		default:
			c.code(model.CodeInsufficientBalance)
		}
	default:
		c.code("013")
	}
}
