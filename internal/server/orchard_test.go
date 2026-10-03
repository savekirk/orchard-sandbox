package server_test

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/savekirk/orchard-sandbox/internal/handler"
	"github.com/savekirk/orchard-sandbox/internal/model"
)

func TestAuthentication(t *testing.T) {
	s := newSandbox(t)
	body := map[string]any{"service_id": 1234, "trans_type": "BLC", "ts": ts()}

	expectCode(t, s.postAs(nil, "/check_wallet_balance", body), "101")

	stranger := *s.run
	stranger.ClientKey = "unknown"
	expectCode(t, s.postAs(&stranger, "/check_wallet_balance", body), "102")

	wrongSecret := *s.run
	wrongSecret.SecretKey = "wrong"
	expectCode(t, s.postAs(&wrongSecret, "/check_wallet_balance", body), "103")

	if got := s.post("/check_wallet_balance", body); got["payout_bal"] != 50000.0 {
		t.Fatalf("signed request should succeed, got %v", got)
	}

	expectCode(t, s.post("/check_wallet_balance", map[string]any{"trans_type": "BLC", "ts": ts()}), "006")
	expectCode(t, s.post("/check_wallet_balance", map[string]any{"service_id": 9999, "trans_type": "BLC", "ts": ts()}), "011")
}

func TestAuthenticationCanBeDisabled(t *testing.T) {
	s := newSandbox(t, func(st *model.Settings) { st.RequireAuth = false })
	got := s.postAs(nil, "/check_wallet_balance", map[string]any{"service_id": "1234", "trans_type": "BLC", "ts": ts()})
	if got["sms_bal"] != 1000.0 {
		t.Fatalf("unsigned request should succeed when auth is off, got %v", got)
	}
}

func TestSignatureCoversExactBytes(t *testing.T) {
	s := newSandbox(t)
	signed := []byte(`{"service_id": 1234, "trans_type": "BLC", "ts": "` + ts() + `"}`)
	sent := bytes.ReplaceAll(signed, []byte(": "), []byte(":"))

	req := httptest.NewRequest(http.MethodPost, "/check_wallet_balance", bytes.NewReader(sent))
	req.Header.Set("Authorization", handler.Sign(s.run.ClientKey, s.run.SecretKey, signed))
	expectCode(t, s.do(req), "103")
}

func TestCollectionLifecycle(t *testing.T) {
	s := newSandbox(t)
	cb := newReceiver(t)

	expectCode(t, s.post("/sendRequest", payment("CTM", "ORDER-1", map[string]any{"amount": "85.50", "callback_url": cb.URL})), "015")

	status := s.post("/checkTransaction", map[string]any{"service_id": 1234, "trans_type": "TSC", "exttrid": "ORDER-1"})
	if status["trans_status"] != "015" {
		t.Fatalf("new collection should be pending, got %v", status)
	}

	s.tick()

	status = s.post("/checkTransaction", map[string]any{"service_id": 1234, "trans_type": "TSC", "exttrid": "ORDER-1"})
	if status["trans_status"] != model.TransStatusSuccess || status["message"] != "SUCCESS" {
		t.Fatalf("collection should settle successfully, got %v", status)
	}
	if b := s.balances(); b.AvailableCollectBal != 75085.50 || b.ActualCollectBal != 75085.50 {
		t.Fatalf("collect balances not credited exactly: %+v", b)
	}

	got := cb.received()
	if len(got) != 1 {
		t.Fatalf("want 1 callback, got %d", len(got))
	}
	if keys(got[0]) != "message,trans_id,trans_ref,trans_status" {
		t.Fatalf("callback must only carry Orchard's documented fields, got %v", got[0])
	}
	if got[0]["trans_ref"] != "ORDER-1" || got[0]["trans_status"] != "000/01" || got[0]["trans_id"] != status["trans_id"] {
		t.Fatalf("unexpected callback payload %v", got[0])
	}

	s.tick()
	if len(cb.received()) != 1 {
		t.Fatal("callback must be delivered exactly once")
	}
}

