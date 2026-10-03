package handler

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/savekirk/orchard-sandbox/internal/model"
	"github.com/savekirk/orchard-sandbox/internal/store"
)

// RunHeader routes an unsigned request to a specific sandbox run.
const RunHeader = "X-Sandbox-Run"

// call is one in-flight Orchard API request.
type call struct {
	w      http.ResponseWriter
	r      *http.Request
	run    *model.Run
	body   []byte
	fields map[string]any
	op     string
	ref    string

	respCode string
}

func (c *call) reply(v any) {
	c.respCode = respCodeOf(v)
	writeJSON(c.w, http.StatusOK, v)
}

func (c *call) code(code string) { c.reply(model.Reply(code)) }

func respCodeOf(v any) string {
	switch r := v.(type) {
	case model.CommonResponse:
		return r.RespCode
	case model.AccountInquiryResponse:
		return r.RespCode
	case model.VerifyIDResponse:
		return r.RespCode
	case model.TransactionStatusResponse:
		return r.TransStatus
	case map[string]any:
		if code, ok := r["resp_code"].(string); ok {
			return code
		}
		if code, ok := r["status_code"].(string); ok {
			return code
		}
	case autoDebitReply:
		return r.StatusCode
	}
	return ""
}

// str returns a request field as a trimmed string ("" if absent or not a scalar).
func (c *call) str(key string) string {
	switch v := c.fields[key].(type) {
	case string:
		return strings.TrimSpace(v)
	case json.Number:
		return v.String()
	case bool:
		return strconv.FormatBool(v)
	}
	return ""
}

// timestamp validates ts. Orchard expects "YYYY-MM-DD HH:MM:SS" in UTC.
func (c *call) timestamp(required bool) bool {
	ts := c.str("ts")
	if ts == "" {
		if required {
			c.code(model.CodeMissingTimestamp)
			return false
		}
		return true
	}
	if _, err := time.Parse(time.DateTime, ts); err != nil {
		c.code(model.CodeInvalidTimestamp)
		return false
	}
	return true
}

// callbackURL validates a mandatory callback URL field.
func (c *call) callbackURL(key string) (string, bool) {
	raw := c.str(key)
	if raw == "" {
		c.code(model.CodeMissingCallbackURL)
		return "", false
	}
	if !validURL(raw) {
		c.code(model.CodeInvalidCallbackURL)
		return "", false
	}
	return raw, true
}

func validURL(raw string) bool {
	u, err := url.Parse(raw)
	return err == nil && (u.Scheme == "http" || u.Scheme == "https") && u.Host != ""
}

// amount parses the amount field into pesewas, replying with the matching code on failure.
func (c *call) amount(minimum int64) (int64, bool) {
	pesewas, err := model.ParseAmount(c.fields["amount"])
	if err == nil && pesewas < minimum {
		err = model.ErrAmountTooLow
	}
	if err != nil {
		c.code(model.AmountCode(err))
		return 0, false
	}
	return pesewas, true
}

