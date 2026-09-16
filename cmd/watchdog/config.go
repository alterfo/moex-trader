package main

import (
	"fmt"
	"os"
	"strings"
	"time"

	"gopkg.in/yaml.v3"

	"github.com/olegsidorkin/moex-trader/internal/failover"
)

type Duration time.Duration

func (d *Duration) UnmarshalYAML(value *yaml.Node) error {
	var raw string
	if err := value.Decode(&raw); err != nil {
		return err
	}
	parsed, err := time.ParseDuration(strings.TrimSpace(raw))
	if err != nil {
		return fmt.Errorf("parse duration %q: %w", raw, err)
	}
	*d = Duration(parsed)
	return nil
}

func (d Duration) Std() time.Duration {
	return time.Duration(d)
}

type Config struct {
	Role     string `yaml:"role"`
	NodeName string `yaml:"node_name"`
	PeerName string `yaml:"peer_name"`
	Listen   string `yaml:"listen"`
	PeerURL  string `yaml:"peer_url"`
	EnvFile  string `yaml:"env_file"`

	WitnessTokenEnv   string `yaml:"witness_token_env"`
	HeartbeatTokenEnv string `yaml:"heartbeat_token_env"`
	TelegramTokenEnv  string `yaml:"telegram_token_env"`

	Witness  WitnessConfig  `yaml:"witness"`
	Trader   TraderConfig   `yaml:"trader"`
	Tunnel   TunnelConfig   `yaml:"tunnel"`
	DB       DBConfig       `yaml:"db"`
	Timing   TimingConfig   `yaml:"timing"`
	Telegram TelegramConfig `yaml:"telegram"`
}

type WitnessConfig struct {
	URL string   `yaml:"url"`
	TTL Duration `yaml:"ttl"`
}

type TraderConfig struct {
	Command             []string   `yaml:"command"`
	WorkingDir          string     `yaml:"working_dir"`
	StopGrace           Duration   `yaml:"stop_grace"`
	StartupGrace        Duration   `yaml:"startup_grace"`
	Backoff             []Duration `yaml:"backoff"`
	MaxFailures         int        `yaml:"max_failures"`
	RetryCooldown       Duration   `yaml:"retry_cooldown"`
	PostFailureCooldown Duration   `yaml:"post_failure_cooldown"`
	StartupMarker       string     `yaml:"startup_marker"`
}

type TunnelConfig struct {
	Command []string `yaml:"command"`
}

type DBConfig struct {
	Path         string   `yaml:"path"`
	PullInterval Duration `yaml:"pull_interval"`
}

type TimingConfig struct {
	TickInterval         Duration `yaml:"tick_interval"`
	FailoverAfter        Duration `yaml:"failover_after"`
	PeerErrorGrace       Duration `yaml:"peer_error_grace"`
	PeerStaleAfter       Duration `yaml:"peer_stale_after"`
	HandoverCooldown     Duration `yaml:"handover_cooldown"`
	YieldRequestCooldown Duration `yaml:"yield_request_cooldown"`
	YieldRequestTTL      Duration `yaml:"yield_request_ttl"`
	FailureMemory        Duration `yaml:"failure_memory"`
}

type TelegramConfig struct {
	ChatID string `yaml:"chat_id"`
	Proxy  string `yaml:"proxy"`
}

func LoadConfig(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read watchdog config %q: %w", path, err)
	}
	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("parse watchdog config %q: %w", path, err)
	}
	cfg.applyDefaults()
	if err := cfg.validate(); err != nil {
		return nil, err
	}
	return &cfg, nil
}

