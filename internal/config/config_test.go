package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func clearEnv(t *testing.T) {
	t.Helper()
	for _, key := range []string{
		"MOEX_TRADER_OLLAMA_HOST",
		"MOEX_TRADER_OLLAMA_MODEL",
		"MOEX_TRADER_OLLAMA_TIMEOUT",
		"MOEX_TRADER_MOEX_ISS_URL",
		"MOEX_TRADER_ALGOPACK_BASE_URL",
		"MOEX_TRADER_ALGOPACK_TOKEN",
		"MOEX_TRADER_STORAGE_PATH",
		"MOEX_TRADER_POLL_INTERVAL",
		"MOEX_TRADER_IS_PAPER_TRADING",
		"MOEX_TRADER_TICKERS",
		"MOEX_TRADER_RISK_MAX_LOTS",
		"MOEX_TRADER_TELEGRAM_BOT_TOKEN",
		"MOEX_TRADER_TELEGRAM_CHAT_ID",
	} {
		t.Setenv(key, "")
	}
}

func writeConfig(t *testing.T, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "config.yaml")
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("write fixture config: %v", err)
	}
	return path
}

func TestLoadValidFile(t *testing.T) {
	clearEnv(t)
	path := writeConfig(t, `
tickers: [SBER, YDEX]
ollama:
  host: "localhost:11434"
  model: "qwen3.8:latest"
  timeout: 15s
moex_iss_base_url: "https://iss.moex.com/iss"
storage:
  path: "/tmp/trader.db"
is_paper_trading: false
poll_interval: 1m
`)
	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load returned error: %v", err)
	}
	if len(cfg.Tickers) != 2 || cfg.Tickers[0] != "SBER" || cfg.Tickers[1] != "YDEX" {
		t.Fatalf("unexpected tickers: %v", cfg.Tickers)
	}
	if cfg.Ollama.Host != "localhost:11434" {
		t.Fatalf("unexpected ollama host: %q", cfg.Ollama.Host)
	}
	if cfg.Ollama.Model != "qwen3.8:latest" {
		t.Fatalf("unexpected ollama model: %q", cfg.Ollama.Model)
	}
	if cfg.Ollama.Timeout.Std() != 15*time.Second {
		t.Fatalf("unexpected ollama timeout: %s", cfg.Ollama.Timeout.Std())
	}
	if cfg.MOEXISSBaseURL != "https://iss.moex.com/iss" {
		t.Fatalf("unexpected moex base url: %q", cfg.MOEXISSBaseURL)
	}
	if cfg.Storage.Path != "/tmp/trader.db" {
		t.Fatalf("unexpected storage path: %q", cfg.Storage.Path)
	}
	if cfg.IsPaperTrading {
		t.Fatal("expected is_paper_trading to be false")
	}
	if cfg.PollInterval.Std() != time.Minute {
		t.Fatalf("unexpected poll interval: %s", cfg.PollInterval.Std())
	}
}

func TestLoadMissingFile(t *testing.T) {
	clearEnv(t)
	_, err := Load(filepath.Join(t.TempDir(), "does-not-exist.yaml"))
	if err == nil {
		t.Fatal("expected error for missing file")
	}
}

func TestParseMalformedYAML(t *testing.T) {
	clearEnv(t)
	_, err := Parse([]byte("tickers: [unclosed\n"))
	if err == nil {
		t.Fatal("expected error for malformed YAML")
	}
}

func TestParseMissingRequiredField(t *testing.T) {
	clearEnv(t)
	_, err := Parse([]byte("tickers: [SBER]\n"))
	if err == nil {
		t.Fatal("expected error for missing required field")
	}
	if !strings.Contains(err.Error(), "storage.path") {
		t.Fatalf("expected storage.path error, got: %v", err)
	}
}

func TestParseEmptyTickers(t *testing.T) {
	clearEnv(t)
	_, err := Parse([]byte("tickers: []\nstorage:\n  path: ./trader.db\n"))
	if err == nil {
		t.Fatal("expected error for empty tickers")
	}
	if !strings.Contains(err.Error(), "tickers") {
		t.Fatalf("expected tickers error, got: %v", err)
	}
}

