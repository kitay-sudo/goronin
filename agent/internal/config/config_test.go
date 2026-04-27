package config

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestLoadAndValidate_HappyPath(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yml")
	content := `
server_name: my-server
telegram:
  bot_token: "1234:abc"
  chat_id: "555"
ai:
  provider: anthropic
  api_key: "sk-ant-xxx"
traps:
  ssh: true
  http: true
  ftp: false
  db: true
auto_ban:
  mode: enforce
  threshold: 3
  window: 5m
  block_duration: 1h
whitelist_ips:
  - 10.0.0.1
watch_files:
  - /root/.env
`
	if err := os.WriteFile(path, []byte(content), 0600); err != nil {
		t.Fatal(err)
	}

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if cfg.ServerName != "my-server" {
		t.Errorf("server_name: %q", cfg.ServerName)
	}
	if cfg.Telegram.BotToken != "1234:abc" || cfg.Telegram.ChatID != "555" {
		t.Errorf("telegram: %+v", cfg.Telegram)
	}
	if cfg.AI.Provider != AIProviderAnthropic {
		t.Errorf("ai provider: %q", cfg.AI.Provider)
	}
	if !cfg.Traps.SSH || cfg.Traps.FTP {
		t.Errorf("traps: %+v", cfg.Traps)
	}
	if cfg.AutoBan.Window != 5*time.Minute {
		t.Errorf("auto_ban.window: %v", cfg.AutoBan.Window)
	}
}

func TestValidate_RejectsMissingTelegram(t *testing.T) {
	cfg := &Config{
		AI:      AIConfig{Provider: AIProviderNone},
		AutoBan: AutoBanConfig{Mode: "enforce"},
	}
	cfg.applyDefaults()
	if err := cfg.Validate(); err == nil {
		t.Fatal("expected error for missing telegram credentials")
	}
}

func TestValidate_RejectsAIProviderWithoutKey(t *testing.T) {
	cfg := &Config{
		Telegram: TelegramConfig{BotToken: "t", ChatID: "c"},
		AI:       AIConfig{Provider: AIProviderOpenAI}, // missing key
	}
	cfg.applyDefaults()
	if err := cfg.Validate(); err == nil {
		t.Fatal("expected error for missing api_key")
	}
}

func TestValidate_AllowsAINoneWithoutKey(t *testing.T) {
	cfg := &Config{
		Telegram: TelegramConfig{BotToken: "t", ChatID: "c"},
		AI:       AIConfig{Provider: AIProviderNone},
	}
	cfg.applyDefaults()
	if err := cfg.Validate(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestValidate_RejectsUnknownAutoBanMode(t *testing.T) {
	cfg := &Config{
		Telegram: TelegramConfig{BotToken: "t", ChatID: "c"},
		AutoBan:  AutoBanConfig{Mode: "yolo"},
	}
	cfg.applyDefaults()
	if err := cfg.Validate(); err == nil {
		t.Fatal("expected error for unknown auto_ban.mode")
	}
}

func TestSaveThenLoad_RoundTrip(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yml")
	original := &Config{
		ServerName: "host",
		Telegram:   TelegramConfig{BotToken: "bot", ChatID: "chat"},
		AI:         AIConfig{Provider: AIProviderGemini, APIKey: "g-key"},
		Traps:      TrapsConfig{SSH: true, HTTP: true, FTP: true, DB: true},
		AutoBan:    AutoBanConfig{Mode: "alert_only", Threshold: 5, Window: 10 * time.Minute, BlockDuration: 2 * time.Hour},
	}
	if err := Save(path, original); err != nil {
		t.Fatalf("save: %v", err)
	}
	loaded, err := Load(path)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if loaded.AI.Provider != AIProviderGemini || loaded.AI.APIKey != "g-key" {
		t.Errorf("ai mismatch: %+v", loaded.AI)
	}
	if loaded.AutoBan.Mode != "alert_only" || loaded.AutoBan.Threshold != 5 {
		t.Errorf("auto_ban mismatch: %+v", loaded.AutoBan)
	}
}

func TestApplyDefaults_FillsBlanks(t *testing.T) {
	cfg := &Config{
		Telegram: TelegramConfig{BotToken: "t", ChatID: "c"},
	}
	cfg.applyDefaults()
	if cfg.AutoBan.Mode != "enforce" {
		t.Errorf("auto_ban.mode default: %q", cfg.AutoBan.Mode)
	}
	if cfg.AutoBan.Threshold != 3 {
		t.Errorf("auto_ban.threshold default: %d", cfg.AutoBan.Threshold)
	}
	if cfg.DataDir != "/var/lib/goronin" {
		t.Errorf("data_dir default: %q", cfg.DataDir)
	}
	if cfg.ServerName == "" {
		t.Error("server_name should fall back to hostname")
	}
}
