// Package telegram is the only output channel for the standalone agent.
// All alerts (per-event, attack-chain, agent-up/down) flow through Send.
//
// Construction is pure (no I/O); Send is a single HTTP POST. The wizard
// validates credentials with Verify() before writing the config.
package telegram

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/kitay-sudo/goronin/agent/internal/config"
	"github.com/kitay-sudo/goronin/agent/pkg/protocol"
)

const apiBase = "https://api.telegram.org"

// Client is a minimal Telegram Bot API client. Single bot, single chat —
// no need for user lists or polling, this is one-way notifications only.
type Client struct {
	botToken string
	chatID   string
	http     *http.Client
	baseURL  string // override for tests
}

// New constructs a client. baseURL is set to the production endpoint;
// tests can swap it via NewWithBaseURL.
func New(cfg config.TelegramConfig) *Client {
	return NewWithBaseURL(cfg, apiBase)
}

// NewWithBaseURL is for tests: point the client at httptest.Server.URL.
func NewWithBaseURL(cfg config.TelegramConfig, baseURL string) *Client {
	return &Client{
		botToken: cfg.BotToken,
		chatID:   cfg.ChatID,
		http:     &http.Client{Timeout: 10 * time.Second},
		baseURL:  baseURL,
	}
}

// Send posts an HTML-formatted message to the configured chat.
func (c *Client) Send(ctx context.Context, html string) error {
	url := fmt.Sprintf("%s/bot%s/sendMessage", c.baseURL, c.botToken)
	body, _ := json.Marshal(map[string]string{
		"chat_id":    c.chatID,
		"text":       html,
		"parse_mode": "HTML",
	})
	req, _ := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("telegram send: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		raw, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("telegram HTTP %d: %s", resp.StatusCode, string(raw))
	}
	return nil
}

