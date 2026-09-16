package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/shopspring/decimal"
	"gopkg.in/yaml.v3"
)

type Config struct {
	Tickers         []string   `yaml:"tickers"`
	Model           Model      `yaml:"model"`
	MOEXISSBaseURL  string     `yaml:"moex_iss_base_url"`
	AlgoPackBaseURL string     `yaml:"algopack_base_url"`
	AlgoPackToken   string     `yaml:"-"`
	Storage         Storage    `yaml:"storage"`
	Risk            Risk       `yaml:"risk"`
	Telegram        Telegram   `yaml:"telegram"`
	News            News       `yaml:"news"`
	Commission      Commission `yaml:"commission"`
	Finam           Finam      `yaml:"finam"`
	Tinkoff         Tinkoff    `yaml:"tinkoff"`
	Preflight       Preflight  `yaml:"preflight"`
	Broker          string     `yaml:"broker"`
	IsPaperTrading  bool       `yaml:"is_paper_trading"`
	PollInterval    Duration   `yaml:"poll_interval"`
}

type Model struct {
	Path         string `yaml:"path"`
	EnsemblePath string `yaml:"ensemble_path"`
}

type Storage struct {
	Path string `yaml:"path"`
}

type Risk struct {
	MaxLots        int             `yaml:"max_lots"`
	TargetNotional decimal.Decimal `yaml:"target_notional"`
}

type Telegram struct {
	BotToken      string   `yaml:"-"`
	ChatID        string   `yaml:"chat_id"`
	SignalTickers []string `yaml:"signal_tickers"`
	Proxy         string   `yaml:"proxy"`
}

type News struct {
	Proxy         string          `yaml:"proxy"`
	HistoryPath   string          `yaml:"history_path"`
	RawPath       string          `yaml:"raw_path"`
	TGChannels    []string        `yaml:"telegram_channels"`
	RetroPages    int             `yaml:"retro_pages"`
	VetoEnabled   bool            `yaml:"veto_enabled"`
	VetoSentiment decimal.Decimal `yaml:"veto_sentiment"`
	VetoMinCount  int             `yaml:"veto_min_count"`
}

type Commission struct {
	Broker string          `yaml:"broker"`
	Rate   decimal.Decimal `yaml:"rate"`
}

type Finam struct {
	BaseURL     string `yaml:"base_url"`
	SecretToken string `yaml:"-"`
}

type Tinkoff struct {
	Endpoint  string          `yaml:"endpoint"`
	Sandbox   bool            `yaml:"sandbox"`
	AccountID string          `yaml:"account_id"`
	PayIn     decimal.Decimal `yaml:"pay_in"`
	OrderType string          `yaml:"order_type"`
	Token     string          `yaml:"-"`
}

type Preflight struct {
	Enabled   bool            `yaml:"enabled"`
	Days      int             `yaml:"days"`
	Deposit   decimal.Decimal `yaml:"deposit"`
	MinNetPnL decimal.Decimal `yaml:"min_net_pnl"`
}

const (
	BrokerPaper   = "paper"
	BrokerTinkoff = "tinkoff"
	BrokerFinam   = "finam"
)

const (
	OrderTypeLimit  = "limit"
	OrderTypeMarket = "market"
)

const (
	defaultModelPath        = "model.json"
	defaultMOEXISSBaseURL   = "https://iss.moex.com/iss"
	defaultAlgoPackBaseURL  = "https://apim.moex.com/iss/datashop"
	defaultFinamBaseURL     = "https://api.finam.ru"
	defaultTinkoffEndpoint  = "sandbox-invest-public-api.tbank.ru:443"
	defaultTinkoffOrderType = OrderTypeLimit
	defaultPreflightDays    = 90
	defaultPollInterval     = Duration(5 * time.Minute)
	defaultRiskMaxLots      = 1
	defaultCommissionBroker = "tinkoff"
)

