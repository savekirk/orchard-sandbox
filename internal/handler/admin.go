package handler

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"time"

	"github.com/savekirk/orchard-sandbox/internal/model"
	"github.com/savekirk/orchard-sandbox/internal/store"
)

// listLimit caps list endpoints so the dashboard stays fast.
const listLimit = 500

// orchardEndpoints are the paths the playground may call.
var orchardEndpoints = []string{"/sendRequest", "/verifyID", "/check_wallet_balance", "/checkTransaction", "/sendSms", "/third_party_request", "/autoDebit"}

// runFrom loads the run named in the path, writing a 404 if it does not exist.
func (h *Handler) runFrom(w http.ResponseWriter, r *http.Request) (*model.Run, bool) {
	run, err := h.st.GetRun(r.PathValue("run"))
	if err != nil {
		writeError(w, err)
		return nil, false
	}
	return run, true
}

// respond writes v, or the error if err is set.
func respond(w http.ResponseWriter, v any, err error) {
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, v)
}

// Info returns static reference data used by the dashboard.
func (h *Handler) Info(w http.ResponseWriter, r *http.Request) {
	banks := make([]map[string]string, 0, len(model.BankCodes))
	for code, name := range model.BankCodes {
		banks = append(banks, map[string]string{"code": code, "name": name})
	}
	slices.SortFunc(banks, func(a, b map[string]string) int { return strings.Compare(a["name"], b["name"]) })
	writeJSON(w, http.StatusOK, map[string]any{
		"base_url":        h.baseURL(r),
		"scenarios":       model.Scenarios,
		"card_scenarios":  model.CardScenarios,
		"bank_codes":      banks,
		"failure_actions": model.FailureActions,
		"endpoints":       orchardEndpoints,
	})
}

// Sink accepts and discards webhooks, so playground payments have a callback_url that answers 200.
func (h *Handler) Sink(w http.ResponseWriter, r *http.Request) {
	_, _ = io.Copy(io.Discard, r.Body)
	writeJSON(w, http.StatusOK, map[string]string{"status": "received"})
}

// ListRuns handles GET /mock/runs.
func (h *Handler) ListRuns(w http.ResponseWriter, r *http.Request) {
	runs, err := h.st.ListRuns()
	respond(w, runs, err)
}

// CreateRun handles POST /mock/runs. Settings given in the body override the default run's settings field by field.
func (h *Handler) CreateRun(w http.ResponseWriter, r *http.Request) {
	var body struct {
		store.NewRun
		Settings json.RawMessage `json:"settings"`
	}
	if r.ContentLength != 0 {
		if err := decodeJSON(r, &body); err != nil {
			writeError(w, err)
			return
		}
	}
	in := body.NewRun
	if len(body.Settings) > 0 {
		var settings model.Settings
		if def, err := h.st.GetRun(store.DefaultRunID); err == nil {
			settings = def.Settings
		}
		if err := json.Unmarshal(body.Settings, &settings); err != nil {
			writeError(w, badRequest("invalid settings: "+err.Error()))
			return
		}
		in.Settings = &settings
	}
	run, err := h.st.CreateRun(in)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, run)
}

// GetRun handles GET /mock/runs/{run}.
func (h *Handler) GetRun(w http.ResponseWriter, r *http.Request) {
	if run, ok := h.runFrom(w, r); ok {
		writeJSON(w, http.StatusOK, run)
	}
}

// DeleteRun handles DELETE /mock/runs/{run}.
func (h *Handler) DeleteRun(w http.ResponseWriter, r *http.Request) {
	respond(w, map[string]string{"status": "deleted"}, h.st.DeleteRun(r.PathValue("run")))
}

// ResetRun handles POST /mock/runs/{run}/reset.
func (h *Handler) ResetRun(w http.ResponseWriter, r *http.Request) {
	respond(w, map[string]string{"status": "reset"}, h.st.ResetRun(r.PathValue("run")))
}

// Overview handles GET /mock/runs/{run}/overview.
func (h *Handler) Overview(w http.ResponseWriter, r *http.Request) {
	run, ok := h.runFrom(w, r)
	if !ok {
		return
	}
	balances, err := h.st.WalletBalances(run.RunID)
	respond(w, map[string]any{"run": run, "balances": balances, "counts": h.st.Counts(run.RunID)}, err)
}

