// Package server wires the sandbox's HTTP routes and background engine together.
package server

import (
	"context"
	"net/http"
	"time"

	"github.com/savekirk/orchard-sandbox/internal/engine"
	"github.com/savekirk/orchard-sandbox/internal/handler"
	"github.com/savekirk/orchard-sandbox/internal/store"
	"github.com/savekirk/orchard-sandbox/internal/web"
)

// Server is the sandbox HTTP server.
type Server struct {
	http   *http.Server
	engine *engine.Engine
	cancel context.CancelFunc
}

// New builds the server. Call Start to serve and run the engine.
func New(addr string, st *store.Store, publicURL string) *Server {
	eng := engine.New(st)
	h := handler.New(st, eng, publicURL)
	mux := http.NewServeMux()

	// Orchard API (https://docs.anmgw.com/docs-page.html)
	mux.HandleFunc("POST /sendRequest", h.Orchard("sendRequest", h.SendRequest))
	mux.HandleFunc("POST /verifyID", h.Orchard("verifyID", h.VerifyID))
	mux.HandleFunc("POST /check_wallet_balance", h.Orchard("check_wallet_balance", h.CheckWalletBalance))
	mux.HandleFunc("POST /checkTransaction", h.Orchard("checkTransaction", h.CheckTransaction))
	mux.HandleFunc("POST /sendSms", h.Orchard("sendSms", h.SendSMS))
	mux.HandleFunc("POST /third_party_request", h.Orchard("third_party_request", h.ThirdPartyRequest))
	mux.HandleFunc("POST /autoDebit", h.Orchard("autoDebit", h.AutoDebit))
	mux.HandleFunc("POST /", h.UnknownEndpoint)

	// Customer-facing pages
	mux.HandleFunc("GET /payment_page", h.PaymentPage)
	mux.HandleFunc("POST /payment_page", h.PaymentAction)
	mux.HandleFunc("GET /ghipss/EcomPayment/RedirectAuthLink", h.PaymentPage)
	mux.HandleFunc("POST /ghipss/EcomPayment/RedirectAuthLink", h.PaymentPage)

	// Sandbox admin API
	mux.HandleFunc("GET /mock/info", h.Info)
	mux.HandleFunc("GET /mock/events", h.Events)
	mux.HandleFunc("POST /mock/sink", h.Sink)
	mux.HandleFunc("GET /mock/runs", h.ListRuns)
	mux.HandleFunc("POST /mock/runs", h.CreateRun)
	mux.HandleFunc("GET /mock/runs/{run}", h.GetRun)
	mux.HandleFunc("DELETE /mock/runs/{run}", h.DeleteRun)
	mux.HandleFunc("POST /mock/runs/{run}/reset", h.ResetRun)
	mux.HandleFunc("GET /mock/runs/{run}/overview", h.Overview)
	mux.HandleFunc("PUT /mock/runs/{run}/settings", h.UpdateSettings)
	mux.HandleFunc("GET /mock/runs/{run}/balances", h.Balances)
	mux.HandleFunc("PUT /mock/runs/{run}/balances", h.SetBalances)
	mux.HandleFunc("GET /mock/runs/{run}/ledger", h.Ledger)
	mux.HandleFunc("GET /mock/runs/{run}/requests", h.Requests)
	mux.HandleFunc("GET /mock/runs/{run}/transactions", h.Transactions)
	mux.HandleFunc("GET /mock/runs/{run}/transactions/{exttrid}", h.Transaction)
	mux.HandleFunc("POST /mock/runs/{run}/transactions/{exttrid}/resolve", h.Resolve)
	mux.HandleFunc("GET /mock/runs/{run}/transactions/{exttrid}/submissions", h.Submissions)
	mux.HandleFunc("GET /mock/runs/{run}/callbacks", h.Callbacks)
	mux.HandleFunc("GET /mock/runs/{run}/callbacks/{id}/attempts", h.CallbackAttempts)
	mux.HandleFunc("POST /mock/runs/{run}/callbacks/{id}/retry", h.RetryCallback)
	mux.HandleFunc("GET /mock/runs/{run}/sms", h.Messages)
	mux.HandleFunc("GET /mock/runs/{run}/cards", h.Cards)
	mux.HandleFunc("POST /mock/runs/{run}/cards", h.SaveCard)
	mux.HandleFunc("DELETE /mock/runs/{run}/cards/{id}", h.DeleteCard)
	mux.HandleFunc("GET /mock/runs/{run}/accounts", h.Accounts)
	mux.HandleFunc("POST /mock/runs/{run}/accounts", h.SaveAccount)
	mux.HandleFunc("DELETE /mock/runs/{run}/accounts/{bank}/{number}", h.DeleteAccount)
	mux.HandleFunc("GET /mock/runs/{run}/subscriptions", h.Subscriptions)
	mux.HandleFunc("POST /mock/runs/{run}/subscriptions/{ref}/debit", h.Debit)
	mux.HandleFunc("GET /mock/runs/{run}/failures", h.FailureRules)
	mux.HandleFunc("POST /mock/runs/{run}/failures", h.AddFailureRule)
	mux.HandleFunc("DELETE /mock/runs/{run}/failures/{id}", h.DeleteFailureRule)
	mux.HandleFunc("GET /mock/runs/{run}/holds", h.Holds)
	mux.HandleFunc("POST /mock/runs/{run}/holds/{id}/release", h.ReleaseHold)
	mux.HandleFunc("POST /mock/runs/{run}/try", h.Try)

	// Dashboard and health
	mux.Handle("GET /dashboard/", web.Handler())
	mux.Handle("GET /dashboard", http.RedirectHandler("/dashboard/", http.StatusMovedPermanently))
	mux.HandleFunc("GET /health", h.Health)
	mux.HandleFunc("GET /{$}", h.Health)

	h.SetAPI(mux)

	return &Server{
		engine: eng,
		http: &http.Server{
			Addr:              addr,
			Handler:           mux,
			ReadHeaderTimeout: 10 * time.Second,
			IdleTimeout:       2 * time.Minute,
		},
	}
}

// Handler returns the root HTTP handler.
func (s *Server) Handler() http.Handler { return s.http.Handler }

// Engine returns the settlement and callback engine.
func (s *Server) Engine() *engine.Engine { return s.engine }

// Start runs the background engine and serves HTTP until Shutdown.
func (s *Server) Start() error {
	ctx, cancel := context.WithCancel(context.Background())
	s.cancel = cancel
	s.engine.Start(ctx)
	return s.http.ListenAndServe()
}

// Shutdown stops the engine and drains in-flight requests.
func (s *Server) Shutdown(ctx context.Context) error {
	if s.cancel != nil {
		s.cancel()
	}
	return s.http.Shutdown(ctx)
}
