// Package wizard runs an interactive terminal questionnaire to produce a
// fully-validated config.Config and write it to disk. It's the entry point
// for `goronin install` and `goronin reconfigure`.
//
// Design choices:
//   - Stdin/stdout only, no TUI library — easier to debug, works over SSH
//     without TERM tricks, and keeps the binary small.
//   - Telegram credentials are verified live (sends a test message) so the
//     user finds out about typos before the daemon starts and stays silent.
//   - AI is opt-in: hitting Enter on the provider prompt picks "none".
package wizard

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"net"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/kitay-sudo/goronin/agent/internal/config"
	"github.com/kitay-sudo/goronin/agent/internal/telegram"
)

// Run prompts on `in`, writes UI to `out`, and returns a populated config
// or the first error encountered. The caller is responsible for actually
// saving the config (config.Save) — separating I/O makes Run testable.
func Run(in io.Reader, out io.Writer) (*config.Config, error) {
	r := bufio.NewReader(in)
	cfg := &config.Config{}

	header(out)

	// --- Server name ---
	hostname, _ := os.Hostname()
	cfg.ServerName = ask(r, out, "Имя сервера", hostname)

	// --- Telegram ---
	section(out, "Telegram")
	fmt.Fprintln(out, "  Создай бота через @BotFather, скопируй токен.")
	fmt.Fprintln(out, "  Узнай свой chat_id у @userinfobot.")
	cfg.Telegram.BotToken = askRequired(r, out, "Bot token")
	cfg.Telegram.ChatID = askRequired(r, out, "Chat ID")

	if err := verifyTelegram(out, cfg.Telegram); err != nil {
		return nil, fmt.Errorf("проверка Telegram: %w", err)
	}

	// --- AI provider (optional) ---
	section(out, "AI-анализ событий (опционально)")
	fmt.Fprintln(out, "  Без AI алерты будут приходить, просто без подробного разбора.")
	fmt.Fprintln(out, "  Провайдеры: anthropic / openai / gemini / none")
	provider := strings.ToLower(strings.TrimSpace(ask(r, out, "Провайдер", "none")))
	switch provider {
	case "", "none", "n":
		cfg.AI.Provider = config.AIProviderNone
	case "anthropic", "a", "claude":
		cfg.AI.Provider = config.AIProviderAnthropic
		cfg.AI.APIKey = askRequired(r, out, "Anthropic API key")
		cfg.AI.Model = ask(r, out, "Модель", "claude-sonnet-4-6")
	case "openai", "o", "gpt":
		cfg.AI.Provider = config.AIProviderOpenAI
		cfg.AI.APIKey = askRequired(r, out, "OpenAI API key")
		cfg.AI.Model = ask(r, out, "Модель", "gpt-4o-mini")
	case "gemini", "g", "google":
		cfg.AI.Provider = config.AIProviderGemini
		cfg.AI.APIKey = askRequired(r, out, "Gemini API key")
		cfg.AI.Model = ask(r, out, "Модель", "gemini-2.0-flash")
	default:
		return nil, fmt.Errorf("неизвестный провайдер: %s", provider)
	}

	// --- Traps ---
	section(out, "Honeypot ловушки")
	cfg.Traps.SSH = askYes(r, out, "Включить SSH ловушку", true)
	cfg.Traps.HTTP = askYes(r, out, "Включить HTTP ловушку", true)
	cfg.Traps.FTP = askYes(r, out, "Включить FTP ловушку", true)
	cfg.Traps.DB = askYes(r, out, "Включить DB ловушку", true)

	// --- Auto-ban ---
	section(out, "Авто-бан атакующих (iptables)")
	fmt.Fprintln(out, "  off        — не банить, только присылать алерты")
	fmt.Fprintln(out, "  alert_only — логировать что забанилось бы, не трогать iptables")
	fmt.Fprintln(out, "  enforce    — банить (рекомендуется после первой недели)")
	mode := strings.ToLower(strings.TrimSpace(ask(r, out, "Режим", "enforce")))
	switch mode {
	case "off", "alert_only", "enforce":
		cfg.AutoBan.Mode = mode
	default:
		return nil, fmt.Errorf("неизвестный режим: %s", mode)
	}
	cfg.AutoBan.Threshold = askInt(r, out, "Сколько коннектов до бана", 3)
	cfg.AutoBan.Window = askDuration(r, out, "Окно подсчёта", 5*time.Minute)
	cfg.AutoBan.BlockDuration = askDuration(r, out, "Длительность бана", 1*time.Hour)

	// --- Whitelist ---
	section(out, "Whitelist IP (через запятую — пентестеры, мониторинг, CI)")
	currentIP := detectOwnIP()
	defaultWL := currentIP
	wl := ask(r, out, "Whitelist", defaultWL)
	for _, raw := range strings.Split(wl, ",") {
		ip := strings.TrimSpace(raw)
		if ip != "" {
			cfg.WhitelistIPs = append(cfg.WhitelistIPs, ip)
		}
	}

	// --- Validate ---
	if err := cfg.Validate(); err != nil {
		return nil, err
	}

	fmt.Fprintln(out)
	fmt.Fprintln(out, "✓ Конфиг готов. Сохраняю в", config.DefaultPath)
	return cfg, nil
}