func TestDefaultsAppliedWhenFieldsOmitted(t *testing.T) {
	clearEnv(t)
	cfg, err := Parse([]byte("storage:\n  path: ./trader.db\n"))
	if err != nil {
		t.Fatalf("Parse returned error: %v", err)
	}
	if len(cfg.Tickers) != len(DefaultTickers()) {
		t.Fatalf("expected default tickers, got %d: %v", len(cfg.Tickers), cfg.Tickers)
	}
	if cfg.Ollama.Host != "192.168.88.193:11434" {
		t.Fatalf("unexpected default ollama host: %q", cfg.Ollama.Host)
	}
	if cfg.Ollama.Model != "qwen3.8" {
		t.Fatalf("unexpected default ollama model: %q", cfg.Ollama.Model)
	}
	if cfg.Ollama.Timeout.Std() != 10*time.Second {
		t.Fatalf("unexpected default ollama timeout: %s", cfg.Ollama.Timeout.Std())
	}
	if cfg.MOEXISSBaseURL != "https://iss.moex.com/iss" {
		t.Fatalf("unexpected default moex base url: %q", cfg.MOEXISSBaseURL)
	}
	if cfg.AlgoPackBaseURL != "https://apim.moex.com/iss/datashop" {
		t.Fatalf("unexpected default algopack base url: %q", cfg.AlgoPackBaseURL)
	}
	if cfg.AlgoPackToken != "" {
		t.Fatalf("unexpected default algopack token: %q", cfg.AlgoPackToken)
	}
	if !cfg.IsPaperTrading {
		t.Fatal("expected default is_paper_trading to be true")
	}
	if cfg.PollInterval.Std() != 5*time.Minute {
		t.Fatalf("unexpected default poll interval: %s", cfg.PollInterval.Std())
	}
	if cfg.Risk.MaxLots != 1 {
		t.Fatalf("unexpected default risk max lots: %d", cfg.Risk.MaxLots)
	}
}

func TestRiskMaxLotsLoadedAndValidated(t *testing.T) {
	clearEnv(t)
	cfg, err := Parse([]byte("storage:\n  path: ./trader.db\nrisk:\n  max_lots: 3\n"))
	if err != nil {
		t.Fatalf("Parse returned error: %v", err)
	}
	if cfg.Risk.MaxLots != 3 {
		t.Fatalf("unexpected risk max lots: %d", cfg.Risk.MaxLots)
	}

	_, err = Parse([]byte("storage:\n  path: ./trader.db\nrisk:\n  max_lots: 0\n"))
	if err == nil {
		t.Fatal("expected error for non-positive risk max lots")
	}
	if !strings.Contains(err.Error(), "risk.max_lots") {
		t.Fatalf("expected risk.max_lots error, got: %v", err)
	}
}

func TestEnvOverrides(t *testing.T) {
	clearEnv(t)
	t.Setenv("MOEX_TRADER_OLLAMA_HOST", "override:9999")
	t.Setenv("MOEX_TRADER_OLLAMA_MODEL", "custom-model")
	t.Setenv("MOEX_TRADER_OLLAMA_TIMEOUT", "25s")
	t.Setenv("MOEX_TRADER_MOEX_ISS_URL", "https://override.example/iss")
	t.Setenv("MOEX_TRADER_ALGOPACK_BASE_URL", "https://override.example/datashop")
	t.Setenv("MOEX_TRADER_ALGOPACK_TOKEN", "env-algopack-token")
	t.Setenv("MOEX_TRADER_STORAGE_PATH", "/override/trader.db")
	t.Setenv("MOEX_TRADER_POLL_INTERVAL", "2m")
	t.Setenv("MOEX_TRADER_IS_PAPER_TRADING", "false")
	t.Setenv("MOEX_TRADER_TICKERS", "SBER, YDEX , OZON")

	cfg, err := Parse([]byte("storage:\n  path: ./trader.db\n"))
	if err != nil {
		t.Fatalf("Parse returned error: %v", err)
	}
	if cfg.Ollama.Host != "override:9999" {
		t.Fatalf("unexpected ollama host: %q", cfg.Ollama.Host)
	}
	if cfg.Ollama.Model != "custom-model" {
		t.Fatalf("unexpected ollama model: %q", cfg.Ollama.Model)
	}
	if cfg.Ollama.Timeout.Std() != 25*time.Second {
		t.Fatalf("unexpected ollama timeout: %s", cfg.Ollama.Timeout.Std())
	}
	if cfg.MOEXISSBaseURL != "https://override.example/iss" {
		t.Fatalf("unexpected moex base url: %q", cfg.MOEXISSBaseURL)
	}
	if cfg.AlgoPackBaseURL != "https://override.example/datashop" {
		t.Fatalf("unexpected algopack base url: %q", cfg.AlgoPackBaseURL)
	}
	if cfg.AlgoPackToken != "env-algopack-token" {
		t.Fatalf("unexpected algopack token: %q", cfg.AlgoPackToken)
	}
	if cfg.Storage.Path != "/override/trader.db" {
		t.Fatalf("unexpected storage path: %q", cfg.Storage.Path)
	}
	if cfg.PollInterval.Std() != 2*time.Minute {
		t.Fatalf("unexpected poll interval: %s", cfg.PollInterval.Std())
	}
	if cfg.IsPaperTrading {
		t.Fatal("expected env override to disable paper trading")
	}
	want := []string{"SBER", "YDEX", "OZON"}
	if len(cfg.Tickers) != len(want) {
		t.Fatalf("unexpected tickers: %v", cfg.Tickers)
	}
	for i := range want {
		if cfg.Tickers[i] != want[i] {
			t.Fatalf("unexpected ticker at %d: %q", i, cfg.Tickers[i])
		}
	}
}

