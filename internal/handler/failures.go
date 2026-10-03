package handler

import (
	"fmt"
	"net/http"
	"slices"
	"strings"
	"time"

	"github.com/savekirk/orchard-sandbox/internal/model"
)

// maxHold bounds how long a held request waits for release.
const maxHold = 5 * time.Minute

type hold struct {
	ID        string `json:"id"`
	RunID     string `json:"run_id"`
	Operation string `json:"operation"`
	Exttrid   string `json:"exttrid"`
	Attempt   int    `json:"attempt"`
	Since     string `json:"since"`
	release   chan struct{}
}

// matchRule returns the first rule that applies to the call, and the attempt number.
func (h *Handler) matchRule(c *call) (*model.FailureRule, int) {
	rules, err := h.st.ListFailureRules(c.run.RunID)
	if err != nil || len(rules) == 0 {
		return nil, 0
	}
	attempt := h.st.CountAttempts(c.run.RunID, c.op, c.ref) + 1
	for _, rule := range rules {
		if !operationMatches(rule.Operation, c.op) {
			continue
		}
		if rule.Exttrid != "*" && rule.Exttrid != c.ref {
			continue
		}
		if rule.Attempt != 0 && rule.Attempt != attempt {
			continue
		}
		return &rule, attempt
	}
	return nil, 0
}

// operationMatches reports whether a rule's operation covers op.
// "*" matches everything and "sendRequest" matches every "sendRequest:<trans_type>".
func operationMatches(rule, op string) bool {
	base, _, _ := strings.Cut(op, ":")
	return rule == "*" || strings.EqualFold(rule, op) || strings.EqualFold(rule, base)
}

// injectFailure applies the first matching failure rule, or serves the request normally.
func (h *Handler) injectFailure(c *call, serve func(*call)) {
	rule, attempt := h.matchRule(c)
	if rule == nil {
		serve(c)
		return
	}
	w := c.w
	raw := func(status int, body string) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		_, _ = w.Write([]byte(body))
	}

	switch rule.Action {
	case model.ActionHTTPStatus:
		status := rule.HTTPStatus
		if status < 100 || status > 599 {
			status = http.StatusInternalServerError
		}
		if status == http.StatusTooManyRequests {
			w.Header().Set("Retry-After", "5")
		}
		body := rule.ResponseBody
		if body == "" {
			body = fmt.Sprintf(`{"error":"simulated %d %s"}`, status, http.StatusText(status))
		}
		raw(status, body)
	case model.ActionRespCode:
		if rule.ResponseBody != "" {
			c.respCode = rule.RespCode
			raw(http.StatusOK, rule.ResponseBody)
			return
		}
		c.code(rule.RespCode)
	case model.ActionDelay:
		if rule.Hold {
			h.waitForRelease(c, attempt)
		} else if rule.DelayMs > 0 {
			select {
			case <-time.After(time.Duration(rule.DelayMs) * time.Millisecond):
			case <-c.r.Context().Done():
				return
			}
		}
		serve(c)
	case model.ActionDrop:
		dropConnection(w)
	case model.ActionDropAfterProcess:
		inner := *c
		inner.w = discard{header: http.Header{}}
		serve(&inner)
		c.respCode = inner.respCode
		dropConnection(w)
	case model.ActionMalformedJSON:
		raw(http.StatusOK, `{"resp_code":"027","resp_desc":"Request successfully compl`)
	case model.ActionMissingFields:
		raw(http.StatusOK, `{"status":"ok"}`)
	case model.ActionWrongTypes:
		raw(http.StatusOK, `{"resp_code":27,"resp_desc":true,"data":{"verified":true,"name":12345},"trans_status":0}`)
	case model.ActionOversized:
		raw(http.StatusOK, `{"resp_code":"027","resp_desc":"Request successfully completed","padding":"`+strings.Repeat("x", 1<<20)+`"}`)
	case model.ActionRedirect:
		http.Redirect(w, c.r, "/unexpected-redirect", http.StatusFound)
	default:
		serve(c)
	}
}

// dropConnection closes the TCP connection without sending a response.
func dropConnection(w http.ResponseWriter) {
	if rec, ok := w.(*recorder); ok {
		rec.status = 0 // logged as a dropped connection
	}
	conn, _, err := http.NewResponseController(w).Hijack()
	if err != nil {
		panic(http.ErrAbortHandler)
	}
	_ = conn.Close()
}

// discard swallows a response so the request is processed but never answered.
type discard struct{ header http.Header }

func (d discard) Header() http.Header         { return d.header }
func (d discard) Write(b []byte) (int, error) { return len(b), nil }
func (d discard) WriteHeader(int)             {}

func (h *Handler) waitForRelease(c *call, attempt int) {
	hd := &hold{
		ID: "hold_" + fmt.Sprint(time.Now().UnixNano()), RunID: c.run.RunID, Operation: c.op, Exttrid: c.ref,
		Attempt: attempt, Since: time.Now().UTC().Format(time.DateTime), release: make(chan struct{}),
	}
	h.holdsMu.Lock()
	h.holds[hd.ID] = hd
	h.holdsMu.Unlock()
	h.st.Publish(c.run.RunID, "holds")

	select {
	case <-hd.release:
	case <-c.r.Context().Done():
	case <-time.After(maxHold):
	}

	h.holdsMu.Lock()
	delete(h.holds, hd.ID)
	h.holdsMu.Unlock()
	h.st.Publish(c.run.RunID, "holds")
}

func (h *Handler) listHolds(runID string) []*hold {
	h.holdsMu.Lock()
	defer h.holdsMu.Unlock()
	out := []*hold{}
	for _, hd := range h.holds {
		if hd.RunID == runID {
			out = append(out, hd)
		}
	}
	slices.SortFunc(out, func(a, b *hold) int { return strings.Compare(a.ID, b.ID) })
	return out
}

func (h *Handler) releaseHold(runID, id string) bool {
	h.holdsMu.Lock()
	defer h.holdsMu.Unlock()
	hd, ok := h.holds[id]
	if !ok || hd.RunID != runID {
		return false
	}
	delete(h.holds, id)
	close(hd.release)
	return true
}
