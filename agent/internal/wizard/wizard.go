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
	"errors"
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

// errNoTTY is returned when stdin closes mid-wizard (pipe, non-interactive
// run). Without this the required-field loops would spin forever printing
// "⚠ Поле обязательно." to a stdout no one is reading.
var errNoTTY = errors.New("stdin закрыт или не интерактивный — запусти `sudo goronin install` напрямую в терминале, не через pipe")

// Run prompts on `in`, writes UI to `out`, and returns a populated config
// or the first error encountered. The caller is responsible for actually
// saving the config (config.Save) — separating I/O makes Run testable.
func Run(in io.Reader, out io.Writer) (*config.Config, error) {
	r := bufio.NewReader(in)
	cfg := &config.Config{}

	header(out)

	// --- Server name ---
	hostname, _ := os.Hostname()
	serverName, err := ask(r, out, "Имя сервера", hostname)
	if err != nil {
		return nil, err
	}
	cfg.ServerName = serverName

	// --- Telegram ---
	section(out, "Telegram")
	fmt.Fprintln(out, "  Создай бота через @BotFather, скопируй токен.")
	fmt.Fprintln(out, "  Узнай свой chat_id у @userinfobot.")
	if cfg.Telegram.BotToken, err = askRequired(r, out, "Bot token"); err != nil {
		return nil, err
	}
	if cfg.Telegram.ChatID, err = askRequired(r, out, "Chat ID"); err != nil {
		return nil, err
	}

	if err := verifyTelegram(out, cfg.Telegram); err != nil {
		return nil, fmt.Errorf("проверка Telegram: %w", err)
	}

	// --- AI provider (optional) ---
	section(out, "AI-анализ событий (опционально)")
	fmt.Fprintln(out, "  Без AI алерты будут приходить, просто без подробного разбора.")
	fmt.Fprintln(out, "  Провайдеры: anthropic / openai / gemini / none")
	providerRaw, err := ask(r, out, "Провайдер", "none")
	if err != nil {
		return nil, err
	}
	provider := strings.ToLower(strings.TrimSpace(providerRaw))
	switch provider {
	case "", "none", "n":
		cfg.AI.Provider = config.AIProviderNone
	case "anthropic", "a", "claude":
		cfg.AI.Provider = config.AIProviderAnthropic
		if cfg.AI.APIKey, err = askRequired(r, out, "Anthropic API key"); err != nil {
			return nil, err
		}
		if cfg.AI.Model, err = ask(r, out, "Модель", "claude-sonnet-4-6"); err != nil {
			return nil, err
		}
	case "openai", "o", "gpt":
		cfg.AI.Provider = config.AIProviderOpenAI
		if cfg.AI.APIKey, err = askRequired(r, out, "OpenAI API key"); err != nil {
			return nil, err
		}
		if cfg.AI.Model, err = ask(r, out, "Модель", "gpt-4o-mini"); err != nil {
			return nil, err
		}
	case "gemini", "g", "google":
		cfg.AI.Provider = config.AIProviderGemini
		if cfg.AI.APIKey, err = askRequired(r, out, "Gemini API key"); err != nil {
			return nil, err
		}
		if cfg.AI.Model, err = ask(r, out, "Модель", "gemini-2.0-flash"); err != nil {
			return nil, err
		}
	default:
		return nil, fmt.Errorf("неизвестный провайдер: %s", provider)
	}

	// --- Traps ---
	section(out, "Honeypot ловушки")
	if cfg.Traps.SSH, err = askYes(r, out, "Включить SSH ловушку", true); err != nil {
		return nil, err
	}
	if cfg.Traps.HTTP, err = askYes(r, out, "Включить HTTP ловушку", true); err != nil {
		return nil, err
	}
	if cfg.Traps.FTP, err = askYes(r, out, "Включить FTP ловушку", true); err != nil {
		return nil, err
	}
	if cfg.Traps.DB, err = askYes(r, out, "Включить DB ловушку", true); err != nil {
		return nil, err
	}

	// --- Auto-ban ---
	section(out, "Авто-бан атакующих (iptables)")
	fmt.Fprintln(out, "  off        — не банить, только присылать алерты")
	fmt.Fprintln(out, "  alert_only — логировать что забанилось бы, не трогать iptables")
	fmt.Fprintln(out, "  enforce    — банить (рекомендуется после первой недели)")
	modeRaw, err := ask(r, out, "Режим", "enforce")
	if err != nil {
		return nil, err
	}
	mode := strings.ToLower(strings.TrimSpace(modeRaw))
	switch mode {
	case "off", "alert_only", "enforce":
		cfg.AutoBan.Mode = mode
	default:
		return nil, fmt.Errorf("неизвестный режим: %s", mode)
	}
	if cfg.AutoBan.Threshold, err = askInt(r, out, "Сколько коннектов до бана", 1); err != nil {
		return nil, err
	}
	if cfg.AutoBan.Window, err = askDuration(r, out, "Окно подсчёта", 5*time.Minute); err != nil {
		return nil, err
	}
	fmt.Fprintln(out, "  0 = бан навсегда (рекомендуется для honeypot — на порт-приманку")
	fmt.Fprintln(out, "  легитимного трафика не бывает; снимать вручную через `goronin unban`).")
	if cfg.AutoBan.BlockDuration, err = askDurationOrForever(r, out, "Длительность бана (0 = навсегда)", 0); err != nil {
		return nil, err
	}

	// --- Whitelist ---
	section(out, "Whitelist IP (через запятую — пентестеры, мониторинг, CI)")
	currentIP := detectOwnIP()
	defaultWL := currentIP
	wl, err := ask(r, out, "Whitelist", defaultWL)
	if err != nil {
		return nil, err
	}
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

// readLine reads one line from r. On EOF it returns errNoTTY so the caller
// bails out instead of looping forever on validation failures.
func readLine(r *bufio.Reader) (string, error) {
	line, err := r.ReadString('\n')
	if err != nil {
		// Partial line on EOF still counts as input (the user pressed Ctrl-D
		// after typing something); only treat fully-empty EOF as no-tty.
		if errors.Is(err, io.EOF) && line == "" {
			return "", errNoTTY
		}
		if !errors.Is(err, io.EOF) {
			return "", err
		}
	}
	return line, nil
}

func ask(r *bufio.Reader, out io.Writer, prompt, defaultVal string) (string, error) {
	if defaultVal != "" {
		fmt.Fprintf(out, "  %s [%s]: ", prompt, defaultVal)
	} else {
		fmt.Fprintf(out, "  %s: ", prompt)
	}
	line, err := readLine(r)
	if err != nil {
		return "", err
	}
	line = strings.TrimSpace(line)
	if line == "" {
		return defaultVal, nil
	}
	return line, nil
}

func askRequired(r *bufio.Reader, out io.Writer, prompt string) (string, error) {
	for {
		val, err := ask(r, out, prompt, "")
		if err != nil {
			return "", err
		}
		if val != "" {
			return val, nil
		}
		fmt.Fprintln(out, "  ⚠ Поле обязательно.")
	}
}

func askYes(r *bufio.Reader, out io.Writer, prompt string, def bool) (bool, error) {
	defStr := "Y/n"
	if !def {
		defStr = "y/N"
	}
	for {
		fmt.Fprintf(out, "  %s [%s]: ", prompt, defStr)
		line, err := readLine(r)
		if err != nil {
			return false, err
		}
		line = strings.ToLower(strings.TrimSpace(line))
		switch line {
		case "":
			return def, nil
		case "y", "yes", "д", "да":
			return true, nil
		case "n", "no", "н", "нет":
			return false, nil
		}
		fmt.Fprintln(out, "  ⚠ Введите y или n.")
	}
}

func askInt(r *bufio.Reader, out io.Writer, prompt string, def int) (int, error) {
	for {
		raw, err := ask(r, out, prompt, strconv.Itoa(def))
		if err != nil {
			return 0, err
		}
		n, err := strconv.Atoi(raw)
		if err == nil {
			return n, nil
		}
		fmt.Fprintln(out, "  ⚠ Нужно целое число.")
	}
}

func askDuration(r *bufio.Reader, out io.Writer, prompt string, def time.Duration) (time.Duration, error) {
	for {
		raw, err := ask(r, out, prompt, def.String())
		if err != nil {
			return 0, err
		}
		d, err := time.ParseDuration(raw)
		if err == nil {
			return d, nil
		}
		fmt.Fprintln(out, "  ⚠ Формат: 5m, 1h, 24h, и т.п.")
	}
}

// askDurationOrForever is askDuration plus the special tokens "0",
// "forever", "навсегда" → time.Duration(0), used as the permanent-ban
// sentinel by the firewall layer.
func askDurationOrForever(r *bufio.Reader, out io.Writer, prompt string, def time.Duration) (time.Duration, error) {
	defStr := def.String()
	if def == 0 {
		defStr = "0"
	}
	for {
		raw, err := ask(r, out, prompt, defStr)
		if err != nil {
			return 0, err
		}
		switch strings.ToLower(strings.TrimSpace(raw)) {
		case "0", "forever", "навсегда", "permanent":
			return 0, nil
		}
		d, err := time.ParseDuration(raw)
		if err == nil {
			return d, nil
		}
		fmt.Fprintln(out, "  ⚠ Формат: 0 (навсегда), либо 5m, 1h, 24h, и т.п.")
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