func TestRiskMaxLotsEnvOverride(t *testing.T) {
	clearEnv(t)
	t.Setenv("MOEX_TRADER_RISK_MAX_LOTS", "7")

	cfg, err := Parse([]byte("storage:\n  path: ./trader.db\n"))
	if err != nil {
		t.Fatalf("Parse returned error: %v", err)
	}
	if cfg.Risk.MaxLots != 7 {
		t.Fatalf("Risk.MaxLots = %d, want 7", cfg.Risk.MaxLots)
	}
}

func TestInvalidEnvDuration(t *testing.T) {
	clearEnv(t)
	t.Setenv("MOEX_TRADER_POLL_INTERVAL", "not-a-duration")
	_, err := Parse([]byte("storage:\n  path: ./trader.db\n"))
	if err == nil {
		t.Fatal("expected error for invalid env duration")
	}
}

func TestTelegramConfigOptional(t *testing.T) {
	clearEnv(t)
	cfg, err := Parse([]byte("storage:\n  path: ./trader.db\n"))
	if err != nil {
		t.Fatalf("Parse returned error: %v", err)
	}
	if cfg.Telegram.BotToken != "" || cfg.Telegram.ChatID != "" {
		t.Fatalf("unexpected telegram defaults: %+v", cfg.Telegram)
	}

	cfg, err = Parse([]byte("storage:\n  path: ./trader.db\ntelegram:\n  bot_token: tok\n  chat_id: chat\n"))
	if err != nil {
		t.Fatalf("Parse returned error: %v", err)
	}
	if cfg.Telegram.BotToken != "" || cfg.Telegram.ChatID != "chat" {
		t.Fatalf("unexpected telegram config: %+v", cfg.Telegram)
	}
}

func TestSecretYAMLFieldsAreIgnored(t *testing.T) {
	clearEnv(t)
	cfg, err := Parse([]byte("storage:\n  path: ./trader.db\nalgopack_token: yaml-token\ntelegram:\n  bot_token: yaml-bot-token\n"))
	if err != nil {
		t.Fatalf("Parse returned error: %v", err)
	}
	if cfg.AlgoPackToken != "" {
		t.Fatalf("AlgoPackToken = %q, want empty because secrets are env-only", cfg.AlgoPackToken)
	}
	if cfg.Telegram.BotToken != "" {
		t.Fatalf("Telegram.BotToken = %q, want empty because secrets are env-only", cfg.Telegram.BotToken)
	}
}

func TestTelegramEnvOverrides(t *testing.T) {
	clearEnv(t)
	t.Setenv("MOEX_TRADER_TELEGRAM_BOT_TOKEN", "env-token")
	t.Setenv("MOEX_TRADER_TELEGRAM_CHAT_ID", "env-chat")

	cfg, err := Parse([]byte("storage:\n  path: ./trader.db\n"))
	if err != nil {
		t.Fatalf("Parse returned error: %v", err)
	}
	if cfg.Telegram.BotToken != "env-token" {
		t.Fatalf("unexpected bot token: %q", cfg.Telegram.BotToken)
	}
	if cfg.Telegram.ChatID != "env-chat" {
		t.Fatalf("unexpected chat id: %q", cfg.Telegram.ChatID)
	}
}