// ---------- prompt helpers ----------

func header(out io.Writer) {
	fmt.Fprintln(out)
	fmt.Fprintln(out, "  ╔══════════════════════════════════════╗")
	fmt.Fprintln(out, "  ║       GORONIN — установка            ║")
	fmt.Fprintln(out, "  ╚══════════════════════════════════════╝")
	fmt.Fprintln(out)
}

func section(out io.Writer, title string) {
	fmt.Fprintln(out)
	fmt.Fprintln(out, "── "+title+" ─────")
}

func ask(r *bufio.Reader, out io.Writer, prompt, defaultVal string) string {
	if defaultVal != "" {
		fmt.Fprintf(out, "  %s [%s]: ", prompt, defaultVal)
	} else {
		fmt.Fprintf(out, "  %s: ", prompt)
	}
	line, _ := r.ReadString('\n')
	line = strings.TrimSpace(line)
	if line == "" {
		return defaultVal
	}
	return line
}

func askRequired(r *bufio.Reader, out io.Writer, prompt string) string {
	for {
		val := ask(r, out, prompt, "")
		if val != "" {
			return val
		}
		fmt.Fprintln(out, "  ⚠ Поле обязательно.")
	}
}

func askYes(r *bufio.Reader, out io.Writer, prompt string, def bool) bool {
	defStr := "Y/n"
	if !def {
		defStr = "y/N"
	}
	for {
		fmt.Fprintf(out, "  %s [%s]: ", prompt, defStr)
		line, _ := r.ReadString('\n')
		line = strings.ToLower(strings.TrimSpace(line))
		switch line {
		case "":
			return def
		case "y", "yes", "д", "да":
			return true
		case "n", "no", "н", "нет":
			return false
		}
		fmt.Fprintln(out, "  ⚠ Введите y или n.")
	}
}

func askInt(r *bufio.Reader, out io.Writer, prompt string, def int) int {
	for {
		raw := ask(r, out, prompt, strconv.Itoa(def))
		n, err := strconv.Atoi(raw)
		if err == nil {
			return n
		}
		fmt.Fprintln(out, "  ⚠ Нужно целое число.")
	}
}

func askDuration(r *bufio.Reader, out io.Writer, prompt string, def time.Duration) time.Duration {
	for {
		raw := ask(r, out, prompt, def.String())
		d, err := time.ParseDuration(raw)
		if err == nil {
			return d
		}
		fmt.Fprintln(out, "  ⚠ Формат: 5m, 1h, 24h, и т.п.")
	}
}

// verifyTelegram does a getMe + sends a test message. If anything fails,
// the wizard surfaces it before writing the config.
func verifyTelegram(out io.Writer, cfg config.TelegramConfig) error {
	tg := telegram.New(cfg)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	username, err := tg.Verify(ctx)
	if err != nil {
		return err
	}
	fmt.Fprintf(out, "  ✓ Bot подтверждён: @%s\n", username)

	if err := tg.Send(ctx, "<b>GORONIN установка</b>\nТестовое сообщение — если ты его видишь, всё настроено правильно."); err != nil {
		return fmt.Errorf("отправка тест-сообщения: %w", err)
	}
	fmt.Fprintln(out, "  ✓ Тест-сообщение отправлено в Telegram")
	return nil
}

// detectOwnIP picks the source IP of an outbound UDP "connection" — never
// actually sends a packet but the kernel resolves the route, giving us the
// IP the box would use to reach the internet. Falls back to "" if no route.
func detectOwnIP() string {
	conn, err := net.Dial("udp", "8.8.8.8:80")
	if err != nil {
		return ""
	}
	defer conn.Close()
	if addr, ok := conn.LocalAddr().(*net.UDPAddr); ok {
		return addr.IP.String()
	}
	return ""
}
