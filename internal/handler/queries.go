package handler

import (
	"encoding/base64"
	"errors"
	"regexp"
	"strings"
	"unicode/utf8"

	"github.com/savekirk/orchard-sandbox/internal/model"
	"github.com/savekirk/orchard-sandbox/internal/store"
)

var ghanaCardPattern = regexp.MustCompile(`^GHA-\d{9}-\d$`)

// VerifyID handles POST /verifyID (Ghana Card verification).
func (h *Handler) VerifyID(c *call) {
	exttrid := c.str("exttrid")
	idNum := strings.ToUpper(c.str("id_num"))
	switch {
	case exttrid == "":
		c.code(model.CodeMissingExttrid)
		return
	case len(exttrid) > 20:
		c.code(model.CodeExttridTooLong20)
		return
	case !c.timestamp(false):
		return
	case c.str("trans_type") != "" && !strings.EqualFold(c.str("trans_type"), "AII"):
		c.code(model.CodeUndefinedTransType)
		return
	case idNum == "" || !strings.EqualFold(c.str("id_type"), "GCA") || c.str("image") == "":
		c.code(model.CodeIncompleteParams)
		return
	case !ghanaCardPattern.MatchString(idNum):
		c.code(model.CodeInvalidCustomerNumber)
		return
	}
	if _, err := base64.StdEncoding.DecodeString(c.str("image")); err != nil {
		c.code(model.CodeIncompleteParams)
		return
	}
	if err := h.st.Reserve(c.run.RunID, store.RefExttrid, exttrid); err != nil {
		h.replyStoreError(c, err)
		return
	}

	data := model.SimulatedCard(idNum, h.st.Now())
	if card, err := h.st.GetCard(c.run.RunID, idNum); err == nil {
		data = card.VerifyIDData
	}
	c.reply(model.VerifyIDResponse{RespCode: model.CodeCompleted, RespDesc: model.Describe(model.CodeCompleted), Data: &data})
}

// CheckWalletBalance handles POST /check_wallet_balance.
func (h *Handler) CheckWalletBalance(c *call) {
	switch tt := strings.ToUpper(c.str("trans_type")); {
	case tt == "":
		c.code(model.CodeMissingTransType)
		return
	case tt != "BLC":
		c.code(model.CodeUndefinedTransType)
		return
	case !c.timestamp(true):
		return
	}
	balances, err := h.st.WalletBalances(c.run.RunID)
	if err != nil {
		c.code("013")
		return
	}
	c.reply(balances)
}

// CheckTransaction handles POST /checkTransaction (transaction status check).
func (h *Handler) CheckTransaction(c *call) {
	exttrid := c.str("exttrid")
	switch tt := strings.ToUpper(c.str("trans_type")); {
	case exttrid == "":
		c.code(model.CodeMissingExttrid)
		return
	case tt == "":
		c.code(model.CodeMissingTransType)
		return
	case tt != "TSC":
		c.code(model.CodeUndefinedTransType)
		return
	}
	t, err := h.st.GetTransaction(c.run.RunID, exttrid)
	if errors.Is(err, store.ErrNotFound) {
		c.code(model.CodeNotFound)
		return
	}
	if err != nil {
		c.code("013")
		return
	}
	c.reply(model.TransactionStatusResponse{TransStatus: t.TransStatus, TransRef: t.Exttrid, TransID: t.TransID, Message: t.Message})
}

// SendSMS handles POST /sendSms.
func (h *Handler) SendSMS(c *call) {
	tt := strings.ToUpper(c.str("trans_type"))
	recipient, body, sender, uniqueID := c.str("recipient_number"), c.str("msg_body"), c.str("sender_id"), c.str("unique_id")
	msgType := strings.ToUpper(c.str("msg_type"))
	switch {
	case tt == "":
		c.code(model.CodeMissingTransType)
	case tt != "SMS":
		c.code(model.CodeUndefinedTransType)
	case recipient == "":
		c.code(model.CodeMissingRecipient)
	case body == "":
		c.code(model.CodeEmptySMSBody)
	case sender == "":
		c.code(model.CodeMissingSenderID)
	case len(sender) > 9:
		c.code(model.CodeSenderIDTooLong)
	case uniqueID == "":
		c.code(model.CodeMissingExttrid)
	case len(uniqueID) > 40:
		c.code(model.CodeSMSUniqueIDTooLong)
	case msgType != "" && msgType != "T" && msgType != "F":
		c.code(model.CodeIncompleteParams)
	default:
		if msgType == "" {
			msgType = "T"
		}
		msg := &model.SMS{
			RunID: c.run.RunID, UniqueID: uniqueID, SenderID: sender, Recipient: recipient, Body: body, MsgType: msgType,
			Pages: (utf8.RuneCountInString(body) + 159) / 160,
		}
		if err := h.st.ChargeSMS(msg); err != nil {
			h.replyStoreError(c, err)
			return
		}
		c.reply(model.CommonResponse{RespCode: model.CodeSMSQueued, RespDesc: model.Describe(model.CodeSMSQueued)})
	}
}
