package server_test

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/savekirk/orchard-sandbox/internal/model"
)

func TestHostedCheckout(t *testing.T) {
	s := newSandbox(t)
	cb := newReceiver(t)
	got := s.post("/third_party_request", map[string]any{
		"service_id": 1234, "exttrid": "CHK-1", "amount": "25.00", "reference": "Order 7", "callback_url": cb.URL,
		"landing_page": "https://shop.test/done?order=7", "payment_mode": "CRM", "ts": ts(),
	})
	expectCode(t, got, "000")
	redirect, _ := url.Parse(got["redirect_url"].(string))
	if redirect.Host != "sandbox.test" || redirect.Path != "/payment_page" || redirect.Query().Get("code") == "" {
		t.Fatalf("unexpected redirect_url %v", got["redirect_url"])
	}

	page := httptest.NewRecorder()
	s.srv.Handler().ServeHTTP(page, httptest.NewRequest(http.MethodGet, redirect.RequestURI(), nil))
	if page.Code != http.StatusOK || !strings.Contains(page.Body.String(), "25.00") {
		t.Fatalf("checkout page did not render: %d", page.Code)
	}

	form := url.Values{"code": {redirect.Query().Get("code")}, "action": {"pay"}, "method": {"momo"}, "network": {"MTN"}, "msisdn": {"0240000001"}}
	req := httptest.NewRequest(http.MethodPost, "/payment_page", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec := httptest.NewRecorder()
	s.srv.Handler().ServeHTTP(rec, req)

	landing, _ := url.Parse(rec.Header().Get("Location"))
	if rec.Code != http.StatusSeeOther || landing.Host != "shop.test" || landing.Query().Get("order") != "7" ||
		landing.Query().Get("trans_status") != model.TransStatusFailed || landing.Query().Get("trans_ref") != "CHK-1" {
		t.Fatalf("customer should return to the landing page with the result, got %d %s", rec.Code, landing)
	}
	s.tick()
	if p := cb.received(); len(p) != 1 || p[0]["trans_status"] != model.TransStatusFailed {
		t.Fatalf("declined checkout should notify the callback, got %v", p)
	}
	if txn := s.transaction("CHK-1"); txn.CustomerNumber != "0240000001" || txn.NW != "MTN" {
		t.Fatalf("payer details not recorded: %+v", txn)
	}
}

func TestGHIPSSCardPayment(t *testing.T) {
	s := newSandbox(t)
	got := s.post("/sendRequest", payment("CTM", "GIP-1", map[string]any{"customer_number": nil, "landing_page": "https://shop.test/return"}))
	expectCode(t, got, "000")
	details := got["form_details"].(map[string]any)
	if got["form_url"] != "http://sandbox.test/ghipss/EcomPayment/RedirectAuthLink" || details["PurchaseAmt"] != "000000001000" {
		t.Fatalf("unexpected GHIPSS response %v", got)
	}

	form := url.Values{"OrderID": {details["OrderID"].(string)}}
	req := httptest.NewRequest(http.MethodPost, "/ghipss/EcomPayment/RedirectAuthLink", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec := httptest.NewRecorder()
	s.srv.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "Card number") {
		t.Fatalf("GHIPSS form post should show the card page, got %d", rec.Code)
	}
}

func TestAutoDebitLifecycle(t *testing.T) {
	s := newSandbox(t)
	cb := newReceiver(t)
	op := func(operation string, extra map[string]any) map[string]any {
		body := map[string]any{"service_id": 1234, "uniq_ref_id": "SUB-1", "operation": operation}
		for k, v := range extra {
			body[k] = v
		}
		return s.post("/autoDebit", body)
	}

	sub := op("SUB", map[string]any{"customer_number": "233241234567", "amount": "5.00", "nw": "MTN", "cycle": "MON",
		"start_date": "2026-01-01", "reference": "Loan", "return_url": cb.URL, "resumable": "Y"})
	if sub["status_code"] != "000" || sub["operation"] != "SUB" {
		t.Fatalf("subscription should be accepted, got %v", sub)
	}
	expectCode(t, op("SUB", map[string]any{"customer_number": "233241234567", "amount": "5.00", "nw": "MTN", "cycle": "MON",
		"start_date": "2026-01-01", "reference": "Loan", "return_url": cb.URL}), "021")

	msgs, _ := s.st.ListSMS("default", 10)
	mandate, _ := s.st.GetSubscription("default", "SUB-1")
	if len(msgs) != 1 || !strings.Contains(msgs[0].Body, mandate.OTP) {
		t.Fatalf("the activation code should be sent to the customer, got %v", msgs)
	}

	if got := op("OTP", map[string]any{"auth_code": "00000x"}); got["status_code"] != "001" {
		t.Fatalf("a wrong code must be refused, got %v", got)
	}
	if got := op("OTP", map[string]any{"auth_code": mandate.OTP}); got["status_code"] != "000" || got["activated_at"] == "" {
		t.Fatalf("activation should succeed, got %v", got)
	}
	s.tick()
	if p := cb.received(); len(p) != 1 || p[0]["operation"] != "OTP" {
		t.Fatalf("activation should be confirmed on return_url, got %v", p)
	}

	if got := op("SUS", nil); got["status_code"] != "000" {
		t.Fatalf("suspend failed: %v", got)
	}
	if code := s.admin(http.MethodPost, "/mock/runs/default/subscriptions/SUB-1/debit", nil, nil); code != http.StatusBadRequest {
		t.Fatalf("suspended mandates cannot be debited, got %d", code)
	}
	op("RES", nil)

	var debit model.Transaction
	if code := s.admin(http.MethodPost, "/mock/runs/default/subscriptions/SUB-1/debit", nil, &debit); code != http.StatusOK || debit.TransType != "AUD" {
		t.Fatalf("debit cycle failed: %d %+v", code, debit)
	}
	expectCode(t, s.post("/sendRequest", payment("AUD", "AUD-API-1", map[string]any{"customer_number": "233241234567"})), "015")
	s.tick()

	status := op("STA", nil)
	profile, txns := status["profile"].(map[string]any), status["transactions"].([]any)
	if profile["status"] != "Active" || profile["cycle"] != "Monthly" || len(txns) != 2 {
		t.Fatalf("unexpected status %v", status)
	}
	if b := s.balances(); b.AvailableCollectBal != 75015 {
		t.Fatalf("both debits (5.00 + 10.00) should be collected, got %v", b.AvailableCollectBal)
	}

	op("CAN", nil)
	if got := op("RES", nil); got["status_code"] != "001" {
		t.Fatalf("cancelled mandates cannot be resumed, got %v", got)
	}
	expectCode(t, s.post("/autoDebit", map[string]any{"service_id": 1234, "uniq_ref_id": "NOPE", "operation": "STA"}), "067")
}

func TestRunsAreIsolated(t *testing.T) {
	s := newSandbox(t)
	var run model.Run
	if code := s.admin(http.MethodPost, "/mock/runs", map[string]any{"run_id": "ci-42", "settings": map[string]any{"require_auth": true, "settle_mode": "manual"}}, &run); code != http.StatusCreated {
		t.Fatalf("create run: %d", code)
	}
	if run.ClientKey == "" || run.SecretKey == "" || run.ServiceID != "1234" {
		t.Fatalf("run should get its own credentials: %+v", run)
	}

	var partial model.Run
	s.admin(http.MethodPost, "/mock/runs", map[string]any{"run_id": "partial", "settings": map[string]any{"settle_mode": "manual"}}, &partial)
	if !partial.Settings.RequireAuth || partial.Settings.SettleMode != model.SettleManual || partial.Settings.CallbackAttempts != 3 {
		t.Fatalf("unspecified settings should come from the default run: %+v", partial.Settings)
	}

	expectCode(t, s.postAs(&run, "/sendRequest", payment("MTC", "ISO-1", map[string]any{"amount": "1000"})), "015")
	s.tick()
	if b, _ := s.st.WalletBalances("ci-42"); b.PayoutBal != 49000 {
		t.Fatalf("payout should hit the run's wallet, got %v", b.PayoutBal)
	}
	if s.balances().PayoutBal != 50000 {
		t.Fatal("the default run must not be affected")
	}
	if txn, _ := s.st.GetTransaction("ci-42", "ISO-1"); txn.Status != model.StatusPending {
		t.Fatalf("manual settle mode should keep the payment pending, got %s", txn.Status)
	}
	expectCode(t, s.post("/sendRequest", payment("MTC", "ISO-1", nil)), "015") // references are per run

	if code := s.admin(http.MethodPost, "/mock/runs/ci-42/reset", nil, nil); code != http.StatusOK {
		t.Fatalf("reset: %d", code)
	}
	if b, _ := s.st.WalletBalances("ci-42"); b.PayoutBal != 50000 {
		t.Fatalf("reset should restore opening balances, got %v", b.PayoutBal)
	}
	if code := s.admin(http.MethodDelete, "/mock/runs/ci-42", nil, nil); code != http.StatusOK {
		t.Fatalf("delete: %d", code)
	}
	if code := s.admin(http.MethodDelete, "/mock/runs/default", nil, nil); code != http.StatusBadRequest {
		t.Fatalf("deleting the default run should be refused, got %d", code)
	}
}

func TestRunHeaderRoutesUnsignedRequests(t *testing.T) {
	s := newSandbox(t, func(st *model.Settings) { st.RequireAuth = false })
	s.admin(http.MethodPost, "/mock/runs", map[string]any{"run_id": "unsigned"}, nil)

	req := httptest.NewRequest(http.MethodPost, "/sendRequest", strings.NewReader(`{"service_id":1234,"trans_type":"CTM","exttrid":"H-1","amount":"1","customer_number":"0241234567","nw":"MTN","callback_url":"https://x.test","ts":"`+ts()+`"}`))
	req.Header.Set("X-Sandbox-Run", "unsigned")
	expectCode(t, s.do(req), "015")
	if _, err := s.st.GetTransaction("unsigned", "H-1"); err != nil {
		t.Fatal("request should be stored in the run named by X-Sandbox-Run")
	}
}

func TestManualResolution(t *testing.T) {
	s := newSandbox(t, func(st *model.Settings) { st.SettleMode = model.SettleManual })
	cb := newReceiver(t)
	expectCode(t, s.post("/sendRequest", payment("MTC", "MAN-1", map[string]any{"amount": "40", "callback_url": cb.URL})), "015")
	s.tick()
	if s.transaction("MAN-1").Status != model.StatusPending {
		t.Fatal("manual mode must not settle automatically")
	}

	var txn model.Transaction
	s.admin(http.MethodPost, "/mock/runs/default/transactions/MAN-1/resolve", map[string]any{"status": "FAILED", "message": "FAILED: Wallet barred"}, &txn)
	s.tick()
	if txn.Status != model.StatusFailed || s.balances().PayoutBal != 50000 {
		t.Fatalf("failed payout should be reversed: %+v balance=%v", txn, s.balances().PayoutBal)
	}
	if p := cb.received(); len(p) != 1 || p[0]["message"] != "FAILED: Wallet barred" {
		t.Fatalf("resolution should notify the callback, got %v", p)
	}

	s.admin(http.MethodPost, "/mock/runs/default/transactions/MAN-1/resolve", map[string]any{"status": "SUCCESSFUL"}, &txn)
	if txn.Status != model.StatusFailed {
		t.Fatal("final transactions must not change again")
	}
}

func TestCallbackRetries(t *testing.T) {
	s := newSandbox(t)
	cb := newReceiver(t)
	cb.status = http.StatusInternalServerError

	expectCode(t, s.post("/sendRequest", payment("CTM", "RETRY-1", map[string]any{"callback_url": cb.URL})), "015")
	s.tick()
	if got := s.callbacks()[0]; got.State != model.CallbackQueued || got.Attempts != 1 || got.LastStatus != 500 {
		t.Fatalf("failed delivery should be retried later: %+v", got)
	}
	s.tick()
	if len(cb.received()) != 1 {
		t.Fatal("retry must wait for the backoff")
	}
	for range 2 {
		s.advance(time.Minute)
		s.tick()
	}
	if got := s.callbacks()[0]; got.State != model.CallbackFailed || got.Attempts != 3 {
		t.Fatalf("delivery should give up after 3 attempts: %+v", got)
	}

	cb.status = http.StatusOK
	s.admin(http.MethodPost, "/mock/runs/default/callbacks/"+s.callbacks()[0].ID+"/retry", nil, nil)
	s.tick()
	if got := s.callbacks()[0]; got.State != model.CallbackDelivered || got.Attempts != 4 {
		t.Fatalf("manual retry should deliver: %+v", got)
	}
	var attempts []model.CallbackAttempt
	s.admin(http.MethodGet, "/mock/runs/default/callbacks/"+s.callbacks()[0].ID+"/attempts", nil, &attempts)
	if len(attempts) != 4 || attempts[3].HTTPStatus != 200 {
		t.Fatalf("every attempt should be recorded: %+v", attempts)
	}
}

func TestFailureInjection(t *testing.T) {
	s := newSandbox(t)
	addRule := func(rule map[string]any) {
		if code := s.admin(http.MethodPost, "/mock/runs/default/failures", rule, nil); code != http.StatusOK {
			t.Fatalf("add rule: %d", code)
		}
	}

	addRule(map[string]any{"operation": "sendRequest:CTM", "exttrid": "F-1", "attempt": 1, "action": "http_status", "http_status": 504})
	raw := httptest.NewRecorder()
	body := payment("CTM", "F-1", nil)
	req := httptest.NewRequest(http.MethodPost, "/sendRequest", strings.NewReader(mustJSON(body)))
	req.Header.Set("Authorization", signBody(s, mustJSON(body)))
	s.srv.Handler().ServeHTTP(raw, req)
	if raw.Code != http.StatusGatewayTimeout {
		t.Fatalf("first attempt should fail with 504, got %d", raw.Code)
	}
	s.post("/checkTransaction", map[string]any{"service_id": 1234, "trans_type": "TSC", "exttrid": "F-1"})
	expectCode(t, s.post("/sendRequest", body), "015") // status checks do not count as payment attempts

	addRule(map[string]any{"operation": "check_wallet_balance", "action": "resp_code", "resp_code": "100"})
	expectCode(t, s.post("/check_wallet_balance", map[string]any{"service_id": 1234, "trans_type": "BLC", "ts": ts()}), "100")

	addRule(map[string]any{"operation": "verifyID", "action": "connection_failure"})
	srv := httptest.NewServer(s.srv.Handler())
	defer srv.Close()
	drop, _ := http.NewRequest(http.MethodPost, srv.URL+"/verifyID", strings.NewReader("{}"))
	drop.Header.Set("Authorization", signBody(s, "{}"))
	resp, err := http.DefaultClient.Do(drop)
	if err == nil {
		resp.Body.Close()
		t.Fatal("connection_failure should drop the connection")
	}

	if code := s.admin(http.MethodPost, "/mock/runs/default/failures", map[string]any{"action": "explode"}, nil); code != http.StatusBadRequest {
		t.Fatalf("unknown actions should be rejected, got %d", code)
	}
}

func TestHeldResponseIsReleased(t *testing.T) {
	s := newSandbox(t)
	s.admin(http.MethodPost, "/mock/runs/default/failures", map[string]any{"operation": "sendRequest", "action": "delayed_response", "hold": true}, nil)

	done := make(chan map[string]any)
	go func() { done <- s.post("/sendRequest", payment("CTM", "HOLD-1", nil)) }()

	var holds []map[string]any
	for range 100 {
		s.admin(http.MethodGet, "/mock/runs/default/holds", nil, &holds)
		if len(holds) == 1 {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if len(holds) != 1 {
		t.Fatal("request should be held")
	}
	s.admin(http.MethodPost, "/mock/runs/default/holds/"+holds[0]["id"].(string)+"/release", nil, nil)
	expectCode(t, <-done, "015")
}

func TestPlaygroundSignsRequests(t *testing.T) {
	s := newSandbox(t)
	var out map[string]any
	s.admin(http.MethodPost, "/mock/runs/default/try", map[string]any{"path": "/sendRequest", "body": payment("CTM", "TRY-1", nil)}, &out)
	if out["status"] != 200.0 || !strings.Contains(out["body"].(string), `"015"`) {
		t.Fatalf("playground request failed: %v", out)
	}
	if !strings.HasPrefix(out["request"].(map[string]any)["authorization"].(string), "test_client_key:") {
		t.Fatalf("playground should show the signed header: %v", out)
	}
	logs, _ := s.st.ListRequests("default", "TRY-1", 10)
	if len(logs) != 1 || logs[0].RespCode != "015" {
		t.Fatalf("playground requests are logged like any other: %+v", logs)
	}
}

func TestDashboardAndHealth(t *testing.T) {
	s := newSandbox(t)
	for path, want := range map[string]int{"/health": 200, "/dashboard/": 200, "/dashboard/transactions": 200, "/dashboard": 301} {
		rec := httptest.NewRecorder()
		s.srv.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, path, nil))
		if rec.Code != want {
			t.Errorf("GET %s = %d, want %d", path, rec.Code, want)
		}
	}
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Accept", "text/html")
	rec := httptest.NewRecorder()
	s.srv.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusFound || rec.Header().Get("Location") != "/dashboard/" {
		t.Fatalf("browsers should land on the dashboard, got %d", rec.Code)
	}
}
