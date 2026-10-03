// Package config loads sandbox configuration from defaults, an optional YAML file, and environment variables.
package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/savekirk/orchard-sandbox/internal/model"
)

// Config is the complete sandbox configuration. It seeds the default sandbox.
type Config struct {
	Port        string          `yaml:"port"`
	DB          string          `yaml:"db"`
	PublicURL   string          `yaml:"public_url"`
	Credentials Credentials     `yaml:"credentials"`
	Settings    model.Settings  `yaml:"settings"`
	Balances    Balances        `yaml:"balances"`
	Cards       []model.Card    `yaml:"cards"`
	Accounts    []model.Account `yaml:"accounts"`
}

// Credentials are the API keys and service ID of the default sandbox.
type Credentials struct {
	ClientKey string `yaml:"client_key"`
	SecretKey string `yaml:"secret_key"`
	ServiceID string `yaml:"service_id"`
}

// Balances are opening wallet balances in GHS (SMS in units).
type Balances struct {
	Collect float64 `yaml:"collect" json:"collect"`
	Payout  float64 `yaml:"payout" json:"payout"`
	Billpay float64 `yaml:"billpay" json:"billpay"`
	Airtime float64 `yaml:"airtime" json:"airtime"`
	SMS     int64   `yaml:"sms" json:"sms"`
}

// Ledger converts balances into per-account amounts in pesewas (SMS in units).
func (b Balances) Ledger() map[string]int64 {
	pesewas := func(v float64) int64 {
		p, err := model.ParseAmount(strconv.FormatFloat(v, 'f', -1, 64))
		if err != nil {
			return 0
		}
		return p
	}
	return map[string]int64{
		model.AccountAvailableCollect: pesewas(b.Collect),
		model.AccountActualCollect:    pesewas(b.Collect),
		model.AccountPayout:           pesewas(b.Payout),
		model.AccountBillpay:          pesewas(b.Billpay),
		model.AccountAirtime:          pesewas(b.Airtime),
		model.AccountSMS:              max(b.SMS, 0),
	}
}

// Default returns the built-in configuration.
func Default() Config {
	return Config{
		Port: "8080",
		Credentials: Credentials{
			ClientKey: "test_client_key",
			SecretKey: "test_secret_key",
			ServiceID: "1234",
		},
		Settings: model.Settings{
			RequireAuth:       true,
			SettleMode:        model.SettleAuto,
			SettleDelayMs:     3000,
			CallbackAttempts:  3,
			CallbackTimeoutMs: 10000,
		},
		Balances: Balances{Collect: 75000, Payout: 50000, Billpay: 10000, Airtime: 5000, SMS: 1000},
	}
}

// Load builds the configuration: defaults, then the YAML file at path (if any), then environment variables.
func Load(path string) (Config, error) {
	cfg := Default()
	if path == "" {
		path = os.Getenv("ORCHARD_SANDBOX_CONFIG")
	}
	if path != "" {
		raw, err := os.ReadFile(path)
		if err != nil {
			return cfg, fmt.Errorf("read config: %w", err)
		}
		if err := yaml.Unmarshal(raw, &cfg); err != nil {
			return cfg, fmt.Errorf("parse config %s: %w", path, err)
		}
	}
	if err := applyEnv(&cfg); err != nil {
		return cfg, err
	}
	cfg.Settings = cfg.Settings.Normalize()
	return cfg, cfg.validate()
}

func applyEnv(cfg *Config) error {
	str := func(key string, dst *string) {
		if v, ok := os.LookupEnv(key); ok && v != "" {
			*dst = v
		}
	}
	var errs []string
	integer := func(key string, dst *int) {
		if v, ok := os.LookupEnv(key); ok && v != "" {
			n, err := strconv.Atoi(v)
			if err != nil {
				errs = append(errs, fmt.Sprintf("%s must be a number, got %q", key, v))
				return
			}
			*dst = n
		}
	}
	boolean := func(key string, dst *bool) {
		if v, ok := os.LookupEnv(key); ok && v != "" {
			b, err := strconv.ParseBool(v)
			if err != nil {
				errs = append(errs, fmt.Sprintf("%s must be true or false, got %q", key, v))
				return
			}
			*dst = b
		}
	}

	str("PORT", &cfg.Port)
	str("ORCHARD_SANDBOX_PORT", &cfg.Port)
	str("ORCHARD_SANDBOX_DB", &cfg.DB)
	str("ORCHARD_SANDBOX_PUBLIC_URL", &cfg.PublicURL)
	str("ORCHARD_SANDBOX_CLIENT_KEY", &cfg.Credentials.ClientKey)
	str("ORCHARD_SANDBOX_SECRET_KEY", &cfg.Credentials.SecretKey)
	str("ORCHARD_SANDBOX_SERVICE_ID", &cfg.Credentials.ServiceID)
	boolean("ORCHARD_SANDBOX_REQUIRE_AUTH", &cfg.Settings.RequireAuth)
	str("ORCHARD_SANDBOX_SETTLE_MODE", &cfg.Settings.SettleMode)
	integer("ORCHARD_SANDBOX_SETTLE_DELAY_MS", &cfg.Settings.SettleDelayMs)
	integer("ORCHARD_SANDBOX_LATENCY_MS", &cfg.Settings.LatencyMs)
	integer("ORCHARD_SANDBOX_CALLBACK_ATTEMPTS", &cfg.Settings.CallbackAttempts)

	if len(errs) > 0 {
		return fmt.Errorf("invalid environment: %s", strings.Join(errs, "; "))
	}
	return nil
}

func (c Config) validate() error {
	switch {
	case c.Credentials.ClientKey == "" || c.Credentials.SecretKey == "":
		return fmt.Errorf("credentials.client_key and credentials.secret_key are required")
	case c.Credentials.ServiceID == "":
		return fmt.Errorf("credentials.service_id is required")
	}
	for _, card := range c.Cards {
		if card.IDNum == "" {
			return fmt.Errorf("every card needs an id_num")
		}
	}
	for _, a := range c.Accounts {
		if a.BankCode == "" || a.AccountNumber == "" {
			return fmt.Errorf("every account needs a bank_code and account_number")
		}
	}
	return nil
}