// UpdateSettings handles PUT /mock/runs/{run}/settings.
func (h *Handler) UpdateSettings(w http.ResponseWriter, r *http.Request) {
	run, ok := h.runFrom(w, r)
	if !ok {
		return
	}
	settings := run.Settings
	if err := decodeJSON(r, &settings); err != nil {
		writeError(w, err)
		return
	}
	updated, err := h.st.UpdateSettings(run.RunID, settings)
	respond(w, updated, err)
}

// Balances handles GET /mock/runs/{run}/balances.
func (h *Handler) Balances(w http.ResponseWriter, r *http.Request) {
	b, err := h.st.WalletBalances(r.PathValue("run"))
	respond(w, b, err)
}

// SetBalances handles PUT /mock/runs/{run}/balances. Fields use Orchard's balance names, in GHS.
func (h *Handler) SetBalances(w http.ResponseWriter, r *http.Request) {
	run, ok := h.runFrom(w, r)
	if !ok {
		return
	}
	var in map[string]json.Number
	if err := decodeJSON(r, &in); err != nil {
		writeError(w, err)
		return
	}
	target := map[string]int64{}
	for field, value := range in {
		account, ok := strings.CutSuffix(field, "_bal")
		if !ok || !slices.Contains(model.Accounts, account) {
			writeError(w, badRequest("unknown balance "+field))
			return
		}
		if account == model.AccountSMS {
			n, err := value.Int64()
			if err != nil || n < 0 {
				writeError(w, badRequest("sms_bal must be a whole number of units"))
				return
			}
			target[account] = n
			continue
		}
		amount, err := model.ParseAmount(value.String())
		if err != nil && !errors.Is(err, model.ErrAmountTooLow) {
			writeError(w, badRequest(field+": "+err.Error()))
			return
		}
		target[account] = amount
	}
	if err := h.st.SetBalances(run.RunID, target); err != nil {
		writeError(w, err)
		return
	}
	b, err := h.st.WalletBalances(run.RunID)
	respond(w, b, err)
}

// Ledger handles GET /mock/runs/{run}/ledger.
func (h *Handler) Ledger(w http.ResponseWriter, r *http.Request) {
	entries, err := h.st.Ledger(r.PathValue("run"), listLimit)
	respond(w, entries, err)
}

// Requests handles GET /mock/runs/{run}/requests (filter with ?ref=).
func (h *Handler) Requests(w http.ResponseWriter, r *http.Request) {
	logs, err := h.st.ListRequests(r.PathValue("run"), r.URL.Query().Get("ref"), listLimit)
	respond(w, logs, err)
}

// Submissions handles GET /mock/runs/{run}/transactions/{exttrid}/submissions: every request that used the reference.
func (h *Handler) Submissions(w http.ResponseWriter, r *http.Request) {
	logs, err := h.st.ListRequests(r.PathValue("run"), r.PathValue("exttrid"), listLimit)
	if err == nil {
		slices.Reverse(logs)
	}
	respond(w, logs, err)
}

// Transactions handles GET /mock/runs/{run}/transactions.
func (h *Handler) Transactions(w http.ResponseWriter, r *http.Request) {
	txns, err := h.st.ListTransactions(r.PathValue("run"), listLimit)
	respond(w, txns, err)
}

// Transaction handles GET /mock/runs/{run}/transactions/{exttrid}.
func (h *Handler) Transaction(w http.ResponseWriter, r *http.Request) {
	t, err := h.st.GetTransaction(r.PathValue("run"), r.PathValue("exttrid"))
	respond(w, t, err)
}

// Resolve handles POST /mock/runs/{run}/transactions/{exttrid}/resolve with {"status": "SUCCESSFUL"|"FAILED", "message": "..."}.
func (h *Handler) Resolve(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Status  string `json:"status"`
		Message string `json:"message"`
	}
	if r.ContentLength != 0 {
		if err := decodeJSON(r, &in); err != nil {
			writeError(w, err)
			return
		}
	}
	status := strings.ToUpper(in.Status)
	if status != model.StatusFailed && status != "" && status != model.StatusSuccessful {
		writeError(w, badRequest(`status must be "SUCCESSFUL" or "FAILED"`))
		return
	}
	t, err := h.eng.Resolve(r.PathValue("run"), r.PathValue("exttrid"), status != model.StatusFailed, in.Message)
	respond(w, t, err)
}

// Callbacks handles GET /mock/runs/{run}/callbacks.
func (h *Handler) Callbacks(w http.ResponseWriter, r *http.Request) {
	cbs, err := h.st.ListCallbacks(r.PathValue("run"), listLimit)
	respond(w, cbs, err)
}