func TestScenarioNumbers(t *testing.T) {
	s := newSandbox(t)
	cb := newReceiver(t)
	for _, tc := range []struct {
		customer, status string
		callbacks        int
	}{
		{"0240000001", model.StatusFailed, 1},
		{"0240000002", model.StatusFailed, 1},
		{"0240000003", model.StatusFailed, 1},
		{"0240000004", model.StatusPending, 0},
		{"0240000005", model.StatusSuccessful, 0},
	} {
		t.Run(tc.customer, func(t *testing.T) {
			ref := "SC-" + tc.customer
			expectCode(t, s.post("/sendRequest", payment("CTM", ref, map[string]any{"customer_number": tc.customer, "callback_url": cb.URL})), "015")
			before := len(cb.received())
			s.tick()
			txn := s.transaction(ref)
			if txn.Status != tc.status {
				t.Fatalf("status = %s, want %s", txn.Status, tc.status)
			}
			if tc.status == model.StatusFailed && (txn.TransStatus != model.TransStatusFailed || !strings.HasPrefix(txn.Message, "FAILED")) {
				t.Fatalf("failed payment should report 001/02 with a reason, got %s %q", txn.TransStatus, txn.Message)
			}
			if n := len(cb.received()) - before; n != tc.callbacks {
				t.Fatalf("callbacks = %d, want %d", n, tc.callbacks)
			}
		})
	}
	if b := s.balances(); b.AvailableCollectBal != 75010 {
		t.Fatalf("only the successful payment should credit collections, got %v", b.AvailableCollectBal)
	}
}

func TestPayoutsReserveFundsAndReverseOnFailure(t *testing.T) {
	s := newSandbox(t)

	expectCode(t, s.post("/sendRequest", payment("MTC", "PAY-1", map[string]any{"amount": "100.00"})), "015")
	if got := s.balances().PayoutBal; got != 49900 {
		t.Fatalf("payout should be reserved at acceptance, balance = %v", got)
	}
	expectCode(t, s.post("/sendRequest", payment("MTC", "PAY-2", map[string]any{"amount": "100.00", "customer_number": "0240000001"})), "015")
	s.tick()
	if got := s.balances().PayoutBal; got != 49900 {
		t.Fatalf("failed payout should be reversed, balance = %v", got)
	}

	expectCode(t, s.post("/sendRequest", payment("MTC", "PAY-3", map[string]any{"amount": "60000"})), "038")
	expectCode(t, s.post("/sendRequest", payment("ATP", "AIR-1", map[string]any{"amount": "6000"})), "064")
	expectCode(t, s.post("/sendRequest", payment("BLP", "BILL-1", map[string]any{"amount": "20000", "account_number": "123456789012", "nw": "DST"})), "086")
	expectCode(t, s.post("/sendRequest", payment("MTC", "PAY-3", nil)), "015") // rejected references are not consumed
}

