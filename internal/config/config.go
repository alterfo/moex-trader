package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/shopspring/decimal"
	"gopkg.in/yaml.v3"
)

type Config struct {
	Tickers         []string   `yaml:"tickers"`
	Ollama          Ollama     `yaml:"ollama"`
	MOEXISSBaseURL  string     `yaml:"moex_iss_base_url"`
	AlgoPackBaseURL string     `yaml:"algopack_base_url"`
	AlgoPackToken   string     `yaml:"-"`
	Storage         Storage    `yaml:"storage"`
	Risk            Risk       `yaml:"risk"`
	Telegram        Telegram   `yaml:"telegram"`
	Commission      Commission `yaml:"commission"`
	IsPaperTrading  bool       `yaml:"is_paper_trading"`
	PollInterval    Duration   `yaml:"poll_interval"`
}

type Ollama struct {
	Host    string   `yaml:"host"`
	Model   string   `yaml:"model"`
	Timeout Duration `yaml:"timeout"`
}

type Storage struct {
	Path string `yaml:"path"`
}

type Risk struct {
	MaxLots int `yaml:"max_lots"`
}

type Telegram struct {
	BotToken string `yaml:"-"`
	ChatID   string `yaml:"chat_id"`
}

type Commission struct {
	Broker string          `yaml:"broker"`
	Rate   decimal.Decimal `yaml:"rate"`
}

const (
	defaultOllamaHost       = "192.168.88.193:11434"
	defaultOllamaModel      = "qwen3.8"
	defaultOllamaTimeout    = Duration(10 * time.Second)
	defaultMOEXISSBaseURL   = "https://iss.moex.com/iss"
	defaultAlgoPackBaseURL  = "https://apim.moex.com/iss/datashop"
	defaultPollInterval     = Duration(5 * time.Minute)
	defaultRiskMaxLots      = 1
	defaultCommissionBroker = "finam"
)

func Default() *Config {
	return &Config{
		Tickers: append([]string(nil), DefaultTickers()...),
		Ollama: Ollama{
			Host:    defaultOllamaHost,
			Model:   defaultOllamaModel,
			Timeout: defaultOllamaTimeout,
		},
		MOEXISSBaseURL:  defaultMOEXISSBaseURL,
		AlgoPackBaseURL: defaultAlgoPackBaseURL,
		Risk:            Risk{MaxLots: defaultRiskMaxLots},
		Commission: Commission{
			Broker: defaultCommissionBroker,
			Rate:   decimal.New(1, -4),
		},
		IsPaperTrading: true,
		PollInterval:   defaultPollInterval,
	}
}

func Load(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read config file %q: %w", path, err)
	}
	return Parse(data)
}

func Parse(data []byte) (*Config, error) {
	cfg := Default()
	if err := yaml.Unmarshal(data, cfg); err != nil {
		return nil, fmt.Errorf("parse config YAML: %w", err)
	}
	if err := applyEnv(cfg); err != nil {
		return nil, err
	}
	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	return cfg, nil
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
	if strings.TrimSpace(c.Ollama.Host) == "" {
		return fmt.Errorf("ollama.host must not be empty")
	}
	if strings.TrimSpace(c.Ollama.Model) == "" {
		return fmt.Errorf("ollama.model must not be empty")
	}
	if c.Ollama.Timeout <= 0 {
		return fmt.Errorf("ollama.timeout must be positive")
	}
	if strings.TrimSpace(c.MOEXISSBaseURL) == "" {
		return fmt.Errorf("moex_iss_base_url must not be empty")
	}
	if strings.TrimSpace(c.AlgoPackBaseURL) == "" {
		return fmt.Errorf("algopack_base_url must not be empty")
	}
	if strings.TrimSpace(c.Storage.Path) == "" {
		return fmt.Errorf("storage.path must not be empty")
	}
	if c.Risk.MaxLots <= 0 {
		return fmt.Errorf("risk.max_lots must be positive")
	}
	if strings.TrimSpace(c.Commission.Broker) == "" {
		return fmt.Errorf("commission.broker must not be empty")
	}
	if c.Commission.Rate.IsNegative() {
		return fmt.Errorf("commission.rate must be non-negative")
	}
	if c.PollInterval <= 0 {
		return fmt.Errorf("poll_interval must be positive")
	}
	return nil
}

func applyEnv(cfg *Config) error {
	if v := os.Getenv("MOEX_TRADER_OLLAMA_HOST"); v != "" {
		cfg.Ollama.Host = v
	}
	if v := os.Getenv("MOEX_TRADER_OLLAMA_MODEL"); v != "" {
		cfg.Ollama.Model = v
	}
	if v := os.Getenv("MOEX_TRADER_OLLAMA_TIMEOUT"); v != "" {
		dur, err := time.ParseDuration(v)
		if err != nil {
			return fmt.Errorf("parse MOEX_TRADER_OLLAMA_TIMEOUT: %w", err)
		}
		cfg.Ollama.Timeout = Duration(dur)
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
	if v := os.Getenv("MOEX_TRADER_TELEGRAM_BOT_TOKEN"); v != "" {
		cfg.Telegram.BotToken = v
	}
	if v := os.Getenv("MOEX_TRADER_TELEGRAM_CHAT_ID"); v != "" {
		cfg.Telegram.ChatID = v
	}
	if v := os.Getenv("MOEX_TRADER_COMMISSION_RATE"); v != "" {
		rate, err := decimal.NewFromString(v)
		if err != nil {
			return fmt.Errorf("parse MOEX_TRADER_COMMISSION_RATE: %w", err)
		}
		cfg.Commission.Rate = rate
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