// CallbackAttempts handles GET /mock/runs/{run}/callbacks/{id}/attempts.
func (h *Handler) CallbackAttempts(w http.ResponseWriter, r *http.Request) {
	attempts, err := h.st.CallbackAttempts(r.PathValue("run"), r.PathValue("id"))
	respond(w, attempts, err)
}

// RetryCallback handles POST /mock/runs/{run}/callbacks/{id}/retry.
func (h *Handler) RetryCallback(w http.ResponseWriter, r *http.Request) {
	respond(w, map[string]string{"status": "queued"}, h.st.RetryCallback(r.PathValue("run"), r.PathValue("id")))
}

// Messages handles GET /mock/runs/{run}/sms.
func (h *Handler) Messages(w http.ResponseWriter, r *http.Request) {
	msgs, err := h.st.ListSMS(r.PathValue("run"), listLimit)
	respond(w, msgs, err)
}

// Cards handles GET /mock/runs/{run}/cards.
func (h *Handler) Cards(w http.ResponseWriter, r *http.Request) {
	cards, err := h.st.ListCards(r.PathValue("run"))
	respond(w, cards, err)
}

// SaveCard handles POST /mock/runs/{run}/cards.
func (h *Handler) SaveCard(w http.ResponseWriter, r *http.Request) {
	run, ok := h.runFrom(w, r)
	if !ok {
		return
	}
	var card model.Card
	if err := decodeJSON(r, &card); err != nil {
		writeError(w, err)
		return
	}
	card.IDNum = strings.ToUpper(strings.TrimSpace(card.IDNum))
	if !ghanaCardPattern.MatchString(card.IDNum) {
		writeError(w, badRequest("id_num must look like GHA-123456789-0"))
		return
	}
	if card.Verified != "false" {
		card.Verified = "true"
	}
	respond(w, card, h.st.SaveCard(run.RunID, card))
}

// DeleteCard handles DELETE /mock/runs/{run}/cards/{id}.
func (h *Handler) DeleteCard(w http.ResponseWriter, r *http.Request) {
	respond(w, map[string]string{"status": "deleted"}, h.st.DeleteCard(r.PathValue("run"), r.PathValue("id")))
}

// Accounts handles GET /mock/runs/{run}/accounts.
func (h *Handler) Accounts(w http.ResponseWriter, r *http.Request) {
	accounts, err := h.st.ListAccounts(r.PathValue("run"))
	respond(w, accounts, err)
}

// SaveAccount handles POST /mock/runs/{run}/accounts.
func (h *Handler) SaveAccount(w http.ResponseWriter, r *http.Request) {
	run, ok := h.runFrom(w, r)
	if !ok {
		return
	}
	var a model.Account
	if err := decodeJSON(r, &a); err != nil {
		writeError(w, err)
		return
	}
	a.BankCode = strings.ToUpper(strings.TrimSpace(a.BankCode))
	if _, ok := model.BankCodes[a.BankCode]; !ok || a.AccountNumber == "" {
		writeError(w, badRequest("bank_code must be an Orchard bank code and account_number is required"))
		return
	}
	respond(w, a, h.st.SaveAccount(run.RunID, a))
}

// DeleteAccount handles DELETE /mock/runs/{run}/accounts/{bank}/{number}.
func (h *Handler) DeleteAccount(w http.ResponseWriter, r *http.Request) {
	respond(w, map[string]string{"status": "deleted"}, h.st.DeleteAccount(r.PathValue("run"), r.PathValue("bank"), r.PathValue("number")))
}

// Subscriptions handles GET /mock/runs/{run}/subscriptions.
func (h *Handler) Subscriptions(w http.ResponseWriter, r *http.Request) {
	subs, err := h.st.ListSubscriptions(r.PathValue("run"))
	respond(w, subs, err)
}

// Debit handles POST /mock/runs/{run}/subscriptions/{ref}/debit: run one billing cycle now.
func (h *Handler) Debit(w http.ResponseWriter, r *http.Request) {
	run, ok := h.runFrom(w, r)
	if !ok {
		return
	}
	sub, err := h.st.GetSubscription(run.RunID, r.PathValue("ref"))
	if err != nil {
		writeError(w, err)
		return
	}
	t, err := h.DebitSubscription(run, sub)
	respond(w, t, err)
}

// FailureRules handles GET /mock/runs/{run}/failures.
func (h *Handler) FailureRules(w http.ResponseWriter, r *http.Request) {
	rules, err := h.st.ListFailureRules(r.PathValue("run"))
	respond(w, rules, err)
}