func TestRequestValidation(t *testing.T) {
	s := newSandbox(t)
	for _, tc := range []struct {
		name string
		body map[string]any
		code string
	}{
		{"missing trans_type", payment("CTM", "V1", map[string]any{"trans_type": nil}), "041"},
		{"unknown trans_type", payment("XYZ", "V1", nil), "089"},
		{"missing exttrid", payment("CTM", "", nil), "018"},
		{"exttrid too long", payment("CTM", strings.Repeat("x", 31), nil), "085"},
		{"missing ts", payment("CTM", "V1", map[string]any{"ts": nil}), "053"},
		{"local time format", payment("CTM", "V1", map[string]any{"ts": "2026-01-01T10:00:00Z"}), "023"},
		{"missing amount", payment("CTM", "V1", map[string]any{"amount": nil}), "010"},
		{"invalid amount", payment("CTM", "V1", map[string]any{"amount": "ten"}), "028"},
		{"negative amount", payment("CTM", "V1", map[string]any{"amount": "-5"}), "028"},
		{"three decimals", payment("CTM", "V1", map[string]any{"amount": "1.005"}), "046"},
		{"zero amount", payment("CTM", "V1", map[string]any{"amount": 0}), "072"},
		{"airtime below minimum", payment("ATP", "V1", map[string]any{"amount": "0.10"}), "072"},
		{"missing customer", payment("CTM", "V1", map[string]any{"customer_number": nil}), "008"},
		{"missing network", payment("CTM", "V1", map[string]any{"nw": nil}), "014"},
		{"debit a bank account", payment("CTM", "V1", map[string]any{"nw": "BNK"}), "029"},
		{"unknown network", payment("CTM", "V1", map[string]any{"nw": "XXX"}), "090"},
		{"missing callback", payment("CTM", "V1", map[string]any{"callback_url": nil}), "039"},
		{"relative callback", payment("CTM", "V1", map[string]any{"callback_url": "/callback"}), "045"},
		{"bank payout without bank_code", payment("MTC", "V1", map[string]any{"nw": "BNK", "recipient_name": "Ama"}), "088"},
		{"bank payout unknown bank", payment("MTC", "V1", map[string]any{"nw": "BNK", "bank_code": "ZZZ", "recipient_name": "Ama"}), "091"},
		{"bank payout without recipient", payment("MTC", "V1", map[string]any{"nw": "BNK", "bank_code": "GCB"}), "026"},
		{"remittance missing sender", payment("RMT", "V1", nil), "026"},
		{"bill without account", payment("BLP", "V1", map[string]any{"nw": "DST"}), "069"},
		{"auto debit without mandate", payment("AUD", "V1", nil), "044"},
	} {
		t.Run(tc.name, func(t *testing.T) { expectCode(t, s.post("/sendRequest", tc.body), tc.code) })
	}

	req := httptest.NewRequest(http.MethodPost, "/sendRequest", strings.NewReader("{not json"))
	req.Header.Set("Authorization", handler.Sign(s.run.ClientKey, s.run.SecretKey, []byte("{not json")))
	expectCode(t, s.do(req), "022")
}

func TestDuplicateReferences(t *testing.T) {
	s := newSandbox(t)
	expectCode(t, s.post("/sendRequest", payment("CTM", "DUP-1", nil)), "015")
	expectCode(t, s.post("/sendRequest", payment("CTM", "DUP-1", nil)), "021")
	expectCode(t, s.post("/sendRequest", payment("MTC", "DUP-1", map[string]any{"amount": "1"})), "021")

	results := make(chan string, 10)
	var wg sync.WaitGroup
	for range 10 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			got := s.post("/sendRequest", payment("MTC", "RACE-1", map[string]any{"amount": "10.00"}))
			results <- got["resp_code"].(string)
		}()
	}
	wg.Wait()
	close(results)
	accepted := 0
	for code := range results {
		if code == "015" {
			accepted++
		}
	}
	if accepted != 1 || s.balances().PayoutBal != 49990 {
		t.Fatalf("concurrent replays must create one payout: accepted=%d payout=%v", accepted, s.balances().PayoutBal)
	}
}

func TestAccountInquiry(t *testing.T) {
	s := newSandbox(t)
	inquiry := func(ref, number, bank string) map[string]any {
		return s.post("/sendRequest", map[string]any{"service_id": 1234, "trans_type": "AII", "exttrid": ref, "customer_number": number, "nw": "BNK", "bank_code": bank, "ts": ts()})
	}
	s.admin(http.MethodPost, "/mock/runs/default/accounts", model.Account{BankCode: "GCB", AccountNumber: "1020304050", AccountName: "Kofi Annan Ltd"}, nil)
	s.admin(http.MethodPost, "/mock/runs/default/accounts", model.Account{BankCode: "MTN", AccountNumber: "0249999999", RespCode: "009"}, nil)

	if got := inquiry("AII-1", "1020304050", "GCB"); got["name"] != "Kofi Annan Ltd" {
		t.Fatalf("registered account should resolve, got %v", got)
	}
	expectCode(t, inquiry("AII-2", "0249999999", "MTN"), "009")
	expectCode(t, inquiry("AII-3", "0240000003", "MTN"), "067")
	first, second := inquiry("AII-4", "0551234567", "MTN"), inquiry("AII-5", "0551234567", "MTN")
	if first["name"] == "" || first["name"] != second["name"] {
		t.Fatalf("unregistered accounts should get a stable generated name: %v vs %v", first, second)
	}
	expectCode(t, inquiry("AII-4", "0551234567", "MTN"), "021")
	expectCode(t, s.post("/sendRequest", map[string]any{"service_id": 1234, "trans_type": "AII", "exttrid": "AII-6", "customer_number": "1", "nw": "MTN", "bank_code": "MTN", "ts": ts()}), "029")
}

