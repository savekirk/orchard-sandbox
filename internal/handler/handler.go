// Package handler implements the Orchard API, the hosted checkout pages, and the sandbox admin API.
package handler

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"sync"

	"github.com/savekirk/orchard-sandbox/internal/engine"
	"github.com/savekirk/orchard-sandbox/internal/store"
)

// Handler serves every sandbox endpoint.
type Handler struct {
	st        *store.Store
	eng       *engine.Engine
	publicURL string

	// api routes playground requests through the full Orchard pipeline.
	api http.Handler

	holdsMu sync.Mutex
	holds   map[string]*hold
}

// New returns a Handler. publicURL overrides the base URL used in redirect links.
func New(st *store.Store, eng *engine.Engine, publicURL string) *Handler {
	return &Handler{st: st, eng: eng, publicURL: strings.TrimRight(publicURL, "/"), holds: map[string]*hold{}}
}

// SetAPI gives the handler the router used by the playground.
func (h *Handler) SetAPI(api http.Handler) { h.api = api }

// baseURL is the externally visible origin of the sandbox.
func (h *Handler) baseURL(r *http.Request) string {
	if h.publicURL != "" {
		return h.publicURL
	}
	scheme := "http"
	if r.TLS != nil || r.Header.Get("X-Forwarded-Proto") == "https" {
		scheme = "https"
	}
	host := r.Header.Get("X-Forwarded-Host")
	if host == "" {
		host = r.Host
	}
	return scheme + "://" + host
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, err error) {
	status := http.StatusInternalServerError
	switch {
	case errors.Is(err, store.ErrNotFound):
		status = http.StatusNotFound
	case errors.Is(err, store.ErrRunExists), errors.Is(err, store.ErrDuplicate):
		status = http.StatusConflict
	case errors.As(err, new(badRequest)), errors.Is(err, store.ErrInvalid):
		status = http.StatusBadRequest
	}
	writeJSON(w, status, map[string]string{"error": err.Error()})
}

type badRequest string

func (b badRequest) Error() string { return string(b) }

func decodeJSON(r *http.Request, v any) error {
	if err := json.NewDecoder(r.Body).Decode(v); err != nil {
		return badRequest("invalid JSON body: " + err.Error())
	}
	return nil
}

// Health reports liveness. Browsers are sent to the dashboard.
func (h *Handler) Health(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path == "/" && strings.Contains(r.Header.Get("Accept"), "text/html") {
		http.Redirect(w, r, "/dashboard/", http.StatusFound)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}
