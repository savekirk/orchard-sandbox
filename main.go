// Command orchard-sandbox runs a local sandbox of the Orchard payments API.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/savekirk/orchard-sandbox/internal/config"
	"github.com/savekirk/orchard-sandbox/internal/server"
	"github.com/savekirk/orchard-sandbox/internal/store"
)

func main() {
	configPath := flag.String("config", "", "path to a YAML config file (or set ORCHARD_SANDBOX_CONFIG)")
	healthcheck := flag.Bool("healthcheck", false, "check that a sandbox is serving on the configured port, then exit")
	flag.Parse()

	cfg, err := config.Load(*configPath)
	if err != nil {
		fail(err)
	}
	if *healthcheck {
		os.Exit(checkHealth(cfg.Port))
	}
	if err := run(cfg); err != nil {
		fail(err)
	}
}

func run(cfg config.Config) error {
	st, err := store.Open(cfg.DB)
	if err != nil {
		return err
	}
	defer st.Close()

	settings := cfg.Settings
	run, err := st.EnsureDefaultRun(store.NewRun{
		ClientKey: cfg.Credentials.ClientKey, SecretKey: cfg.Credentials.SecretKey, ServiceID: cfg.Credentials.ServiceID,
		Settings: &settings, Balances: cfg.Balances.Ledger(),
	})
	if err != nil {
		return err
	}
	for _, card := range cfg.Cards {
		if err := st.SaveCard(run.RunID, card); err != nil {
			return fmt.Errorf("seed card %s: %w", card.IDNum, err)
		}
	}
	for _, account := range cfg.Accounts {
		if err := st.SaveAccount(run.RunID, account); err != nil {
			return fmt.Errorf("seed account %s: %w", account.AccountNumber, err)
		}
	}

	srv := server.New(":"+cfg.Port, st, cfg.PublicURL)
	errs := make(chan error, 1)
	go func() { errs <- srv.Start() }()

	storage := cfg.DB
	if storage == "" {
		storage = "memory"
	}
	slog.Info("Orchard Sandbox ready",
		"api", "http://localhost:"+cfg.Port, "dashboard", "http://localhost:"+cfg.Port+"/dashboard/",
		"client_key", run.ClientKey, "service_id", run.ServiceID, "auth", map[bool]string{true: "required", false: "off"}[run.Settings.RequireAuth],
		"storage", storage)

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt, syscall.SIGTERM)
	select {
	case err := <-errs:
		if !errors.Is(err, http.ErrServerClosed) {
			return err
		}
	case <-stop:
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	return srv.Shutdown(ctx)
}

func checkHealth(port string) int {
	client := http.Client{Timeout: 3 * time.Second}
	resp, err := client.Get("http://127.0.0.1:" + port + "/health")
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return 1
	}
	return 0
}

func fail(err error) {
	fmt.Fprintln(os.Stderr, "orchard-sandbox:", err)
	os.Exit(1)
}