func TestVerifyID(t *testing.T) {
	s := newSandbox(t)
	verify := func(ref, id string) map[string]any {
		return s.post("/verifyID", map[string]any{"service_id": 1234, "id_num": id, "id_type": "GCA", "exttrid": ref, "image": "aGVsbG8="})
	}

	got := verify("ID-1", "GHA-123456789-0")
	expectCode(t, got, "027")
	if keys(got) != "data,resp_code,resp_desc" {
		t.Fatalf("unexpected top-level keys %s", keys(got))
	}
	data := got["data"].(map[string]any)
	if keys(data) != "card_valid_end,card_valid_start,gender,name,verified" || data["verified"] != "true" {
		t.Fatalf("unexpected identity data %v", data)
	}

	if d := verify("ID-2", "GHA-123456789-9")["data"].(map[string]any); d["verified"] != "false" {
		t.Fatalf("card ending in 9 should not verify, got %v", d)
	}

	s.admin(http.MethodPost, "/mock/runs/default/cards", map[string]any{"id_num": "GHA-111111111-1", "name": "Yaa Asantewaa", "gender": "F", "verified": "true", "card_valid_start": "2020-01-01", "card_valid_end": "2030-01-01"}, nil)
	if d := verify("ID-3", "GHA-111111111-1")["data"].(map[string]any); d["name"] != "Yaa Asantewaa" {
		t.Fatalf("registered card should be returned, got %v", d)
	}

	expectCode(t, verify("ID-4", "GHA-12345-0"), "009")
	expectCode(t, verify(strings.Repeat("x", 21), "GHA-123456789-0"), "025")
	expectCode(t, verify("ID-1", "GHA-123456789-0"), "021")
	expectCode(t, s.post("/verifyID", map[string]any{"service_id": 1234, "id_num": "GHA-123456789-0", "id_type": "GCA", "exttrid": "ID-5"}), "026")
}

func TestSendSMS(t *testing.T) {
	s := newSandbox(t)
	sms := func(id, body, sender string) map[string]any {
		return s.post("/sendSms", map[string]any{"service_id": 1234, "trans_type": "SMS", "recipient_number": "233241234567", "msg_body": body, "sender_id": sender, "unique_id": id, "msg_type": "T"})
	}
	got := sms("SMS-1", strings.Repeat("a", 161), "SHOP")
	expectCode(t, got, "082")
	if keys(got) != "resp_code,resp_desc" {
		t.Fatalf("unexpected keys %s", keys(got))
	}
	if s.balances().SMSBal != 998 {
		t.Fatalf("a 161 character message costs 2 units, balance = %v", s.balances().SMSBal)
	}
	expectCode(t, sms("SMS-1", "again", "SHOP"), "021")
	expectCode(t, sms("SMS-2", "hi", "TOOLONGSENDER"), "078")
	expectCode(t, sms("SMS-3", "", "SHOP"), "079")

	s.admin(http.MethodPut, "/mock/runs/default/balances", map[string]any{"sms_bal": 0}, nil)
	expectCode(t, sms("SMS-4", "hi", "SHOP"), "076")
}

func TestCheckTransaction(t *testing.T) {
	s := newSandbox(t)
	expectCode(t, s.post("/checkTransaction", map[string]any{"service_id": 1234, "trans_type": "TSC", "exttrid": "nope"}), "033")
	expectCode(t, s.post("/checkTransaction", map[string]any{"service_id": 1234, "trans_type": "CTR", "exttrid": "nope"}), "089")
	expectCode(t, s.post("/checkWallet", map[string]any{}), "051")
}
