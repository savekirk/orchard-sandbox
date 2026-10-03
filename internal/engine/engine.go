// Package engine settles pending payments and delivers callbacks in the background.
package engine

import (
	"bytes"
	"context"
	"io"
	"log/slog"
	"net/http"
	"sync"
	"time"

	"github.com/savekirk/orchard-sandbox/internal/model"
	"github.com/savekirk/orchard-sandbox/internal/store"
)

// maxConcurrentDeliveries bounds simultaneous outbound callback requests.
const maxConcurrentDeliveries = 8

// Engine drives the asynchronous side of the sandbox: settlement and webhooks.
type Engine struct {
	st     *store.Store
	client *http.Client
	slots  chan struct{}
	wg     sync.WaitGroup
}

// New returns an engine backed by st.
func New(st *store.Store) *Engine {
	return &Engine{st: st, client: &http.Client{}, slots: make(chan struct{}, maxConcurrentDeliveries)}
}

// Start runs the engine until ctx is cancelled.
func (e *Engine) Start(ctx context.Context) {
	if err := e.st.RequeueStuckCallbacks(); err != nil {
		slog.Error("requeue callbacks", "err", err)
	}
	go func() {
		ticker := time.NewTicker(100 * time.Millisecond)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				e.Tick(ctx)
			}
		}
	}()
}

// Tick settles due transactions and starts delivery of due callbacks.
func (e *Engine) Tick(ctx context.Context) {
	e.settleDue()
	e.deliverDue(ctx)
}

// Wait blocks until in-flight callback deliveries finish.
func (e *Engine) Wait() { e.wg.Wait() }

func (e *Engine) settleDue() {
	due, err := e.st.DueTransactions(50)
	if err != nil {
		slog.Error("load due transactions", "err", err)
		return
	}
	for _, key := range due {
		t, err := e.st.GetTransaction(key.RunID, key.Exttrid)
		if err != nil {
			continue
		}
		sc := model.ScenarioByKey(t.Scenario)
		if _, err := e.settle(key.RunID, key.Exttrid, sc.Success, sc.Message, !sc.NoCallback); err != nil {
			slog.Error("settle transaction", "run", key.RunID, "exttrid", key.Exttrid, "err", err)
		}
	}
}

// Resolve settles a pending transaction by hand and notifies its callback_url.
func (e *Engine) Resolve(runID, exttrid string, success bool, message string) (*model.Transaction, error) {
	return e.settle(runID, exttrid, success, message, true)
}

func (e *Engine) settle(runID, exttrid string, success bool, message string, notify bool) (*model.Transaction, error) {
	run, err := e.st.GetRun(runID)
	if err != nil {
		return nil, err
	}
	return e.st.Settle(runID, exttrid, store.Outcome{Success: success, Message: message, Notify: notify, Settings: run.Settings})
}

func (e *Engine) deliverDue(ctx context.Context) {
	free := cap(e.slots) - len(e.slots)
	if free == 0 {
		return
	}
	due, err := e.st.ClaimDueCallbacks(free)
	if err != nil {
		slog.Error("claim callbacks", "err", err)
		return
	}
	for _, d := range due {
		e.wg.Add(1)
		e.slots <- struct{}{}
		go func() {
			defer func() { <-e.slots; e.wg.Done() }()
			e.deliver(ctx, d)
		}()
	}
}

func (e *Engine) deliver(ctx context.Context, d store.Delivery) {
	ctx, cancel := context.WithTimeout(ctx, time.Duration(d.TimeoutMs)*time.Millisecond)
	defer cancel()

	attempt := model.CallbackAttempt{CallbackID: d.ID}
	start := time.Now()
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, d.URL, bytes.NewReader([]byte(d.Payload)))
	if err == nil {
		req.Header.Set("Content-Type", "application/json")
		var resp *http.Response
		resp, err = e.client.Do(req)
		if err == nil {
			body, _ := io.ReadAll(io.LimitReader(resp.Body, 2048))
			_ = resp.Body.Close()
			attempt.HTTPStatus, attempt.ResponseBody = resp.StatusCode, string(body)
		}
	}
	if err != nil {
		attempt.Error = err.Error()
	}
	attempt.DurationMs = time.Since(start).Milliseconds()

	if err := e.st.RecordDelivery(d, attempt, backoff(d.Attempts)); err != nil {
		slog.Error("record callback attempt", "callback", d.ID, "err", err)
	}
}

// backoff is the wait before the next attempt: 2s, 4s, 8s ... capped at one minute.
func backoff(previousAttempts int) time.Duration {
	return min(2*time.Second<<min(previousAttempts, 5), time.Minute)
}
