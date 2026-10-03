package server_test

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"maps"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/savekirk/orchard-sandbox/internal/config"
	"github.com/savekirk/orchard-sandbox/internal/handler"
	"github.com/savekirk/orchard-sandbox/internal/model"
	"github.com/savekirk/orchard-sandbox/internal/server"
	"github.com/savekirk/orchard-sandbox/internal/store"
)

// sandbox is a test harness around an in-memory sandbox.
type sandbox struct {
	t   *testing.T
	st  *store.Store
	srv *server.Server
	run *model.Run
	now time.Time
}

func newSandbox(t *testing.T, configure ...func(*model.Settings)) *sandbox {
	t.Helper()
	st, err := store.Open("")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })

	cfg := config.Default()
	cfg.Settings.SettleDelayMs = 0
	for _, f := range configure {
		f(&cfg.Settings)
	}
	run, err := st.EnsureDefaultRun(store.NewRun{
		ClientKey: cfg.Credentials.ClientKey, SecretKey: cfg.Credentials.SecretKey, ServiceID: cfg.Credentials.ServiceID,
		Settings: &cfg.Settings, Balances: cfg.Balances.Ledger(),
	})
	if err != nil {
		t.Fatal(err)
	}
	s := &sandbox{t: t, st: st, srv: server.New(":0", st, "http://sandbox.test"), run: run, now: time.Now()}
	st.SetClock(func() time.Time { return s.now })
	return s
}

// ts is a valid Orchard timestamp.
func ts() string { return time.Now().UTC().Format(time.DateTime) }

// post sends a signed Orchard request using the default run's credentials.
func (s *sandbox) post(path string, body any) map[string]any {
	s.t.Helper()
	return s.postAs(s.run, path, body)
}

func (s *sandbox) postAs(run *model.Run, path string, body any) map[string]any {
	s.t.Helper()
	raw, _ := json.Marshal(body)
	req := httptest.NewRequest(http.MethodPost, path, bytes.NewReader(raw))
	req.Header.Set("Content-Type", "application/json")
	if run != nil {
		req.Header.Set("Authorization", handler.Sign(run.ClientKey, run.SecretKey, raw))
	}
	return s.do(req)
}

func (s *sandbox) do(req *http.Request) map[string]any {
	s.t.Helper()
	rec := httptest.NewRecorder()
	s.srv.Handler().ServeHTTP(rec, req)
	var out map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		s.t.Fatalf("%s %s: invalid JSON %q", req.Method, req.URL.Path, rec.Body.String())
	}
	return out
}

// admin calls the sandbox admin API and decodes the JSON response into out (if non-nil).
func (s *sandbox) admin(method, path string, body any, out any) int {
	s.t.Helper()
	var reader io.Reader
	if body != nil {
		raw, _ := json.Marshal(body)
		reader = bytes.NewReader(raw)
	}
	req := httptest.NewRequest(method, path, reader)
	rec := httptest.NewRecorder()
	s.srv.Handler().ServeHTTP(rec, req)
	if out != nil {
		if err := json.Unmarshal(rec.Body.Bytes(), out); err != nil {
			s.t.Fatalf("%s %s: invalid JSON %q", method, path, rec.Body.String())
		}
	}
	return rec.Code
}

// tick runs one engine cycle and waits for callback deliveries.
func (s *sandbox) tick() {
	s.srv.Engine().Tick(context.Background())
	s.srv.Engine().Wait()
}

func (s *sandbox) advance(d time.Duration) { s.now = s.now.Add(d) }

func (s *sandbox) balances() model.Balances {
	s.t.Helper()
	b, err := s.st.WalletBalances(s.run.RunID)
	if err != nil {
		s.t.Fatal(err)
	}
	return b
}

func (s *sandbox) transaction(exttrid string) *model.Transaction {
	s.t.Helper()
	t, err := s.st.GetTransaction(s.run.RunID, exttrid)
	if err != nil {
		s.t.Fatalf("transaction %s: %v", exttrid, err)
	}
	return t
}

func (s *sandbox) callbacks() []model.Callback {
	s.t.Helper()
	cbs, err := s.st.ListCallbacks(s.run.RunID, 100)
	if err != nil {
		s.t.Fatal(err)
	}
	return cbs
}

func expectCode(t *testing.T, got map[string]any, code string) {
	t.Helper()
	if got["resp_code"] != code {
		t.Fatalf("resp_code = %v, want %s (%v)", got["resp_code"], code, got)
	}
}

// unreachableCallback refuses connections immediately, so deliveries fail fast.
const unreachableCallback = "http://127.0.0.1:9/callback"

// payment builds a sendRequest body; extra fields override the defaults (nil deletes a field).
func payment(transType, exttrid string, extra map[string]any) map[string]any {
	body := map[string]any{
		"service_id": 1234, "trans_type": transType, "exttrid": exttrid, "amount": "10.00",
		"customer_number": "0241234567", "nw": "MTN", "reference": "Test", "callback_url": unreachableCallback, "ts": ts(),
	}
	for k, v := range extra {
		if v == nil {
			delete(body, k)
		} else {
			body[k] = v
		}
	}
	return body
}

// receiver is a callback endpoint that records payloads.
type receiver struct {
	*httptest.Server
	mu       sync.Mutex
	payloads []map[string]any
	status   int
}

func newReceiver(t *testing.T) *receiver {
	r := &receiver{status: http.StatusOK}
	r.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		var p map[string]any
		_ = json.NewDecoder(req.Body).Decode(&p)
		r.mu.Lock()
		r.payloads = append(r.payloads, p)
		status := r.status
		r.mu.Unlock()
		w.WriteHeader(status)
	}))
	t.Cleanup(r.Close)
	return r
}

func (r *receiver) received() []map[string]any {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]map[string]any(nil), r.payloads...)
}

func mustJSON(v any) string {
	b, _ := json.Marshal(v)
	return string(b)
}

func signBody(s *sandbox, body string) string {
	return handler.Sign(s.run.ClientKey, s.run.SecretKey, []byte(body))
}

func keys(m map[string]any) string {
	return strings.Join(slices.Sorted(maps.Keys(m)), ",")
}