func Default() *Config {
	return &Config{
		Tickers:         append([]string(nil), DefaultTickers()...),
		Model:           Model{Path: defaultModelPath},
		MOEXISSBaseURL:  defaultMOEXISSBaseURL,
		AlgoPackBaseURL: defaultAlgoPackBaseURL,
		Risk:            Risk{MaxLots: defaultRiskMaxLots},
		Telegram:        Telegram{},
		News: News{
			VetoEnabled:   true,
			VetoSentiment: decimal.NewFromFloat(0.5),
			VetoMinCount:  1,
		},
		Commission: Commission{
			Broker: defaultCommissionBroker,
			Rate:   decimal.New(5, -4), // 0.0005, T-Bank "Трейдер" tariff — flat rate, no per-trade minimum
		},
		Finam: Finam{
			BaseURL: defaultFinamBaseURL,
		},
		Tinkoff: Tinkoff{
			Endpoint:  defaultTinkoffEndpoint,
			OrderType: defaultTinkoffOrderType,
		},
		Preflight: Preflight{
			Enabled:   true,
			Days:      defaultPreflightDays,
			Deposit:   decimal.NewFromInt(100_000),
			MinNetPnL: decimal.Zero,
		},
		Broker:         BrokerPaper,
		IsPaperTrading: true,
		PollInterval:   defaultPollInterval,
	}
}

func Load(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read config file %q: %w", path, err)
	}
	if err := loadDotEnv(filepath.Dir(path)); err != nil {
		return nil, err
	}
	return Parse(data)
}

func Parse(data []byte) (*Config, error) {
	cfg := Default()
	if err := yaml.Unmarshal(data, cfg); err != nil {
		return nil, fmt.Errorf("parse config YAML: %w", err)
	}
	cfg.normalize()
	if err := applyEnv(cfg); err != nil {
		return nil, err
	}
	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	return cfg, nil
}

func (c *Config) normalize() {
	if strings.TrimSpace(c.Tinkoff.OrderType) == "" {
		c.Tinkoff.OrderType = defaultTinkoffOrderType
	}
}

func (c *Config) Validate() error {
	if len(c.Tickers) == 0 {
		return fmt.Errorf("tickers must not be empty")
	}
	for i, ticker := range c.Tickers {
		if strings.TrimSpace(ticker) == "" {
			return fmt.Errorf("tickers[%d] must not be empty", i)
		}
	}
	if strings.TrimSpace(c.MOEXISSBaseURL) == "" {
		return fmt.Errorf("moex_iss_base_url must not be empty")
	}
	if strings.TrimSpace(c.AlgoPackBaseURL) == "" {
		return fmt.Errorf("algopack_base_url must not be empty")
	}
	if strings.TrimSpace(c.Finam.BaseURL) == "" {
		return fmt.Errorf("finam.base_url must not be empty")
	}
	if strings.TrimSpace(c.Storage.Path) == "" {
		return fmt.Errorf("storage.path must not be empty")
	}
	if strings.TrimSpace(c.Model.Path) == "" {
		return fmt.Errorf("model.path must not be empty")
	}
	if c.Risk.MaxLots <= 0 {
		return fmt.Errorf("risk.max_lots must be positive")
	}
	if c.Risk.TargetNotional.IsNegative() {
		return fmt.Errorf("risk.target_notional must be non-negative")
	}
	if strings.TrimSpace(c.Commission.Broker) == "" {
		return fmt.Errorf("commission.broker must not be empty")
	}
	if c.Commission.Rate.IsNegative() {
		return fmt.Errorf("commission.rate must be non-negative")
	}
	if c.News.VetoSentiment.IsNegative() || c.News.VetoSentiment.GreaterThan(decimal.NewFromInt(1)) {
		return fmt.Errorf("news.veto_sentiment must be in [0,1]")
	}
	if c.News.VetoMinCount < 0 {
		return fmt.Errorf("news.veto_min_count must be non-negative")
	}
	switch c.Broker {
	case BrokerPaper, BrokerTinkoff, BrokerFinam:
	default:
		return fmt.Errorf("broker must be one of paper, tinkoff, finam")
	}
	if strings.TrimSpace(c.Tinkoff.Endpoint) == "" {
		return fmt.Errorf("tinkoff.endpoint must not be empty")
	}
	switch c.Tinkoff.OrderType {
	case OrderTypeLimit, OrderTypeMarket:
	default:
		return fmt.Errorf("tinkoff.order_type must be one of limit, market")
	}
	if c.Tinkoff.PayIn.IsNegative() {
		return fmt.Errorf("tinkoff.pay_in must be non-negative")
	}
	if c.Broker == BrokerTinkoff && !c.IsPaperTrading {
		if !c.Tinkoff.Sandbox {
			return fmt.Errorf("tinkoff.sandbox must be true: real-money live trading is not wired")
		}
		if !c.Tinkoff.PayIn.IsPositive() {
			return fmt.Errorf("tinkoff.pay_in must be positive in sandbox mode: it funds a new sandbox account and sets the risk drawdown baseline")
		}
	}
	if c.Preflight.Enabled {
		if c.Preflight.Days <= 0 {
			return fmt.Errorf("preflight.days must be positive when preflight is enabled")
		}
		if !c.Preflight.Deposit.IsPositive() {
			return fmt.Errorf("preflight.deposit must be positive when preflight is enabled")
		}
	}
	if c.PollInterval <= 0 {
		return fmt.Errorf("poll_interval must be positive")
	}
	return nil
}