// Orchard wraps an Orchard endpoint with run resolution, authentication, fault injection and logging.
func (h *Handler) Orchard(endpoint string, serve func(*call)) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		body, _ := io.ReadAll(io.LimitReader(r.Body, 1<<20))
		rec := &recorder{ResponseWriter: w, status: http.StatusOK}
		c := &call{w: rec, r: r, body: body, op: endpoint}

		dec := json.NewDecoder(bytes.NewReader(body))
		dec.UseNumber()
		if dec.Decode(&c.fields) != nil {
			c.fields = nil
		}
		for _, key := range []string{"exttrid", "unique_id", "uniq_ref_id"} {
			if c.ref = c.str(key); c.ref != "" {
				break
			}
		}
		if tt := c.str("trans_type"); endpoint == "sendRequest" && tt != "" {
			c.op = endpoint + ":" + strings.ToUpper(tt)
		}

		run, err := h.resolveRun(r)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}
		c.run = run

		defer func() {
			headers := map[string]string{}
			for k, v := range r.Header {
				headers[strings.ToLower(k)] = strings.Join(v, ", ")
			}
			h.st.LogRequest(&model.RequestLog{
				RunID: run.RunID, Method: r.Method, Path: r.URL.Path, Operation: c.op, Ref: c.ref,
				Headers: headers, Body: string(body), Status: rec.status, ResponseBody: rec.body.String(),
				RespCode: c.respCode, DurationMs: time.Since(start).Milliseconds(),
			})
		}()

		if latency := run.Settings.LatencyMs; latency > 0 {
			select {
			case <-time.After(time.Duration(latency) * time.Millisecond):
			case <-r.Context().Done():
				return
			}
		}
		if !h.authenticate(c) {
			return
		}
		h.injectFailure(c, func(c *call) {
			switch sid := c.str("service_id"); {
			case c.fields == nil:
				c.code(model.CodeInvalidJSON)
			case sid == "":
				c.code(model.CodeMissingServiceID)
			case sid != run.ServiceID:
				c.code(model.CodeInvalidServiceID)
			default:
				serve(c)
			}
		})
	}
}

// resolveRun finds the sandbox a request belongs to: the owner of the client key, the X-Sandbox-Run header, or the default run.
func (h *Handler) resolveRun(r *http.Request) (*model.Run, error) {
	if clientKey, _, ok := strings.Cut(r.Header.Get("Authorization"), ":"); ok {
		if run, err := h.st.RunByClientKey(strings.TrimSpace(clientKey)); err == nil {
			return run, nil
		}
	}
	runID := r.Header.Get(RunHeader)
	if runID == "" {
		runID = store.DefaultRunID
	}
	run, err := h.st.GetRun(runID)
	if errors.Is(err, store.ErrNotFound) {
		return nil, errors.New("unknown sandbox run " + strconv.Quote(runID))
	}
	return run, err
}

// authenticate checks Orchard's HMAC header: "Authorization: CLIENT_KEY:hex(HMAC-SHA256(body, SECRET_KEY))".
func (h *Handler) authenticate(c *call) bool {
	if !c.run.Settings.RequireAuth {
		return true
	}
	clientKey, signature, ok := strings.Cut(c.r.Header.Get("Authorization"), ":")
	if !ok || strings.TrimSpace(clientKey) == "" {
		c.code(model.CodeNoAuthHeader)
		return false
	}
	if strings.TrimSpace(clientKey) != c.run.ClientKey {
		c.code(model.CodeInvalidTokens)
		return false
	}
	mac := hmac.New(sha256.New, []byte(c.run.SecretKey))
	mac.Write(c.body)
	expected := hex.EncodeToString(mac.Sum(nil))
	if !hmac.Equal([]byte(strings.ToLower(strings.TrimSpace(signature))), []byte(expected)) {
		c.code(model.CodeInvalidSignature)
		return false
	}
	return true
}

// Sign returns the Authorization header value for body.
func Sign(clientKey, secretKey string, body []byte) string {
	mac := hmac.New(sha256.New, []byte(secretKey))
	mac.Write(body)
	return clientKey + ":" + hex.EncodeToString(mac.Sum(nil))
}

// UnknownEndpoint answers requests to paths Orchard does not serve.
func (h *Handler) UnknownEndpoint(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, model.Reply("051"))
}

// recorder captures the response for the request log while passing it through.
type recorder struct {
	http.ResponseWriter
	status int
	body   bytes.Buffer
}

func (r *recorder) WriteHeader(status int) {
	r.status = status
	r.ResponseWriter.WriteHeader(status)
}

func (r *recorder) Write(b []byte) (int, error) {
	if r.body.Len() < 64<<10 {
		r.body.Write(b[:min(len(b), 64<<10-r.body.Len())])
	}
	return r.ResponseWriter.Write(b)
}

func (r *recorder) Unwrap() http.ResponseWriter { return r.ResponseWriter }
