package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadAppliesFileThenEnvironment(t *testing.T) {
	path := filepath.Join(t.TempDir(), "sandbox.yaml")
	err := os.WriteFile(path, []byte(`
port: "9000"
credentials:
  client_key: file_key
  secret_key: file_secret
settings:
  settle_mode: manual
  settle_delay_ms: 100
balances:
  payout: 12.5
cards:
  - id_num: GHA-000000001-1
    name: Esi Ansah
accounts:
  - bank_code: GCB
    account_number: "1234"
    account_name: Esi Ansah
`), 0o600)
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv("ORCHARD_SANDBOX_CLIENT_KEY", "env_key")
	t.Setenv("ORCHARD_SANDBOX_REQUIRE_AUTH", "false")

	cfg, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	switch {
	case cfg.Port != "9000", cfg.Credentials.ClientKey != "env_key", cfg.Credentials.SecretKey != "file_secret":
		t.Fatalf("file and env not merged: %+v", cfg)
	case cfg.Credentials.ServiceID != "1234", cfg.Settings.RequireAuth, cfg.Settings.SettleMode != "manual":
		t.Fatalf("settings not merged: %+v", cfg)
	case cfg.Balances.Ledger()["payout"] != 1250, cfg.Balances.Ledger()["sms"] != 1000:
		t.Fatalf("balances not merged: %+v", cfg.Balances)
	case len(cfg.Cards) != 1 || cfg.Cards[0].Name != "Esi Ansah", len(cfg.Accounts) != 1:
		t.Fatalf("seed data not loaded: %+v %+v", cfg.Cards, cfg.Accounts)
	}
}

func TestLoadRejectsInvalidEnvironment(t *testing.T) {
	t.Setenv("ORCHARD_SANDBOX_SETTLE_DELAY_MS", "soon")
	if _, err := Load(""); err == nil {
		t.Fatal("expected an error for a non-numeric delay")
	}
}
