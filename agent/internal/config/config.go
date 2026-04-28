// Package config holds the on-disk configuration for the standalone goronin
// agent. There is no backend — every setting (AI keys, Telegram credentials,
// trap toggles, auto-ban policy) lives in /etc/goronin/config.yml and is
// produced by the install wizard.
package config

import (
	"fmt"
	"os"
	"time"

	"gopkg.in/yaml.v3"
)

// AIProvider names. Empty string means AI analysis is disabled — events
// still flow to Telegram, just without the explanatory paragraph.
const (
	AIProviderNone      = ""
	AIProviderAnthropic = "anthropic"
	AIProviderOpenAI    = "openai"
	AIProviderGemini    = "gemini"
)

// AIConfig selects which provider to call for event/chain analysis.
// APIKey is required when Provider != "". Model has provider-specific
// defaults (see internal/ai package) and is only set when the user
// wants to override.
type AIConfig struct {
	Provider string `yaml:"provider"`
	APIKey   string `yaml:"api_key"`
	Model    string `yaml:"model,omitempty"`
}

// TelegramConfig is the only output channel for alerts. Both fields are
// required for alerts to fire; if either is empty the alerter logs a
// warning and drops the message.
type TelegramConfig struct {
	BotToken string `yaml:"bot_token"`
	ChatID   string `yaml:"chat_id"`
}

// TrapsConfig toggles individual trap listeners. Each enabled trap binds
// to a random high port at startup.
type TrapsConfig struct {
	SSH  bool `yaml:"ssh"`
	HTTP bool `yaml:"http"`
	FTP  bool `yaml:"ftp"`
	DB   bool `yaml:"db"`
}

// AutoBanConfig controls the firewall response. Mode "off" disables
// blocking entirely (events still reach Telegram). Mode "alert_only"
// logs would-be blocks without touching iptables — useful for the first
// 24h to validate whitelist coverage.
type AutoBanConfig struct {
	Mode          string        `yaml:"mode"` // "off" | "alert_only" | "enforce"
	Threshold     int           `yaml:"threshold"`
	Window        time.Duration `yaml:"window"`
	BlockDuration time.Duration `yaml:"block_duration"`
}

// WatchConfig contains live-monitoring knobs that affect what the daemon
// tells the operator about itself (as opposed to about attackers).
type WatchConfig struct {
	// HeartbeatHours: how often to send a "still alive" message to Telegram.
	// 0 disables; default 12 (twice a day). Pointer so YAML can distinguish
	// "absent" (→ backfilled to 12) from "explicitly 0" (→ disabled). Old
	// installs without the key get the default automatically — no manual
	// YAML edits needed after a binary upgrade.
	HeartbeatHours *int `yaml:"heartbeat_hours"`
}

// AlertingConfig controls how events are batched into Telegram messages
// to keep noise and LLM token spend bounded. See HOW_IT_WORKS.md for the
// full model. Zero values fall back to the defaults below.
type AlertingConfig struct {
	// UrgentWindow: how long to accumulate events before deciding whether
	// to flush as urgent or move to background. Default 5m.
	UrgentWindow time.Duration `yaml:"urgent_window"`

	// BackgroundWindow: how long low-score noise accumulates before being
	// sent as a single hourly digest. Default 1h.
	BackgroundWindow time.Duration `yaml:"background_window"`

	// InterestThreshold: aggregate score below which an urgent batch is
	// demoted to the background bucket instead of being sent immediately.
	// Default 30 (matches the alerter's AI threshold).
	InterestThreshold int `yaml:"interest_threshold"`
}

// Config is the top-level on-disk shape.
type Config struct {
	ServerName   string         `yaml:"server_name"`
	Telegram     TelegramConfig `yaml:"telegram"`
	AI           AIConfig       `yaml:"ai"`
	Traps        TrapsConfig    `yaml:"traps"`
	AutoBan      AutoBanConfig  `yaml:"auto_ban"`
	Alerting     AlertingConfig `yaml:"alerting"`
	Watch        WatchConfig    `yaml:"watch"`
	WhitelistIPs []string       `yaml:"whitelist_ips"`
	WatchFiles   []string       `yaml:"watch_files"`
	DataDir      string         `yaml:"data_dir"` // default /var/lib/goronin
}

