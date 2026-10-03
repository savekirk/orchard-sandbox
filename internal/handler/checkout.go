package handler

import (
	"embed"
	"encoding/base64"
	"errors"
	"html/template"
	"net/http"
	"net/url"
	"strings"

	"github.com/savekirk/orchard-sandbox/internal/model"
	"github.com/savekirk/orchard-sandbox/internal/store"
)

const (
	checkoutPath   = "/payment_page"
	ghipssFormPath = "/ghipss/EcomPayment/RedirectAuthLink"
)

//go:embed templates/checkout.html
var templates embed.FS

var checkoutPage = template.Must(template.ParseFS(templates, "templates/checkout.html"))

// checkoutCode encodes a transaction reference for the hosted payment page URL.
func checkoutCode(runID, exttrid string) string {
	return base64.RawURLEncoding.EncodeToString([]byte(runID + "\n" + exttrid))
}

func parseCheckoutCode(code string) (runID, exttrid string, ok bool) {
	raw, err := base64.RawURLEncoding.DecodeString(strings.TrimRight(code, "="))
	if err != nil {
		return "", "", false
	}
	return strings.Cut(string(raw), "\n")
}

// ThirdPartyRequest handles POST /third_party_request (hosted checkout).
func (h *Handler) ThirdPartyRequest(c *call) {
	exttrid := c.str("exttrid")
	mode := strings.ToUpper(c.str("payment_mode"))
	switch {
	case exttrid == "":
		c.code(model.CodeMissingExttrid)
		return
	case len(exttrid) > 30:
		c.code(model.CodeExttridTooLong30)
		return
	case !c.timestamp(true):
		return
	}
	pesewas, ok := c.amount(1)
	if !ok {
		return
	}
	callbackURL, ok := c.callbackURL("callback_url")
	if !ok {
		return
	}
	landing := c.str("landing_page")
	if !validURL(landing) || (mode != "CRD" && mode != "MOM" && mode != "CRM") {
		c.code(model.CodeIncompleteParams)
		return
	}

	t := &model.Transaction{
		RunID: c.run.RunID, Exttrid: exttrid, TransType: "CTM", Channel: model.ChannelCheckout,
		AmountPesewas: pesewas, Reference: c.str("reference"), CallbackURL: callbackURL, Scenario: model.ScenarioSuccess.Key,
		Meta: map[string]string{"landing_page": landing, "payment_mode": mode},
	}
	if nick := c.str("nickname"); nick != "" {
		t.Meta["nickname"] = nick
	}
	if err := h.st.CreateTransaction(t); err != nil {
		h.replyStoreError(c, err)
		return
	}
	c.reply(map[string]any{
		"resp_code":    "000",
		"resp_desc":    "Passed",
		"redirect_url": h.baseURL(c.r) + checkoutPath + "?code=" + checkoutCode(t.RunID, t.Exttrid),
	})
}

type checkoutView struct {
	Code       string
	Merchant   string
	Txn        *model.Transaction
	Mode       string
	Error      string
	Result     string
	Scenarios  []model.Scenario
	BackToShop string
}

// PaymentPage renders the hosted checkout (GET /payment_page, GHIPSS form posts).
func (h *Handler) PaymentPage(w http.ResponseWriter, r *http.Request) {
	code := r.FormValue("code")
	if code == "" {
		code = r.FormValue("OrderID")
	}
	view, err := h.checkoutView(code)
	if err != nil {
		http.Error(w, "Payment session not found", http.StatusNotFound)
		return
	}
	h.renderCheckout(w, view)
}

// PaymentAction completes or cancels a hosted checkout (POST /payment_page).
func (h *Handler) PaymentAction(w http.ResponseWriter, r *http.Request) {
	view, err := h.checkoutView(r.FormValue("code"))
	if err != nil {
		http.Error(w, "Payment session not found", http.StatusNotFound)
		return
	}
	t := view.Txn
	if t.Status != model.StatusPending {
		h.renderCheckout(w, view)
		return
	}

	run, err := h.st.GetRun(t.RunID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	outcome := store.Outcome{Notify: true, Settings: run.Settings}
	switch r.FormValue("action") {
	case "cancel":
		outcome.Message = "FAILED: Customer cancelled the payment"
	default:
		card := r.FormValue("method") == "card"
		number, nw := strings.TrimPrefix(strings.TrimSpace(r.FormValue("msisdn")), "+"), strings.ToUpper(r.FormValue("network"))
		if card {
			number, nw = strings.ReplaceAll(r.FormValue("card_number"), " ", ""), "VIS"
			if strings.HasPrefix(number, "5") {
				nw = "MAS"
			}
		}
		if !isNumber(number) || (card && (len(number) < 12 || len(number) > 19)) || (!card && len(number) < 9) {
			view.Error = map[bool]string{true: "Enter a valid card number", false: "Enter a valid mobile money number"}[card]
			h.renderCheckout(w, view)
			return
		}
		sc := model.ScenarioFor(number)
		if card {
			number = number[:6] + strings.Repeat("*", len(number)-10) + number[len(number)-4:]
		}
		if err := h.st.SetPayer(t.RunID, t.Exttrid, number, nw, sc.Key); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		if sc.NeverSettle {
			h.finishCheckout(w, r, view.Code)
			return
		}
		outcome.Success, outcome.Message, outcome.Notify = sc.Success, sc.Message, !sc.NoCallback
	}
	if _, err := h.st.Settle(t.RunID, t.Exttrid, outcome); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	h.finishCheckout(w, r, view.Code)
}

// finishCheckout sends the customer back to the merchant's landing page with the payment status.
func (h *Handler) finishCheckout(w http.ResponseWriter, r *http.Request, code string) {
	view, err := h.checkoutView(code)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if landing := view.Txn.Meta["landing_page"]; landing != "" {
		http.Redirect(w, r, landingURL(landing, view.Txn), http.StatusSeeOther)
		return
	}
	h.renderCheckout(w, view)
}

func isNumber(s string) bool {
	return s != "" && strings.Trim(s, "0123456789") == ""
}

func landingURL(landing string, t *model.Transaction) string {
	u, err := url.Parse(landing)
	if err != nil {
		return landing
	}
	q := u.Query()
	q.Set("trans_status", t.TransStatus)
	q.Set("trans_ref", t.Exttrid)
	q.Set("trans_id", t.TransID)
	u.RawQuery = q.Encode()
	return u.String()
}

func (h *Handler) checkoutView(code string) (*checkoutView, error) {
	runID, exttrid, ok := parseCheckoutCode(code)
	if !ok {
		return nil, errors.New("invalid payment code")
	}
	t, err := h.st.GetTransaction(runID, exttrid)
	if err != nil {
		return nil, err
	}
	if t.Channel != model.ChannelCheckout && t.Channel != model.ChannelGHIPSS {
		return nil, store.ErrNotFound
	}
	view := &checkoutView{Code: code, Merchant: "Sandbox Merchant", Txn: t, Mode: t.Meta["payment_mode"], Scenarios: model.Scenarios}
	if t.Channel == model.ChannelGHIPSS {
		view.Mode = "CRD"
	}
	if t.Meta["nickname"] != "" {
		view.Merchant = t.Meta["nickname"]
	}
	if landing := t.Meta["landing_page"]; landing != "" && t.Status != model.StatusPending {
		view.BackToShop = landingURL(landing, t)
	}
	return view, nil
}

func (h *Handler) renderCheckout(w http.ResponseWriter, view *checkoutView) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	_ = checkoutPage.Execute(w, view)
}