// Verify hits getMe to confirm the bot token is valid. Used by the wizard.
// Returns the bot's username on success.
func (c *Client) Verify(ctx context.Context) (string, error) {
	url := fmt.Sprintf("%s/bot%s/getMe", c.baseURL, c.botToken)
	req, _ := http.NewRequestWithContext(ctx, "GET", url, nil)
	resp, err := c.http.Do(req)
	if err != nil {
		return "", fmt.Errorf("telegram verify: %w", err)
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= 400 {
		return "", fmt.Errorf("telegram HTTP %d: %s", resp.StatusCode, string(raw))
	}
	var parsed struct {
		OK     bool `json:"ok"`
		Result struct {
			Username string `json:"username"`
		} `json:"result"`
		Description string `json:"description"`
	}
	if err := json.Unmarshal(raw, &parsed); err != nil {
		return "", err
	}
	if !parsed.OK {
		return "", fmt.Errorf("telegram api: %s", parsed.Description)
	}
	return parsed.Result.Username, nil
}

// ---------- formatters ----------

var typeLabels = map[string]string{
	protocol.EventSSHTrap:    "🔒 SSH ловушка",
	protocol.EventHTTPTrap:   "🌐 HTTP ловушка",
	protocol.EventFTPTrap:    "📁 FTP ловушка",
	protocol.EventDBTrap:     "🗄 DB ловушка",
	protocol.EventFileCanary: "📄 Файловая ловушка",
}

var typeLabelsShort = map[string]string{
	protocol.EventSSHTrap:    "SSH trap",
	protocol.EventHTTPTrap:   "HTTP trap",
	protocol.EventFTPTrap:    "FTP trap",
	protocol.EventDBTrap:     "DB trap",
	protocol.EventFileCanary: "File canary",
}

// FormatEventAlert builds the per-event message. aiAnalysis is optional.
func FormatEventAlert(serverName string, ev protocol.EventRequest, aiAnalysis string) string {
	label, ok := typeLabels[ev.Type]
	if !ok {
		label = ev.Type
	}
	t := ev.CreatedAt.In(mustTZ()).Format("02.01.2006 15:04:05 MST")

	var b strings.Builder
	fmt.Fprintf(&b, "🚨 <b>HONEYPOT ALERT — %s</b>\n\n", htmlEscape(serverName))
	fmt.Fprintf(&b, "⚠️ Событие: %s\n", label)
	fmt.Fprintf(&b, "🌍 IP: <code>%s</code>\n", htmlEscape(ev.SourceIP))
	if ev.TrapPort > 0 {
		fmt.Fprintf(&b, "🎯 Порт ловушки: %d\n", ev.TrapPort)
	}
	fmt.Fprintf(&b, "🕐 Время: %s\n", t)
	if action, ok := ev.Details[protocol.DetailActionTaken]; ok {
		fmt.Fprintf(&b, "🛡 Действие: %s\n", htmlEscape(action))
	}
	if aiAnalysis != "" {
		fmt.Fprintf(&b, "\n🤖 <b>AI анализ:</b>\n%s\n", htmlEscape(aiAnalysis))
	}
	return b.String()
}

// FormatChainAlert builds the attack-chain summary.
func FormatChainAlert(serverName, sourceIP string, score int, events []protocol.EventRequest, aiAnalysis string) string {
	sorted := make([]protocol.EventRequest, len(events))
	copy(sorted, events)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i].CreatedAt.Before(sorted[j].CreatedAt) })

	first, last := sorted[0], sorted[len(sorted)-1]
	spanSec := int(last.CreatedAt.Sub(first.CreatedAt).Seconds())
	uniqTypes := map[string]struct{}{}
	for _, ev := range sorted {
		uniqTypes[ev.Type] = struct{}{}
	}

	severity := "СРЕДНЯЯ"
	switch {
	case score >= 80:
		severity = "КРИТИЧЕСКАЯ"
	case score >= 60:
		severity = "ВЫСОКАЯ"
	}

	var b strings.Builder
	fmt.Fprintf(&b, "🚨 <b>ATTACK CHAIN — %s</b>\n", htmlEscape(serverName))
	fmt.Fprintf(&b, "🎯 Оценка: <b>%d/100</b> — угроза: <b>%s</b>\n\n", score, severity)
	fmt.Fprintf(&b, "🌍 IP: <code>%s</code>\n", htmlEscape(sourceIP))
	fmt.Fprintf(&b, "📊 Событий: %d (%d типов) за %dс\n\n", len(sorted), len(uniqTypes), spanSec)
	b.WriteString("<b>Timeline:</b>\n")
	for _, ev := range sorted {
		t := ev.CreatedAt.In(mustTZ()).Format("15:04:05")
		label, ok := typeLabelsShort[ev.Type]
		if !ok {
			label = ev.Type
		}
		extra := ""
		if f, ok := ev.Details["file"]; ok {
			extra = " (" + htmlEscape(f) + ")"
		}
		marker := ""
		if ev.Details[protocol.DetailActionTaken] == "blocked" {
			marker = " 🛡"
		}
		fmt.Fprintf(&b, "  %s  %s%s%s\n", t, label, extra, marker)
	}
	if aiAnalysis != "" {
		fmt.Fprintf(&b, "\n🤖 <b>AI анализ цепочки:</b>\n%s\n", htmlEscape(aiAnalysis))
	}
	return b.String()
}

// IPSummary is one row of a batched alert.
type IPSummary struct {
	IP         string
	Score      int
	EventCount int
	Types      []string // distinct trap types this IP hit, ordered by name
}

// FormatBatchAlert produces the urgent 5-minute sweep message: "за окно
// было N событий с M IP, вот разбивка". aiAnalysis is optional — empty
// means "no AI was called for this batch".
func FormatBatchAlert(serverName string, totalScore, eventCount int, windowMinutes int, summaries []IPSummary, aiAnalysis string) string {
	severity := "СРЕДНЯЯ"
	switch {
	case totalScore >= 80:
		severity = "КРИТИЧЕСКАЯ"
	case totalScore >= 60:
		severity = "ВЫСОКАЯ"
	}

	var b strings.Builder
	fmt.Fprintf(&b, "🚨 <b>GORONIN — %s</b>\n", htmlEscape(serverName))
	fmt.Fprintf(&b, "Окно: %d мин · Событий: %d · IP: %d\n", windowMinutes, eventCount, len(summaries))
	fmt.Fprintf(&b, "Общая угроза: <b>%d/100</b> (%s)\n\n", totalScore, severity)

	for _, s := range summaries {
		marker := scoreMarker(s.Score)
		fmt.Fprintf(&b, "%s <code>%s</code> — score %d — %s×%d\n",
			marker, htmlEscape(s.IP), s.Score, htmlEscape(strings.Join(s.Types, "/")), s.EventCount)
	}

	if aiAnalysis != "" {
		fmt.Fprintf(&b, "\n🤖 <b>AI разбор:</b>\n%s\n", htmlEscape(aiAnalysis))
	}
	return b.String()
}

