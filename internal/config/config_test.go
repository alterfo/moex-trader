package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/shopspring/decimal"
)

func clearEnv(t *testing.T) {
	t.Helper()
	for _, key := range []string{
		"MOEX_TRADER_MODEL_PATH",
		"MOEX_TRADER_MOEX_ISS_URL",
		"MOEX_TRADER_ALGOPACK_BASE_URL",
		"MOEX_TRADER_ALGOPACK_TOKEN",
		"MOEX_TRADER_FINAM_BASE_URL",
		"MOEX_TRADER_FINAM_SECRET_TOKEN",
		"MOEX_TRADER_TINKOFF_ENDPOINT",
		"MOEX_TRADER_TINKOFF_TOKEN",
		"MOEX_TRADER_TINKOFF_SANDBOX",
		"MOEX_TRADER_TINKOFF_ACCOUNT_ID",
		"MOEX_TRADER_TINKOFF_PAY_IN",
		"MOEX_TRADER_TINKOFF_ORDER_TYPE",
		"MOEX_TRADER_PREFLIGHT_ENABLED",
		"MOEX_TRADER_PREFLIGHT_DAYS",
		"MOEX_TRADER_PREFLIGHT_DEPOSIT",
		"MOEX_TRADER_PREFLIGHT_MIN_NET_PNL",
		"MOEX_TRADER_STORAGE_PATH",
		"MOEX_TRADER_POLL_INTERVAL",
		"MOEX_TRADER_IS_PAPER_TRADING",
		"MOEX_TRADER_BROKER",
		"MOEX_TRADER_TICKERS",
		"MOEX_TRADER_RISK_MAX_LOTS",
		"MOEX_TRADER_TELEGRAM_BOT_TOKEN",
		"MOEX_TRADER_TELEGRAM_CHAT_ID",
		"MOEX_TRADER_TELEGRAM_SIGNAL_TICKERS",
		"MOEX_TRADER_TELEGRAM_PROXY",
		"MOEX_TRADER_NEWS_PROXY",
		"MOEX_TRADER_NEWS_HISTORY_PATH",
		"MOEX_TRADER_NEWS_RAW_PATH",
		"MOEX_TRADER_COMMISSION_RATE",
		"MOEX_TRADER_COMMISSION_BROKER",
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
	if cfg.Model.Path != "model.json" {
		t.Fatalf("unexpected default model path: %q", cfg.Model.Path)
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
	if cfg.Finam.BaseURL != "https://api.finam.ru" {
		t.Fatalf("unexpected default finam base url: %q", cfg.Finam.BaseURL)
	}
	if cfg.Finam.SecretToken != "" {
		t.Fatalf("unexpected default finam secret token: %q", cfg.Finam.SecretToken)
	}
	if !cfg.IsPaperTrading {
		t.Fatal("expected default is_paper_trading to be true")
	}
	if cfg.Broker != BrokerPaper {
		t.Fatalf("unexpected default broker: %q", cfg.Broker)
	}
	if cfg.PollInterval.Std() != 5*time.Minute {
		t.Fatalf("unexpected default poll interval: %s", cfg.PollInterval.Std())
	}
	if cfg.Risk.MaxLots != 1 {
		t.Fatalf("unexpected default risk max lots: %d", cfg.Risk.MaxLots)
	}
	if cfg.Commission.Broker != "tinkoff" {
		t.Fatalf("unexpected default commission broker: %q", cfg.Commission.Broker)
	}
	if !cfg.Commission.Rate.Equal(decimal.New(5, -4)) {
		t.Fatalf("unexpected default commission rate: %s", cfg.Commission.Rate)
	}
	if cfg.Tinkoff.Endpoint != "sandbox-invest-public-api.tbank.ru:443" {
		t.Fatalf("unexpected default tinkoff endpoint: %q", cfg.Tinkoff.Endpoint)
	}
	if cfg.Tinkoff.Sandbox {
		t.Fatal("expected tinkoff.sandbox to default to false")
	}
	if cfg.Tinkoff.OrderType != OrderTypeLimit {
		t.Fatalf("unexpected default tinkoff order type: %q", cfg.Tinkoff.OrderType)
	}
	if cfg.Tinkoff.Token != "" {
		t.Fatalf("unexpected default tinkoff token: %q", cfg.Tinkoff.Token)
	}
	if !cfg.Preflight.Enabled {
		t.Fatal("expected preflight to be enabled by default")
	}
	if cfg.Preflight.Days != 90 {
		t.Fatalf("unexpected default preflight days: %d", cfg.Preflight.Days)
	}
	if !cfg.Preflight.Deposit.Equal(decimal.NewFromInt(100_000)) {
		t.Fatalf("unexpected default preflight deposit: %s", cfg.Preflight.Deposit)
	}
	if !cfg.Preflight.MinNetPnL.IsZero() {
		t.Fatalf("unexpected default preflight min net pnl: %s", cfg.Preflight.MinNetPnL)
	}
}

func TestModelPathLoadedAndDefaults(t *testing.T) {
	clearEnv(t)
	cfg, err := Parse([]byte("storage:\n  path: ./trader.db\nmodel:\n  path: ./weights.json\n"))
	if err != nil {
		t.Fatalf("Parse returned error: %v", err)
	}
	if cfg.Model.Path != "./weights.json" {
		t.Fatalf("Model.Path = %q, want ./weights.json", cfg.Model.Path)
	}
}

func TestModelPathEnvOverride(t *testing.T) {
	clearEnv(t)
	t.Setenv("MOEX_TRADER_MODEL_PATH", "./env-model.json")

	cfg, err := Parse([]byte("storage:\n  path: ./trader.db\n"))
	if err != nil {
		t.Fatalf("Parse returned error: %v", err)
	}
	if cfg.Model.Path != "./env-model.json" {
		t.Fatalf("Model.Path = %q, want ./env-model.json", cfg.Model.Path)
	}
}

func TestModelPathEmptyFailsValidation(t *testing.T) {
	clearEnv(t)
	_, err := Parse([]byte("storage:\n  path: ./trader.db\nmodel:\n  path: \"\"\n"))
	if err == nil {
		t.Fatal("Parse returned nil error for empty model.path")
	}
	if !strings.Contains(err.Error(), "model.path") {
		t.Fatalf("Parse error = %v, want model.path validation message", err)
	}
}

func TestCommissionLoaded(t *testing.T) {
	clearEnv(t)
	cfg, err := Parse([]byte("storage:\n  path: ./trader.db\ncommission:\n  broker: tinkoff\n  rate: \"0.0015\"\n"))
	if err != nil {
		t.Fatalf("Parse returned error: %v", err)
	}
	if cfg.Commission.Broker != "tinkoff" {
		t.Fatalf("Commission.Broker = %q, want tinkoff", cfg.Commission.Broker)
	}
	want := decimal.RequireFromString("0.0015")
	if !cfg.Commission.Rate.Equal(want) {
		t.Fatalf("Commission.Rate = %s, want %s", cfg.Commission.Rate, want)
	}
}

func TestCommissionMissingSectionFallsBackToDefault(t *testing.T) {
	clearEnv(t)
	cfg, err := Parse([]byte("storage:\n  path: ./trader.db\n"))
	if err != nil {
		t.Fatalf("Parse returned error: %v", err)
	}
	if cfg.Commission.Broker != "tinkoff" {
		t.Fatalf("Commission.Broker = %q, want tinkoff", cfg.Commission.Broker)
	}
	if !cfg.Commission.Rate.Equal(decimal.New(5, -4)) {
		t.Fatalf("Commission.Rate = %s, want %s", cfg.Commission.Rate, decimal.New(5, -4))
	}
}

func TestCommissionInvalidRateYAML(t *testing.T) {
	clearEnv(t)
	_, err := Parse([]byte("storage:\n  path: ./trader.db\ncommission:\n  rate: \"not-a-decimal\"\n"))
	if err == nil {
		t.Fatal("expected error for invalid commission rate")
	}
}

func TestCommissionEnvOverride(t *testing.T) {
	clearEnv(t)
	t.Setenv("MOEX_TRADER_COMMISSION_RATE", "0.0025")

	cfg, err := Parse([]byte("storage:\n  path: ./trader.db\n"))
	if err != nil {
		t.Fatalf("Parse returned error: %v", err)
	}
	want := decimal.RequireFromString("0.0025")
	if !cfg.Commission.Rate.Equal(want) {
		t.Fatalf("Commission.Rate = %s, want %s", cfg.Commission.Rate, want)
	}
	if cfg.Commission.Broker != "tinkoff" {
		t.Fatalf("Commission.Broker = %q, want tinkoff", cfg.Commission.Broker)
	}
}

func TestCommissionInvalidEnvRate(t *testing.T) {
	clearEnv(t)
	t.Setenv("MOEX_TRADER_COMMISSION_RATE", "not-a-decimal")
	_, err := Parse([]byte("storage:\n  path: ./trader.db\n"))
	if err == nil {
		t.Fatal("expected error for invalid commission rate env var")
	}
	if !strings.Contains(err.Error(), "MOEX_TRADER_COMMISSION_RATE") {
		t.Fatalf("expected MOEX_TRADER_COMMISSION_RATE error, got: %v", err)
	}
}

func TestCommissionNegativeRateRejected(t *testing.T) {
	clearEnv(t)
	_, err := Parse([]byte("storage:\n  path: ./trader.db\ncommission:\n  rate: \"-0.01\"\n"))
	if err == nil {
		t.Fatal("expected error for negative commission rate")
	}
	if !strings.Contains(err.Error(), "commission.rate") {
		t.Fatalf("expected commission.rate error, got: %v", err)
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
	t.Setenv("MOEX_TRADER_MOEX_ISS_URL", "https://override.example/iss")
	t.Setenv("MOEX_TRADER_ALGOPACK_BASE_URL", "https://override.example/datashop")
	t.Setenv("MOEX_TRADER_ALGOPACK_TOKEN", "env-algopack-token")
	t.Setenv("MOEX_TRADER_FINAM_BASE_URL", "https://override.example/finam")
	t.Setenv("MOEX_TRADER_FINAM_SECRET_TOKEN", "env-finam-token")
	t.Setenv("MOEX_TRADER_STORAGE_PATH", "/override/trader.db")
	t.Setenv("MOEX_TRADER_POLL_INTERVAL", "2m")
	t.Setenv("MOEX_TRADER_IS_PAPER_TRADING", "false")
	t.Setenv("MOEX_TRADER_TICKERS", "SBER, YDEX , OZON")

	cfg, err := Parse([]byte("storage:\n  path: ./trader.db\n"))
	if err != nil {
		t.Fatalf("Parse returned error: %v", err)
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
	if cfg.Finam.BaseURL != "https://override.example/finam" {
		t.Fatalf("unexpected finam base url: %q", cfg.Finam.BaseURL)
	}
	if cfg.Finam.SecretToken != "env-finam-token" {
		t.Fatalf("unexpected finam secret token: %q", cfg.Finam.SecretToken)
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
	t.Setenv("MOEX_TRADER_TELEGRAM_SIGNAL_TICKERS", "POSI, SBER")
	t.Setenv("MOEX_TRADER_TELEGRAM_PROXY", "socks5://127.0.0.1:3333")

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
	if len(cfg.Telegram.SignalTickers) != 2 || cfg.Telegram.SignalTickers[0] != "POSI" || cfg.Telegram.SignalTickers[1] != "SBER" {
		t.Fatalf("unexpected signal tickers: %v", cfg.Telegram.SignalTickers)
	}
	if cfg.Telegram.Proxy != "socks5://127.0.0.1:3333" {
		t.Fatalf("unexpected telegram proxy: %q", cfg.Telegram.Proxy)
	}
}

func TestTelegramSignalTickersLoaded(t *testing.T) {
	clearEnv(t)
	cfg, err := Parse([]byte("storage:\n  path: ./trader.db\ntelegram:\n  chat_id: \"123\"\n  signal_tickers: [POSI, SBER]\n"))
	if err != nil {
		t.Fatalf("Parse returned error: %v", err)
	}
	if len(cfg.Telegram.SignalTickers) != 2 || cfg.Telegram.SignalTickers[0] != "POSI" {
		t.Fatalf("unexpected signal tickers: %v", cfg.Telegram.SignalTickers)
	}
}

func TestNewsDefaultsApplied(t *testing.T) {
	clearEnv(t)
	cfg, err := Parse([]byte("storage:\n  path: ./trader.db\n"))
	if err != nil {
		t.Fatalf("Parse returned error: %v", err)
	}
	if !cfg.News.VetoEnabled {
		t.Fatal("expected news veto to default to enabled")
	}
	if !cfg.News.VetoSentiment.Equal(decimal.NewFromFloat(0.5)) {
		t.Fatalf("unexpected default veto sentiment: %s", cfg.News.VetoSentiment)
	}
	if cfg.News.VetoMinCount != 1 {
		t.Fatalf("unexpected default veto min count: %d", cfg.News.VetoMinCount)
	}
}

func TestNewsVetoLoadedAndValidated(t *testing.T) {
	clearEnv(t)
	cfg, err := Parse([]byte("storage:\n  path: ./trader.db\nnews:\n  veto_enabled: false\n  veto_sentiment: \"0.7\"\n  veto_min_count: 2\n  proxy: \"socks5://127.0.0.1:3333\"\n"))
	if err != nil {
		t.Fatalf("Parse returned error: %v", err)
	}
	if cfg.News.VetoEnabled {
		t.Fatal("expected news veto to be disabled")
	}
	if !cfg.News.VetoSentiment.Equal(decimal.NewFromFloat(0.7)) {
		t.Fatalf("unexpected veto sentiment: %s", cfg.News.VetoSentiment)
	}
	if cfg.News.VetoMinCount != 2 {
		t.Fatalf("unexpected veto min count: %d", cfg.News.VetoMinCount)
	}
	if cfg.News.Proxy != "socks5://127.0.0.1:3333" {
		t.Fatalf("unexpected news proxy: %q", cfg.News.Proxy)
	}

	_, err = Parse([]byte("storage:\n  path: ./trader.db\nnews:\n  veto_sentiment: \"1.5\"\n"))
	if err == nil {
		t.Fatal("expected error for veto_sentiment > 1")
	}
}

func TestNewsEnvOverrides(t *testing.T) {
	clearEnv(t)
	t.Setenv("MOEX_TRADER_NEWS_PROXY", "socks5://127.0.0.1:3333")
	t.Setenv("MOEX_TRADER_NEWS_HISTORY_PATH", "data/history.jsonl")
	t.Setenv("MOEX_TRADER_NEWS_RAW_PATH", "data/raw.jsonl")

	cfg, err := Parse([]byte("storage:\n  path: ./trader.db\n"))
	if err != nil {
		t.Fatalf("Parse returned error: %v", err)
	}
	if cfg.News.Proxy != "socks5://127.0.0.1:3333" {
		t.Fatalf("unexpected news proxy: %q", cfg.News.Proxy)
	}
	if cfg.News.HistoryPath != "data/history.jsonl" {
		t.Fatalf("unexpected history path: %q", cfg.News.HistoryPath)
	}
	if cfg.News.RawPath != "data/raw.jsonl" {
		t.Fatalf("unexpected raw path: %q", cfg.News.RawPath)
	}
}

func TestBrokerLoadedAndValidated(t *testing.T) {
	clearEnv(t)
	for _, broker := range []string{BrokerPaper, BrokerTinkoff, BrokerFinam} {
		cfg, err := Parse([]byte("storage:\n  path: ./trader.db\nbroker: \"" + broker + "\"\n"))
		if err != nil {
			t.Fatalf("Parse(%q) returned error: %v", broker, err)
		}
		if cfg.Broker != broker {
			t.Fatalf("Broker = %q, want %q", cfg.Broker, broker)
		}
	}

	_, err := Parse([]byte("storage:\n  path: ./trader.db\nbroker: alfa\n"))
	if err == nil {
		t.Fatal("expected error for unknown broker")
	}
	if !strings.Contains(err.Error(), "broker must be one of") {
		t.Fatalf("expected broker validation error, got: %v", err)
	}
}

func TestBrokerEnvOverride(t *testing.T) {
	clearEnv(t)
	t.Setenv("MOEX_TRADER_BROKER", BrokerFinam)

	cfg, err := Parse([]byte("storage:\n  path: ./trader.db\n"))
	if err != nil {
		t.Fatalf("Parse returned error: %v", err)
	}
	if cfg.Broker != BrokerFinam {
		t.Fatalf("Broker = %q, want %q", cfg.Broker, BrokerFinam)
	}
}

func TestFinamConfigLoaded(t *testing.T) {
	clearEnv(t)
	cfg, err := Parse([]byte("storage:\n  path: ./trader.db\nfinam:\n  base_url: \"https://finam.example\"\n"))
	if err != nil {
		t.Fatalf("Parse returned error: %v", err)
	}
	if cfg.Finam.BaseURL != "https://finam.example" {
		t.Fatalf("Finam.BaseURL = %q, want https://finam.example", cfg.Finam.BaseURL)
	}
	if cfg.Finam.SecretToken != "" {
		t.Fatalf("Finam.SecretToken = %q, want empty because secrets are env-only", cfg.Finam.SecretToken)
	}
}

func TestFinamEmptyBaseURLRejected(t *testing.T) {
	clearEnv(t)
	_, err := Parse([]byte("storage:\n  path: ./trader.db\nfinam:\n  base_url: \"\"\n"))
	if err == nil {
		t.Fatal("expected error for empty finam base url")
	}
	if !strings.Contains(err.Error(), "finam.base_url") {
		t.Fatalf("expected finam.base_url error, got: %v", err)
	}
}

func TestFinamSecretYAMLIgnored(t *testing.T) {
	clearEnv(t)
	cfg, err := Parse([]byte("storage:\n  path: ./trader.db\nfinam:\n  secret_token: yaml-secret\n"))
	if err != nil {
		t.Fatalf("Parse returned error: %v", err)
	}
	if cfg.Finam.SecretToken != "" {
		t.Fatalf("Finam.SecretToken = %q, want empty because secrets are env-only", cfg.Finam.SecretToken)
	}
}

func TestCommissionBrokerEnvOverride(t *testing.T) {
	clearEnv(t)
	t.Setenv("MOEX_TRADER_COMMISSION_BROKER", "tinkoff")

	cfg, err := Parse([]byte("storage:\n  path: ./trader.db\n"))
	if err != nil {
		t.Fatalf("Parse returned error: %v", err)
	}
	if cfg.Commission.Broker != "tinkoff" {
		t.Fatalf("Commission.Broker = %q, want tinkoff", cfg.Commission.Broker)
	}
}

func TestTinkoffConfigLoaded(t *testing.T) {
	clearEnv(t)
	cfg, err := Parse([]byte(`
storage:
  path: ./trader.db
tinkoff:
  endpoint: "sandbox.example:443"
  sandbox: true
  account_id: "12345"
  pay_in: "100000"
  order_type: "market"
`))
	if err != nil {
		t.Fatalf("Parse returned error: %v", err)
	}
	if cfg.Tinkoff.Endpoint != "sandbox.example:443" {
		t.Fatalf("Endpoint = %q, want sandbox.example:443", cfg.Tinkoff.Endpoint)
	}
	if !cfg.Tinkoff.Sandbox {
		t.Fatal("Sandbox = false, want true")
	}
	if cfg.Tinkoff.AccountID != "12345" {
		t.Fatalf("AccountID = %q, want 12345", cfg.Tinkoff.AccountID)
	}
	if !cfg.Tinkoff.PayIn.Equal(decimal.RequireFromString("100000")) {
		t.Fatalf("PayIn = %s, want 100000", cfg.Tinkoff.PayIn)
	}
	if cfg.Tinkoff.OrderType != OrderTypeMarket {
		t.Fatalf("OrderType = %q, want market", cfg.Tinkoff.OrderType)
	}
	if cfg.Tinkoff.Token != "" {
		t.Fatalf("Token = %q, want empty because secrets are env-only", cfg.Tinkoff.Token)
	}
}

func TestTinkoffSecretYAMLIgnored(t *testing.T) {
	clearEnv(t)
	cfg, err := Parse([]byte("storage:\n  path: ./trader.db\ntinkoff:\n  token: yaml-token\n"))
	if err != nil {
		t.Fatalf("Parse returned error: %v", err)
	}
	if cfg.Tinkoff.Token != "" {
		t.Fatalf("Token = %q, want empty because secrets are env-only", cfg.Tinkoff.Token)
	}
}

func TestTinkoffEnvOverrides(t *testing.T) {
	clearEnv(t)
	t.Setenv("MOEX_TRADER_TINKOFF_ENDPOINT", "override.example:443")
	t.Setenv("MOEX_TRADER_TINKOFF_TOKEN", "env-token")
	t.Setenv("MOEX_TRADER_TINKOFF_SANDBOX", "true")
	t.Setenv("MOEX_TRADER_TINKOFF_ACCOUNT_ID", "env-account")
	t.Setenv("MOEX_TRADER_TINKOFF_PAY_IN", "50000")
	t.Setenv("MOEX_TRADER_TINKOFF_ORDER_TYPE", "market")

	cfg, err := Parse([]byte("storage:\n  path: ./trader.db\n"))
	if err != nil {
		t.Fatalf("Parse returned error: %v", err)
	}
	if cfg.Tinkoff.Endpoint != "override.example:443" {
		t.Fatalf("Endpoint = %q, want override.example:443", cfg.Tinkoff.Endpoint)
	}
	if cfg.Tinkoff.Token != "env-token" {
		t.Fatalf("Token = %q, want env-token", cfg.Tinkoff.Token)
	}
	if !cfg.Tinkoff.Sandbox {
		t.Fatal("Sandbox = false, want true")
	}
	if cfg.Tinkoff.AccountID != "env-account" {
		t.Fatalf("AccountID = %q, want env-account", cfg.Tinkoff.AccountID)
	}
	if !cfg.Tinkoff.PayIn.Equal(decimal.RequireFromString("50000")) {
		t.Fatalf("PayIn = %s, want 50000", cfg.Tinkoff.PayIn)
	}
	if cfg.Tinkoff.OrderType != OrderTypeMarket {
		t.Fatalf("OrderType = %q, want market", cfg.Tinkoff.OrderType)
	}
}

func TestTinkoffInvalidPayInEnv(t *testing.T) {
	clearEnv(t)
	t.Setenv("MOEX_TRADER_TINKOFF_PAY_IN", "not-a-decimal")
	_, err := Parse([]byte("storage:\n  path: ./trader.db\n"))
	if err == nil {
		t.Fatal("expected error for invalid tinkoff pay_in env var")
	}
	if !strings.Contains(err.Error(), "MOEX_TRADER_TINKOFF_PAY_IN") {
		t.Fatalf("expected MOEX_TRADER_TINKOFF_PAY_IN error, got: %v", err)
	}
}

func TestTinkoffInvalidSandboxEnv(t *testing.T) {
	clearEnv(t)
	t.Setenv("MOEX_TRADER_TINKOFF_SANDBOX", "not-a-bool")
	_, err := Parse([]byte("storage:\n  path: ./trader.db\n"))
	if err == nil {
		t.Fatal("expected error for invalid tinkoff sandbox env var")
	}
	if !strings.Contains(err.Error(), "MOEX_TRADER_TINKOFF_SANDBOX") {
		t.Fatalf("expected MOEX_TRADER_TINKOFF_SANDBOX error, got: %v", err)
	}
}

func TestTinkoffOrderTypeValidated(t *testing.T) {
	clearEnv(t)
	_, err := Parse([]byte("storage:\n  path: ./trader.db\ntinkoff:\n  order_type: \"stop\"\n"))
	if err == nil {
		t.Fatal("expected error for invalid tinkoff order type")
	}
	if !strings.Contains(err.Error(), "tinkoff.order_type") {
		t.Fatalf("expected tinkoff.order_type error, got: %v", err)
	}
}

func TestTinkoffRealLiveModeRefused(t *testing.T) {
	clearEnv(t)
	_, err := Parse([]byte("storage:\n  path: ./trader.db\nbroker: tinkoff\nis_paper_trading: false\ntinkoff:\n  sandbox: false\n  pay_in: \"100000\"\n"))
	if err == nil {
		t.Fatal("expected error for tinkoff live mode without sandbox")
	}
	if !strings.Contains(err.Error(), "tinkoff.sandbox") {
		t.Fatalf("expected tinkoff.sandbox error, got: %v", err)
	}
}

func TestTinkoffSandboxRequiresPositivePayIn(t *testing.T) {
	clearEnv(t)
	_, err := Parse([]byte("storage:\n  path: ./trader.db\nbroker: tinkoff\nis_paper_trading: false\ntinkoff:\n  sandbox: true\n  pay_in: \"0\"\n"))
	if err == nil {
		t.Fatal("expected error for non-positive sandbox pay_in")
	}
	if !strings.Contains(err.Error(), "tinkoff.pay_in") {
		t.Fatalf("expected tinkoff.pay_in error, got: %v", err)
	}
}

func TestTinkoffSandboxConfigAccepted(t *testing.T) {
	clearEnv(t)
	cfg, err := Parse([]byte("storage:\n  path: ./trader.db\nbroker: tinkoff\nis_paper_trading: false\ntinkoff:\n  sandbox: true\n  pay_in: \"100000\"\n"))
	if err != nil {
		t.Fatalf("Parse returned error: %v", err)
	}
	if !cfg.Tinkoff.Sandbox || cfg.Broker != BrokerTinkoff || cfg.IsPaperTrading {
		t.Fatalf("unexpected config: broker=%q paper=%v sandbox=%v", cfg.Broker, cfg.IsPaperTrading, cfg.Tinkoff.Sandbox)
	}
}

func TestPreflightConfigLoaded(t *testing.T) {
	clearEnv(t)
	cfg, err := Parse([]byte(`
storage:
  path: ./trader.db
preflight:
  enabled: false
  days: 30
  deposit: "50000"
  min_net_pnl: "100"
`))
	if err != nil {
		t.Fatalf("Parse returned error: %v", err)
	}
	if cfg.Preflight.Enabled {
		t.Fatal("Preflight.Enabled = true, want false")
	}
	if cfg.Preflight.Days != 30 {
		t.Fatalf("Preflight.Days = %d, want 30", cfg.Preflight.Days)
	}
	if !cfg.Preflight.Deposit.Equal(decimal.RequireFromString("50000")) {
		t.Fatalf("Preflight.Deposit = %s, want 50000", cfg.Preflight.Deposit)
	}
	if !cfg.Preflight.MinNetPnL.Equal(decimal.RequireFromString("100")) {
		t.Fatalf("Preflight.MinNetPnL = %s, want 100", cfg.Preflight.MinNetPnL)
	}
}

func TestPreflightEnvOverrides(t *testing.T) {
	clearEnv(t)
	t.Setenv("MOEX_TRADER_PREFLIGHT_ENABLED", "false")
	t.Setenv("MOEX_TRADER_PREFLIGHT_DAYS", "45")
	t.Setenv("MOEX_TRADER_PREFLIGHT_DEPOSIT", "200000")
	t.Setenv("MOEX_TRADER_PREFLIGHT_MIN_NET_PNL", "-50")

	cfg, err := Parse([]byte("storage:\n  path: ./trader.db\n"))
	if err != nil {
		t.Fatalf("Parse returned error: %v", err)
	}
	if cfg.Preflight.Enabled {
		t.Fatal("Preflight.Enabled = true, want false")
	}
	if cfg.Preflight.Days != 45 {
		t.Fatalf("Preflight.Days = %d, want 45", cfg.Preflight.Days)
	}
	if !cfg.Preflight.Deposit.Equal(decimal.RequireFromString("200000")) {
		t.Fatalf("Preflight.Deposit = %s, want 200000", cfg.Preflight.Deposit)
	}
	if !cfg.Preflight.MinNetPnL.Equal(decimal.RequireFromString("-50")) {
		t.Fatalf("Preflight.MinNetPnL = %s, want -50", cfg.Preflight.MinNetPnL)
	}
}

func TestPreflightInvalidEnvValues(t *testing.T) {
	clearEnv(t)
	t.Setenv("MOEX_TRADER_PREFLIGHT_ENABLED", "not-a-bool")
	if _, err := Parse([]byte("storage:\n  path: ./trader.db\n")); err == nil || !strings.Contains(err.Error(), "MOEX_TRADER_PREFLIGHT_ENABLED") {
		t.Fatalf("expected MOEX_TRADER_PREFLIGHT_ENABLED error, got: %v", err)
	}

	clearEnv(t)
	t.Setenv("MOEX_TRADER_PREFLIGHT_DAYS", "not-a-number")
	if _, err := Parse([]byte("storage:\n  path: ./trader.db\n")); err == nil || !strings.Contains(err.Error(), "MOEX_TRADER_PREFLIGHT_DAYS") {
		t.Fatalf("expected MOEX_TRADER_PREFLIGHT_DAYS error, got: %v", err)
	}

	clearEnv(t)
	t.Setenv("MOEX_TRADER_PREFLIGHT_DEPOSIT", "not-a-decimal")
	if _, err := Parse([]byte("storage:\n  path: ./trader.db\n")); err == nil || !strings.Contains(err.Error(), "MOEX_TRADER_PREFLIGHT_DEPOSIT") {
		t.Fatalf("expected MOEX_TRADER_PREFLIGHT_DEPOSIT error, got: %v", err)
	}

	clearEnv(t)
	t.Setenv("MOEX_TRADER_PREFLIGHT_MIN_NET_PNL", "not-a-decimal")
	if _, err := Parse([]byte("storage:\n  path: ./trader.db\n")); err == nil || !strings.Contains(err.Error(), "MOEX_TRADER_PREFLIGHT_MIN_NET_PNL") {
		t.Fatalf("expected MOEX_TRADER_PREFLIGHT_MIN_NET_PNL error, got: %v", err)
	}
}

func TestPreflightValidation(t *testing.T) {
	clearEnv(t)
	_, err := Parse([]byte("storage:\n  path: ./trader.db\npreflight:\n  enabled: true\n  days: 0\n"))
	if err == nil || !strings.Contains(err.Error(), "preflight.days") {
		t.Fatalf("expected preflight.days error, got: %v", err)
	}

	_, err = Parse([]byte("storage:\n  path: ./trader.db\npreflight:\n  enabled: true\n  deposit: \"0\"\n"))
	if err == nil || !strings.Contains(err.Error(), "preflight.deposit") {
		t.Fatalf("expected preflight.deposit error, got: %v", err)
	}

	cfg, err := Parse([]byte("storage:\n  path: ./trader.db\npreflight:\n  enabled: false\n  days: 0\n  deposit: \"0\"\n"))
	if err != nil {
		t.Fatalf("Parse returned error for disabled preflight with zero values: %v", err)
	}
	if cfg.Preflight.Enabled {
		t.Fatal("Preflight.Enabled = true, want false")
	}
}