// DefaultPath is where the wizard writes the config and the daemon reads
// it on startup.
const DefaultPath = "/etc/goronin/config.yml"

// Defaults applied in-place if the user left a field blank.
func (c *Config) applyDefaults() {
	if c.ServerName == "" {
		hostname, _ := os.Hostname()
		c.ServerName = hostname
	}
	if c.AutoBan.Mode == "" {
		c.AutoBan.Mode = "enforce"
	}
	if c.AutoBan.Threshold == 0 {
		c.AutoBan.Threshold = 3
	}
	if c.AutoBan.Window == 0 {
		c.AutoBan.Window = 5 * time.Minute
	}
	if c.AutoBan.BlockDuration == 0 {
		c.AutoBan.BlockDuration = 1 * time.Hour
	}
	if c.DataDir == "" {
		c.DataDir = "/var/lib/goronin"
	}
	if c.Alerting.UrgentWindow == 0 {
		c.Alerting.UrgentWindow = 5 * time.Minute
	}
	if c.Alerting.BackgroundWindow == 0 {
		c.Alerting.BackgroundWindow = 1 * time.Hour
	}
	if c.Alerting.InterestThreshold == 0 {
		c.Alerting.InterestThreshold = 30
	}
	// HeartbeatHours: default 12 (twice a day). nil = absent in YAML →
	// backfill; explicit 0 = user disabled, leave alone.
	if c.Watch.HeartbeatHours == nil {
		twelve := 12
		c.Watch.HeartbeatHours = &twelve
	}
}

// Validate returns a human-readable error if required fields are missing.
// Called by both the wizard (before writing) and the daemon (on startup).
func (c *Config) Validate() error {
	if c.Telegram.BotToken == "" || c.Telegram.ChatID == "" {
		return fmt.Errorf("telegram bot_token and chat_id are required")
	}
	switch c.AI.Provider {
	case AIProviderNone, AIProviderAnthropic, AIProviderOpenAI, AIProviderGemini:
		// ok
	default:
		return fmt.Errorf("unknown ai.provider: %q (must be anthropic, openai, gemini, or empty)", c.AI.Provider)
	}
	if c.AI.Provider != AIProviderNone && c.AI.APIKey == "" {
		return fmt.Errorf("ai.api_key is required when ai.provider is set")
	}
	switch c.AutoBan.Mode {
	case "off", "alert_only", "enforce":
		// ok
	default:
		return fmt.Errorf("unknown auto_ban.mode: %q (must be off, alert_only, or enforce)", c.AutoBan.Mode)
	}
	return nil
}

// Load reads and parses the YAML config from disk, applies defaults, then
// validates. Returns the same error from Validate() if required fields are
// missing — caller decides whether to bail or run the wizard.
func Load(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	cfg := &Config{}
	if err := yaml.Unmarshal(data, cfg); err != nil {
		return nil, fmt.Errorf("parse %s: %w", path, err)
	}
	cfg.applyDefaults()
	if err := cfg.Validate(); err != nil {
		return cfg, err
	}
	return cfg, nil
}

// Save writes the config back to disk with 0600 permissions (it contains
// API keys and bot tokens). Used by the wizard.
func Save(path string, cfg *Config) error {
	cfg.applyDefaults()
	if err := cfg.Validate(); err != nil {
		return err
	}
	data, err := yaml.Marshal(cfg)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(dirOf(path), 0755); err != nil {
		return err
	}
	return os.WriteFile(path, data, 0600)
}

func dirOf(path string) string {
	for i := len(path) - 1; i >= 0; i-- {
		if path[i] == '/' || path[i] == '\\' {
			return path[:i]
		}
	}
	return "."
}