func (c *Config) applyDefaults() {
	if c.Listen == "" {
		c.Listen = ":9091"
	}
	if c.NodeName == "" {
		c.NodeName = "node"
	}
	if c.EnvFile == "" {
		c.EnvFile = "watchdog.env"
	}
	if c.WitnessTokenEnv == "" {
		c.WitnessTokenEnv = "MOEX_WITNESS_TOKEN"
	}
	if c.HeartbeatTokenEnv == "" {
		c.HeartbeatTokenEnv = "MOEX_FAILOVER_TOKEN"
	}
	if c.TelegramTokenEnv == "" {
		c.TelegramTokenEnv = "MOEX_TRADER_TELEGRAM_BOT_TOKEN"
	}
	if c.Witness.TTL <= 0 {
		c.Witness.TTL = Duration(5 * time.Minute)
	}
	if c.Trader.StopGrace <= 0 {
		c.Trader.StopGrace = Duration(60 * time.Second)
	}
	if c.Trader.StartupGrace <= 0 {
		c.Trader.StartupGrace = Duration(15 * time.Minute)
	}
	if c.Trader.MaxFailures <= 0 {
		c.Trader.MaxFailures = 3
	}
	if c.Trader.RetryCooldown <= 0 {
		c.Trader.RetryCooldown = Duration(5 * time.Minute)
	}
	if c.Trader.PostFailureCooldown <= 0 {
		c.Trader.PostFailureCooldown = Duration(30 * time.Minute)
	}
	if c.Trader.StartupMarker == "" {
		c.Trader.StartupMarker = "starting trader:"
	}
	if len(c.Trader.Backoff) == 0 {
		c.Trader.Backoff = []Duration{Duration(15 * time.Second), Duration(30 * time.Second), Duration(60 * time.Second), Duration(2 * time.Minute)}
	}
	if c.DB.PullInterval <= 0 {
		c.DB.PullInterval = Duration(15 * time.Second)
	}
	if c.Timing.TickInterval <= 0 {
		c.Timing.TickInterval = Duration(5 * time.Second)
	}
	if c.Timing.PeerStaleAfter <= 0 {
		c.Timing.PeerStaleAfter = Duration(30 * time.Second)
	}
	if c.Timing.FailoverAfter <= 0 {
		c.Timing.FailoverAfter = Duration(10 * time.Minute)
	}
	if c.Timing.PeerErrorGrace <= 0 {
		c.Timing.PeerErrorGrace = Duration(2 * time.Minute)
	}
	if c.Timing.HandoverCooldown <= 0 {
		c.Timing.HandoverCooldown = Duration(30 * time.Minute)
	}
	if c.Timing.YieldRequestCooldown <= 0 {
		c.Timing.YieldRequestCooldown = Duration(60 * time.Second)
	}
	if c.Timing.YieldRequestTTL <= 0 {
		c.Timing.YieldRequestTTL = Duration(2 * time.Minute)
	}
	if c.Timing.FailureMemory <= 0 {
		c.Timing.FailureMemory = Duration(30 * time.Minute)
	}
}

func (c *Config) validate() error {
	role := failover.Role(strings.TrimSpace(c.Role))
	if !role.Valid() {
		return fmt.Errorf("watchdog: role must be %q or %q, got %q", failover.RolePrimary, failover.RoleStandby, c.Role)
	}
	c.Role = string(role)
	if strings.TrimSpace(c.Witness.URL) == "" {
		return fmt.Errorf("watchdog: witness.url is required")
	}
	if strings.TrimSpace(c.PeerURL) == "" {
		return fmt.Errorf("watchdog: peer_url is required")
	}
	if strings.TrimSpace(c.DB.Path) == "" {
		return fmt.Errorf("watchdog: db.path is required")
	}
	if len(c.Trader.Command) == 0 {
		return fmt.Errorf("watchdog: trader.command is required")
	}
	return nil
}

func (c *Config) params() failover.Params {
	return failover.Params{
		PeerStaleAfter:       c.Timing.PeerStaleAfter.Std(),
		FailoverAfter:        c.Timing.FailoverAfter.Std(),
		PeerErrorGrace:       c.Timing.PeerErrorGrace.Std(),
		HandoverCooldown:     c.Timing.HandoverCooldown.Std(),
		YieldRequestCooldown: c.Timing.YieldRequestCooldown.Std(),
		YieldRequestTTL:      c.Timing.YieldRequestTTL.Std(),
		StartupGrace:         c.Trader.StartupGrace.Std(),
		FailureMemory:        c.Timing.FailureMemory.Std(),
	}
}

func lookupEnv(values map[string]string, key string) string {
	if value, ok := values[key]; ok && strings.TrimSpace(value) != "" {
		return strings.TrimSpace(value)
	}
	return strings.TrimSpace(os.Getenv(key))
}
