package config

import (
	"os"
	"path/filepath"
	"testing"
)

func unsetEnv(t *testing.T, key string) {
	t.Helper()
	old, existed := os.LookupEnv(key)
	if err := os.Unsetenv(key); err != nil {
		t.Fatalf("unset %s: %v", key, err)
	}
	t.Cleanup(func() {
		if existed {
			_ = os.Setenv(key, old)
			return
		}
		_ = os.Unsetenv(key)
	})
}

func writeDotEnvFixture(t *testing.T, dir, content string) string {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, ".env"), []byte(content), 0o600); err != nil {
		t.Fatalf("write .env: %v", err)
	}
	configPath := filepath.Join(dir, "config.yaml")
	if err := os.WriteFile(configPath, []byte("storage:\n  path: ./trader.db\n"), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}
	return configPath
}

func TestLoadReadsDotEnv(t *testing.T) {
	clearEnv(t)
	unsetEnv(t, "MOEX_TRADER_TINKOFF_TOKEN")
	unsetEnv(t, "MOEX_TRADER_TINKOFF_ACCOUNT_ID")
	configPath := writeDotEnvFixture(t, t.TempDir(), `
# T-Invest token
export MOEX_TRADER_TINKOFF_TOKEN="sandbox-token"
MOEX_TRADER_TINKOFF_ACCOUNT_ID='12345'
`)

	cfg, err := Load(configPath)
	if err != nil {
		t.Fatalf("Load returned error: %v", err)
	}
	if cfg.Tinkoff.Token != "sandbox-token" {
		t.Fatalf("Token = %q, want sandbox-token", cfg.Tinkoff.Token)
	}
	if cfg.Tinkoff.AccountID != "12345" {
		t.Fatalf("AccountID = %q, want 12345", cfg.Tinkoff.AccountID)
	}
}

func TestLoadDotEnvDoesNotOverrideRealEnv(t *testing.T) {
	clearEnv(t)
	t.Setenv("MOEX_TRADER_TINKOFF_TOKEN", "real-env-token")
	configPath := writeDotEnvFixture(t, t.TempDir(), "MOEX_TRADER_TINKOFF_TOKEN=file-token\n")

	cfg, err := Load(configPath)
	if err != nil {
		t.Fatalf("Load returned error: %v", err)
	}
	if cfg.Tinkoff.Token != "real-env-token" {
		t.Fatalf("Token = %q, want real-env-token", cfg.Tinkoff.Token)
	}
}

func TestLoadDotEnvMissingFileIsFine(t *testing.T) {
	clearEnv(t)
	dir := t.TempDir()
	configPath := filepath.Join(dir, "config.yaml")
	if err := os.WriteFile(configPath, []byte("storage:\n  path: ./trader.db\n"), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}
	if _, err := Load(configPath); err != nil {
		t.Fatalf("Load returned error without .env: %v", err)
	}
}

func TestParseDotEnvLine(t *testing.T) {
	tests := []struct {
		line      string
		wantKey   string
		wantValue string
		wantOK    bool
	}{
		{line: "KEY=value", wantKey: "KEY", wantValue: "value", wantOK: true},
		{line: "KEY = value ", wantKey: "KEY", wantValue: "value", wantOK: true},
		{line: "export KEY=value", wantKey: "KEY", wantValue: "value", wantOK: true},
		{line: `KEY="quoted value"`, wantKey: "KEY", wantValue: "quoted value", wantOK: true},
		{line: "KEY='quoted value'", wantKey: "KEY", wantValue: "quoted value", wantOK: true},
		{line: "KEY=", wantKey: "KEY", wantValue: "", wantOK: true},
		{line: "# comment", wantOK: false},
		{line: "", wantOK: false},
		{line: "NOT_A_PAIR", wantOK: false},
		{line: "=value", wantOK: false},
	}
	for _, tt := range tests {
		key, value, ok := parseDotEnvLine(tt.line)
		if ok != tt.wantOK {
			t.Fatalf("parseDotEnvLine(%q) ok = %v, want %v", tt.line, ok, tt.wantOK)
		}
		if !ok {
			continue
		}
		if key != tt.wantKey || value != tt.wantValue {
			t.Fatalf("parseDotEnvLine(%q) = (%q, %q), want (%q, %q)", tt.line, key, value, tt.wantKey, tt.wantValue)
		}
	}
}