// FormatBackgroundDigest is the low-noise hourly summary: small numbers,
// no AI, just "был фон, всё забанено, не отвлекайся".
func FormatBackgroundDigest(serverName string, eventCount int, windowMinutes int, summaries []IPSummary) string {
	var b strings.Builder
	fmt.Fprintf(&b, "🔵 <b>Фоновый дайджест — %s</b>\n", htmlEscape(serverName))
	fmt.Fprintf(&b, "За последние %d мин: %d событий с %d IP, все заблокированы.\n", windowMinutes, eventCount, len(summaries))

	if len(summaries) > 0 {
		b.WriteString("\nТоп IP:\n")
		shown := 5
		if len(summaries) < shown {
			shown = len(summaries)
		}
		for i := 0; i < shown; i++ {
			s := summaries[i]
			fmt.Fprintf(&b, "  • <code>%s</code> — %s×%d\n",
				htmlEscape(s.IP), htmlEscape(strings.Join(s.Types, "/")), s.EventCount)
		}
		if len(summaries) > shown {
			fmt.Fprintf(&b, "  • …и ещё %d IP\n", len(summaries)-shown)
		}
	}
	return b.String()
}

// scoreMarker maps a per-IP score to a coloured emoji for the batch list.
func scoreMarker(score int) string {
	switch {
	case score >= 70:
		return "🔴"
	case score >= 40:
		return "🟡"
	default:
		return "⚪"
	}
}

// FormatAgentStartup is sent when goronin starts. Confirms to the operator
// that traps are listening, prints the running version (so it's obvious that
// an update actually rolled), and lists the file canaries under watch plus
// any that failed to be created (those are real problems — disk full, RO
// mount — and the operator needs to see them).
func FormatAgentStartup(serverName, version string, traps, canaries, canariesFailed []string) string {
	t := time.Now().In(mustTZ()).Format("02.01.2006 15:04:05 MST")

	var b strings.Builder
	fmt.Fprintf(&b, "🪴 <b>GORONIN запущен — %s</b>\n", htmlEscape(serverName))
	fmt.Fprintf(&b, "Версия: <code>%s</code>\n\n", htmlEscape(version))
	fmt.Fprintf(&b, "Ловушки: %s\n", htmlEscape(strings.Join(traps, ", ")))

	// Canaries: cap at 10 entries shown, summarise the rest as "+N", so a
	// box with many auto-discovered secrets doesn't blow up the message.
	if n := len(canaries); n > 0 {
		shown := canaries
		extra := 0
		if n > 10 {
			shown = canaries[:10]
			extra = n - 10
		}
		fmt.Fprintf(&b, "Канарейки (%d): %s", n, htmlEscape(strings.Join(shown, ", ")))
		if extra > 0 {
			fmt.Fprintf(&b, " +%d", extra)
		}
		b.WriteString("\n")
	} else {
		b.WriteString("Канарейки: нет\n")
	}

	// Show failures separately so they don't look like normal canaries —
	// these are paths the OS refused (no write permission, RO mount, full
	// disk). Operator should investigate.
	if n := len(canariesFailed); n > 0 {
		shown := canariesFailed
		extra := 0
		if n > 10 {
			shown = canariesFailed[:10]
			extra = n - 10
		}
		fmt.Fprintf(&b, "⚠ Не удалось создать (%d): %s", n, htmlEscape(strings.Join(shown, ", ")))
		if extra > 0 {
			fmt.Fprintf(&b, " +%d", extra)
		}
		b.WriteString("\n")
	}

	fmt.Fprintf(&b, "Время: %s", t)
	return b.String()
}

// htmlEscape escapes the four chars Telegram's HTML parser cares about.
func htmlEscape(s string) string {
	r := strings.NewReplacer("&", "&amp;", "<", "&lt;", ">", "&gt;", `"`, "&quot;")
	return r.Replace(s)
}

// mustTZ returns Europe/Moscow if available, UTC otherwise. Telegram messages
// show local times to the operator — Moscow is the dominant timezone for the
// target user base; ops in other zones can still parse the offset from MST.
func mustTZ() *time.Location {
	if loc, err := time.LoadLocation("Europe/Moscow"); err == nil {
		return loc
	}
	return time.UTC
}