// AddFailureRule handles POST /mock/runs/{run}/failures.
func (h *Handler) AddFailureRule(w http.ResponseWriter, r *http.Request) {
	run, ok := h.runFrom(w, r)
	if !ok {
		return
	}
	var rule model.FailureRule
	if err := decodeJSON(r, &rule); err != nil {
		writeError(w, err)
		return
	}
	if !slices.Contains(model.FailureActions, rule.Action) {
		writeError(w, badRequest("action must be one of "+strings.Join(model.FailureActions, ", ")))
		return
	}
	if rule.Action == model.ActionRespCode && rule.RespCode == "" {
		writeError(w, badRequest("resp_code is required for the resp_code action"))
		return
	}
	rule.RunID = run.RunID
	respond(w, rule, h.st.AddFailureRule(&rule))
}

// DeleteFailureRule handles DELETE /mock/runs/{run}/failures/{id}.
func (h *Handler) DeleteFailureRule(w http.ResponseWriter, r *http.Request) {
	respond(w, map[string]string{"status": "deleted"}, h.st.DeleteFailureRule(r.PathValue("run"), r.PathValue("id")))
}

// Holds handles GET /mock/runs/{run}/holds: requests paused by a hold rule.
func (h *Handler) Holds(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, h.listHolds(r.PathValue("run")))
}

// ReleaseHold handles POST /mock/runs/{run}/holds/{id}/release.
func (h *Handler) ReleaseHold(w http.ResponseWriter, r *http.Request) {
	if !h.releaseHold(r.PathValue("run"), r.PathValue("id")) {
		writeError(w, store.ErrNotFound)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "released"})
}

// Try handles POST /mock/runs/{run}/try: sign a request with the run's keys and send it through the Orchard API.
func (h *Handler) Try(w http.ResponseWriter, r *http.Request) {
	run, ok := h.runFrom(w, r)
	if !ok {
		return
	}
	var in struct {
		Path string          `json:"path"`
		Body json.RawMessage `json:"body"`
	}
	if err := decodeJSON(r, &in); err != nil {
		writeError(w, err)
		return
	}
	if !slices.Contains(orchardEndpoints, in.Path) {
		writeError(w, badRequest("path must be one of "+strings.Join(orchardEndpoints, ", ")))
		return
	}
	body := bytes.TrimSpace(in.Body)
	auth := Sign(run.ClientKey, run.SecretKey, body)

	req := httptest.NewRequestWithContext(r.Context(), http.MethodPost, in.Path, bytes.NewReader(body))
	req.Host, req.TLS = r.Host, r.TLS
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", auth)
	for _, key := range []string{"X-Forwarded-Proto", "X-Forwarded-Host"} {
		req.Header.Set(key, r.Header.Get(key))
	}
	rec := httptest.NewRecorder()

	start := time.Now()
	func() {
		defer func() {
			if p := recover(); p != nil && p != http.ErrAbortHandler {
				panic(p)
			}
		}()
		h.api.ServeHTTP(rec, req)
	}()
	resp := rec.Result()
	respBody, _ := io.ReadAll(resp.Body)

	writeJSON(w, http.StatusOK, map[string]any{
		"request":     map[string]string{"method": http.MethodPost, "path": in.Path, "authorization": auth, "body": string(body)},
		"status":      resp.StatusCode,
		"body":        string(respBody),
		"duration_ms": time.Since(start).Milliseconds(),
	})
}

// Events handles GET /mock/events: a Server-Sent Events stream of change notifications (filter with ?run=).
func (h *Handler) Events(w http.ResponseWriter, r *http.Request) {
	rc := http.NewResponseController(w)
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("X-Accel-Buffering", "no")

	events, unsubscribe := h.st.Subscribe()
	defer unsubscribe()
	runID := r.URL.Query().Get("run")

	fmt.Fprint(w, "retry: 2000\n\n")
	if rc.Flush() != nil {
		return
	}
	keepalive := time.NewTicker(20 * time.Second)
	defer keepalive.Stop()
	for {
		select {
		case <-r.Context().Done():
			return
		case <-keepalive.C:
			fmt.Fprint(w, ": keepalive\n\n")
		case e := <-events:
			if runID != "" && e.RunID != runID {
				continue
			}
			data, _ := json.Marshal(e)
			fmt.Fprintf(w, "data: %s\n\n", data)
		}
		if rc.Flush() != nil {
			return
		}
	}
}