func applyEnv(cfg *Config) error {
	if v := os.Getenv("MOEX_TRADER_MODEL_PATH"); v != "" {
		cfg.Model.Path = v
	}
	if v := os.Getenv("MOEX_TRADER_MOEX_ISS_URL"); v != "" {
		cfg.MOEXISSBaseURL = v
	}
	if v := os.Getenv("MOEX_TRADER_ALGOPACK_BASE_URL"); v != "" {
		cfg.AlgoPackBaseURL = v
	}
	if v := os.Getenv("MOEX_TRADER_ALGOPACK_TOKEN"); v != "" {
		cfg.AlgoPackToken = v
	}
	if v := os.Getenv("MOEX_TRADER_FINAM_BASE_URL"); v != "" {
		cfg.Finam.BaseURL = v
	}
	if v := os.Getenv("MOEX_TRADER_FINAM_SECRET_TOKEN"); v != "" {
		cfg.Finam.SecretToken = v
	}
	if v := os.Getenv("MOEX_TRADER_TINKOFF_ENDPOINT"); v != "" {
		cfg.Tinkoff.Endpoint = v
	}
	if v := os.Getenv("MOEX_TRADER_TINKOFF_TOKEN"); v != "" {
		cfg.Tinkoff.Token = v
	}
	if v := os.Getenv("MOEX_TRADER_TINKOFF_SANDBOX"); v != "" {
		b, err := strconv.ParseBool(v)
		if err != nil {
			return fmt.Errorf("parse MOEX_TRADER_TINKOFF_SANDBOX: %w", err)
		}
		cfg.Tinkoff.Sandbox = b
	}
	if v := os.Getenv("MOEX_TRADER_TINKOFF_ACCOUNT_ID"); v != "" {
		cfg.Tinkoff.AccountID = v
	}
	if v := os.Getenv("MOEX_TRADER_TINKOFF_PAY_IN"); v != "" {
		payIn, err := decimal.NewFromString(v)
		if err != nil {
			return fmt.Errorf("parse MOEX_TRADER_TINKOFF_PAY_IN: %w", err)
		}
		cfg.Tinkoff.PayIn = payIn
	}
	if v := os.Getenv("MOEX_TRADER_TINKOFF_ORDER_TYPE"); v != "" {
		cfg.Tinkoff.OrderType = v
	}
	if v := os.Getenv("MOEX_TRADER_PREFLIGHT_ENABLED"); v != "" {
		b, err := strconv.ParseBool(v)
		if err != nil {
			return fmt.Errorf("parse MOEX_TRADER_PREFLIGHT_ENABLED: %w", err)
		}
		cfg.Preflight.Enabled = b
	}
	if v := os.Getenv("MOEX_TRADER_PREFLIGHT_DAYS"); v != "" {
		days, err := strconv.Atoi(v)
		if err != nil {
			return fmt.Errorf("parse MOEX_TRADER_PREFLIGHT_DAYS: %w", err)
		}
		cfg.Preflight.Days = days
	}
	if v := os.Getenv("MOEX_TRADER_PREFLIGHT_DEPOSIT"); v != "" {
		deposit, err := decimal.NewFromString(v)
		if err != nil {
			return fmt.Errorf("parse MOEX_TRADER_PREFLIGHT_DEPOSIT: %w", err)
		}
		cfg.Preflight.Deposit = deposit
	}
	if v := os.Getenv("MOEX_TRADER_PREFLIGHT_MIN_NET_PNL"); v != "" {
		minNetPnL, err := decimal.NewFromString(v)
		if err != nil {
			return fmt.Errorf("parse MOEX_TRADER_PREFLIGHT_MIN_NET_PNL: %w", err)
		}
		cfg.Preflight.MinNetPnL = minNetPnL
	}
	if v := os.Getenv("MOEX_TRADER_STORAGE_PATH"); v != "" {
		cfg.Storage.Path = v
	}
	if v := os.Getenv("MOEX_TRADER_POLL_INTERVAL"); v != "" {
		dur, err := time.ParseDuration(v)
		if err != nil {
			return fmt.Errorf("parse MOEX_TRADER_POLL_INTERVAL: %w", err)
		}
		cfg.PollInterval = Duration(dur)
	}
	if v := os.Getenv("MOEX_TRADER_IS_PAPER_TRADING"); v != "" {
		b, err := strconv.ParseBool(v)
		if err != nil {
			return fmt.Errorf("parse MOEX_TRADER_IS_PAPER_TRADING: %w", err)
		}
		cfg.IsPaperTrading = b
	}
	if v := os.Getenv("MOEX_TRADER_BROKER"); v != "" {
		cfg.Broker = v
	}
	if v := os.Getenv("MOEX_TRADER_TICKERS"); v != "" {
		cfg.Tickers = splitComma(v)
	}
	if v := os.Getenv("MOEX_TRADER_RISK_MAX_LOTS"); v != "" {
		maxLots, err := strconv.Atoi(v)
		if err != nil {
			return fmt.Errorf("parse MOEX_TRADER_RISK_MAX_LOTS: %w", err)
		}
		cfg.Risk.MaxLots = maxLots
	}
	if v := os.Getenv("MOEX_TRADER_RISK_TARGET_NOTIONAL"); v != "" {
		targetNotional, err := decimal.NewFromString(v)
		if err != nil {
			return fmt.Errorf("parse MOEX_TRADER_RISK_TARGET_NOTIONAL: %w", err)
		}
		cfg.Risk.TargetNotional = targetNotional
	}
	if v := os.Getenv("MOEX_TRADER_TELEGRAM_BOT_TOKEN"); v != "" {
		cfg.Telegram.BotToken = v
	}
	if v := os.Getenv("MOEX_TRADER_TELEGRAM_CHAT_ID"); v != "" {
		cfg.Telegram.ChatID = v
	}
	if v := os.Getenv("MOEX_TRADER_TELEGRAM_SIGNAL_TICKERS"); v != "" {
		cfg.Telegram.SignalTickers = splitComma(v)
	}
	if v := os.Getenv("MOEX_TRADER_TELEGRAM_PROXY"); v != "" {
		cfg.Telegram.Proxy = v
	}
	if v := os.Getenv("MOEX_TRADER_NEWS_PROXY"); v != "" {
		cfg.News.Proxy = v
	}
	if v := os.Getenv("MOEX_TRADER_NEWS_HISTORY_PATH"); v != "" {
		cfg.News.HistoryPath = v
	}
	if v := os.Getenv("MOEX_TRADER_NEWS_RAW_PATH"); v != "" {
		cfg.News.RawPath = v
	}
	if v := os.Getenv("MOEX_TRADER_COMMISSION_RATE"); v != "" {
		rate, err := decimal.NewFromString(v)
		if err != nil {
			return fmt.Errorf("parse MOEX_TRADER_COMMISSION_RATE: %w", err)
		}
		cfg.Commission.Rate = rate
	}
	if v := os.Getenv("MOEX_TRADER_COMMISSION_BROKER"); v != "" {
		cfg.Commission.Broker = v
	}
	return nil
}

func splitComma(s string) []string {
	parts := strings.Split(s, ",")
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part != "" {
			out = append(out, part)
		}
	}
	return out
}
