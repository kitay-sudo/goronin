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
//
// All alert messages share a tree-style layout (├ entries + └ on the last
// line of a section) and skip emoji decorations — they were noisy and
// inconsistent across clients. The header carries the only colour signal:
// a severity word parsed out of the AI response (КРИТИЧЕСКАЯ / ВЫСОКАЯ /
// СРЕДНЯЯ / НИЗКАЯ). When AI is disabled, we fall back to a heuristic
// (file_canary write/remove → КРИТИЧЕСКАЯ).

var typeLabels = map[string]string{
	protocol.EventSSHTrap:    "SSH ловушка",
	protocol.EventHTTPTrap:   "HTTP ловушка",
	protocol.EventFTPTrap:    "FTP ловушка",
	protocol.EventDBTrap:     "DB ловушка",
	protocol.EventFileCanary: "Файловая ловушка",
}

var typeLabelsShort = map[string]string{
	protocol.EventSSHTrap:    "SSH",
	protocol.EventHTTPTrap:   "HTTP",
	protocol.EventFTPTrap:    "FTP",
	protocol.EventDBTrap:     "DB",
	protocol.EventFileCanary: "FILE",
}

// severityHeader returns the title-bar string for a given severity word.
// Coloured square is the one bit of visual signal we keep — Telegram's
// inline emoji rendering is consistent for these four.
func severityHeader(severity string) string {
	switch strings.ToUpper(severity) {
	case "КРИТИЧЕСКАЯ":
		return "🥊 КРИТИЧЕСКАЯ"
	case "ВЫСОКАЯ":
		return "🍑 ВЫСОКАЯ"
	case "СРЕДНЯЯ":
		return "🍔 СРЕДНЯЯ"
	case "НИЗКАЯ":
		return "🥝 НИЗКАЯ"
	default:
		return "🍔 СРЕДНЯЯ"
	}
}

// fallbackSeverity picks a sane severity when AI is disabled / failed.
// Logic mirrors the eventSystemPrompt rules: file_canary write/remove is
// always critical; read on canary is high; everything else medium.
func fallbackSeverity(ev protocol.EventRequest) string {
	if ev.Type == protocol.EventFileCanary {
		op := strings.ToUpper(ev.Details["operation"])
		if strings.Contains(op, "WRITE") || strings.Contains(op, "REMOVE") {
			return "КРИТИЧЕСКАЯ"
		}
		return "ВЫСОКАЯ"
	}
	return "СРЕДНЯЯ"
}

// extractSeverity reads "Severity: X" from the first line of an AI
// response and returns X (or "" if the line is missing). The rest of the
// AI text is returned with that line stripped, so the body shown to the
// user doesn't repeat the severity already in the header.
func extractSeverity(ai string) (severity, body string) {
	ai = strings.TrimSpace(ai)
	if ai == "" {
		return "", ""
	}
	lines := strings.SplitN(ai, "\n", 2)
	first := strings.TrimSpace(lines[0])
	const prefix = "Severity:"
	if strings.HasPrefix(first, prefix) {
		severity = strings.TrimSpace(strings.TrimPrefix(first, prefix))
		if len(lines) > 1 {
			body = strings.TrimSpace(lines[1])
		}
		return severity, body
	}
	return "", ai
}

// FormatEventAlert builds the per-event message in tree style.
//
// Layout:
//   <severity-bar> server
//   ├ Тип:  <label>
//   ├ IP:   <ip>
//   ├ Порт: <n>            (omitted if 0)
//   ├ Время: <ts>
//   └ Действие: <...>      (omitted if absent)
//
//   <AI body, parsed from "Что произошло / Что делать / Команды:">
func FormatEventAlert(serverName string, ev protocol.EventRequest, aiAnalysis string) string {
	label, ok := typeLabels[ev.Type]
	if !ok {
		label = ev.Type
	}
	t := ev.CreatedAt.In(mustTZ()).Format("02.01.2006 15:04:05 MST")
	action, hasAction := ev.Details[protocol.DetailActionTaken]

	severity, body := extractSeverity(aiAnalysis)
	if severity == "" {
		severity = fallbackSeverity(ev)
	}

	// Build the tree section. Lines list out as ├, then mark the final
	// line as └ once we know which fields are present.
	type row struct{ k, v string }
	rows := []row{
		{"Тип", label},
		{"IP", "<code>" + htmlEscape(ev.SourceIP) + "</code>"},
	}
	if ev.TrapPort > 0 {
		rows = append(rows, row{"Порт", fmt.Sprintf("%d", ev.TrapPort)})
	}
	if file, ok := ev.Details["file"]; ok {
		rows = append(rows, row{"Файл", "<code>" + htmlEscape(file) + "</code>"})
	}
	if op, ok := ev.Details["operation"]; ok {
		rows = append(rows, row{"Операция", htmlEscape(op)})
	}
	rows = append(rows, row{"Время", t})
	if hasAction {
		rows = append(rows, row{"Действие", htmlEscape(action)})
	}

	var b strings.Builder
	fmt.Fprintf(&b, "<b>%s — %s</b>\n", severityHeader(severity), htmlEscape(serverName))
	for i, r := range rows {
		prefix := "├"
		if i == len(rows)-1 {
			prefix = "└"
		}
		fmt.Fprintf(&b, "%s %s: %s\n", prefix, r.k, r.v)
	}
	if body != "" {
		b.WriteString("\n")
		b.WriteString(formatAIBody(body))
	}
	return b.String()
}

// formatAIBody takes the AI response (with "Severity:" already stripped)
// and renders it for Telegram in tree style: first labelled line is the
// root, subsequent labelled lines get ├ (and the last one └). "Команды:"
// becomes a <pre> block. Anything else is escaped and shown as-is.
func formatAIBody(body string) string {
	type labelledLine struct{ label, rest string }

	lines := strings.Split(body, "\n")
	var out strings.Builder
	var inCommands bool
	var cmds []string

	// First pass: collect labelled lines (non-Команды) so we know which is
	// last for the └ vs ├ choice. Everything else passes through to a
	// "tail" buffer that's emitted after the labelled tree.
	var labelled []labelledLine
	var tail []string

	for _, raw := range lines {
		line := strings.TrimSpace(raw)
		if line == "" {
			if inCommands {
				inCommands = false
			}
			continue
		}
		if strings.HasPrefix(line, "Команды:") {
			inCommands = true
			continue
		}
		if inCommands {
			cmds = append(cmds, line)
			continue
		}
		if idx := strings.Index(line, ":"); idx > 0 && idx < 40 {
			labelled = append(labelled, labelledLine{label: line[:idx], rest: strings.TrimSpace(line[idx+1:])})
		} else {
			tail = append(tail, line)
		}
	}

	for i, l := range labelled {
		var prefix string
		switch {
		case i == 0:
			prefix = ""
		case i == len(labelled)-1:
			prefix = "└ "
		default:
			prefix = "├ "
		}
		fmt.Fprintf(&out, "%s<b>%s:</b> %s\n", prefix, htmlEscape(l.label), htmlEscape(l.rest))
	}

	for _, line := range tail {
		out.WriteString(htmlEscape(line))
		out.WriteString("\n")
	}

	if len(cmds) > 0 {
		out.WriteString("<pre>")
		out.WriteString(htmlEscape(strings.Join(cmds, "\n")))
		out.WriteString("</pre>\n")
	}
	return out.String()
}

// FormatChainAlert builds the attack-chain summary in tree style.
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

	severity, body := extractSeverity(aiAnalysis)
	if severity == "" {
		// Score-based fallback when AI is off / parse failed.
		switch {
		case score >= 80:
			severity = "КРИТИЧЕСКАЯ"
		case score >= 60:
			severity = "ВЫСОКАЯ"
		default:
			severity = "СРЕДНЯЯ"
		}
	}

	var b strings.Builder
	fmt.Fprintf(&b, "<b>%s — %s</b>\n", severityHeader(severity), htmlEscape(serverName))
	fmt.Fprintf(&b, "├ Цепочка от: <code>%s</code>\n", htmlEscape(sourceIP))
	fmt.Fprintf(&b, "├ Score: %d/100\n", score)
	fmt.Fprintf(&b, "└ События: %d (%d типов) за %dс\n", len(sorted), len(uniqTypes), spanSec)

	b.WriteString("\n<b>Timeline:</b>\n<pre>")
	for _, ev := range sorted {
		t := ev.CreatedAt.In(mustTZ()).Format("15:04:05")
		label, ok := typeLabelsShort[ev.Type]
		if !ok {
			label = ev.Type
		}
		extra := ""
		if f, ok := ev.Details["file"]; ok {
			extra = " " + f
		}
		marker := ""
		if ev.Details[protocol.DetailActionTaken] == "blocked" {
			marker = " [blocked]"
		}
		fmt.Fprintf(&b, "%s  %-4s%s%s\n", t, label, htmlEscape(extra), marker)
	}
	b.WriteString("</pre>")

	if body != "" {
		b.WriteString("\n")
		b.WriteString(formatAIBody(body))
	}
	return b.String()
}

// IPSummary is one row of a batched alert.
type IPSummary struct {
	IP         string
	Score      int
	EventCount int
	Types      []string // distinct trap types this IP hit, ordered by name

	// Blocked is true if firewall has an active block for this IP.
	// BlockDuration is the human label ("1ч", "24ч") used in the alert footer.
	Blocked       bool
	BlockDuration string
}

// trapLabel maps short type code to a Russian label for IP detail rows.
var trapLabel = map[string]string{
	"SSH":  "SSH-ловушка",
	"HTTP": "HTTP-ловушка",
	"FTP":  "FTP-ловушка",
	"DB":   "DB-ловушка",
	"file": "Файловая ловушка",
}

// FormatBatchAlert produces the urgent 5-minute sweep message in tree style.
// aiAnalysis is optional — empty means no AI was called for this batch.
func FormatBatchAlert(serverName string, totalScore, eventCount int, windowMinutes int, summaries []IPSummary, aiAnalysis string) string {
	severity, body := extractSeverity(aiAnalysis)
	if severity == "" {
		switch {
		case totalScore >= 80:
			severity = "КРИТИЧЕСКАЯ"
		case totalScore >= 60:
			severity = "ВЫСОКАЯ"
		default:
			severity = "СРЕДНЯЯ"
		}
	}

	blockedCount := 0
	for _, s := range summaries {
		if s.Blocked {
			blockedCount++
		}
	}

	var b strings.Builder
	fmt.Fprintf(&b, "<b>%s — %s</b>\n", severityHeader(severity), htmlEscape(serverName))
	fmt.Fprintf(&b, "├ Окно: %d мин\n", windowMinutes)
	fmt.Fprintf(&b, "├ События: %d с %d IP\n", eventCount, len(summaries))
	if blockedCount > 0 {
		fmt.Fprintf(&b, "├ Общий score: %d/100\n", totalScore)
		fmt.Fprintf(&b, "└ Заблокировано: %d IP\n", blockedCount)
	} else {
		fmt.Fprintf(&b, "└ Общий score: %d/100\n", totalScore)
	}

	if len(summaries) > 0 {
		b.WriteString("\n<b>IP:</b>\n")
		for i, s := range summaries {
			ipLine := fmt.Sprintf("<code>%s</code>    score %d/100", htmlEscape(s.IP), s.Score)
			if s.Blocked {
				suffix := "🛡 заблокирован"
				if s.BlockDuration != "" {
					suffix = fmt.Sprintf("🛡 заблокирован (%s)", s.BlockDuration)
				}
				ipLine += "    " + suffix
			}
			b.WriteString(ipLine)
			b.WriteString("\n")

			for j, tp := range s.Types {
				prefix := "├"
				if j == len(s.Types)-1 {
					prefix = "└"
				}
				label, ok := trapLabel[tp]
				if !ok {
					label = tp
				}
				fmt.Fprintf(&b, "  %s %s\n", prefix, htmlEscape(label))
			}
			if i < len(summaries)-1 {
				b.WriteString("\n")
			}
		}
	}

	if body != "" {
		b.WriteString("\n")
		b.WriteString(formatAIBody(body))
	}
	return b.String()
}

// FormatBackgroundDigest is the low-noise hourly summary in tree style.
// No AI, no severity bar — this is the "ничего срочного" channel.
func FormatBackgroundDigest(serverName string, eventCount int, windowMinutes int, summaries []IPSummary) string {
	var b strings.Builder
	fmt.Fprintf(&b, "<b>🟦 Фоновый дайджест — %s</b>\n", htmlEscape(serverName))
	fmt.Fprintf(&b, "├ Окно: %d мин\n", windowMinutes)
	fmt.Fprintf(&b, "└ %d событий с %d IP — все заблокированы\n", eventCount, len(summaries))

	if len(summaries) > 0 {
		b.WriteString("\n<b>Топ IP:</b>\n<pre>")
		shown := 5
		if len(summaries) < shown {
			shown = len(summaries)
		}
		for i := 0; i < shown; i++ {
			s := summaries[i]
			fmt.Fprintf(&b, "%-15s  %s×%d\n",
				htmlEscape(s.IP), htmlEscape(strings.Join(s.Types, "/")), s.EventCount)
		}
		if len(summaries) > shown {
			fmt.Fprintf(&b, "...и ещё %d IP\n", len(summaries)-shown)
		}
		b.WriteString("</pre>")
	}
	return b.String()
}

// FormatAgentStartup is sent when goronin starts. Compact: confirms the
// agent is up, the version (so an update is visibly applied), how many
// traps and canaries are armed. We deliberately don't list ports — the
// operator can `goronin status` if they need them, and showing four
// random ports in every startup message was noise.
func FormatAgentStartup(serverName, version string, traps, canaries, canariesFailed []string) string {
	t := time.Now().In(mustTZ()).Format("02.01.2006 15:04:05 MST")

	var b strings.Builder
	fmt.Fprintf(&b, "<b>🪴 GORONIN запущен — %s</b>\n", htmlEscape(serverName))
	fmt.Fprintf(&b, "├ Версия: <code>%s</code>\n", htmlEscape(version))
	fmt.Fprintf(&b, "├ Ловушки: %d активны\n", len(traps))

	if len(canariesFailed) > 0 {
		fmt.Fprintf(&b, "├ Канарейки: %d активны, %d не удалось создать\n", len(canaries), len(canariesFailed))
	} else if len(canaries) > 0 {
		fmt.Fprintf(&b, "├ Канарейки: %d активны\n", len(canaries))
	} else {
		b.WriteString("├ Канарейки: нет\n")
	}

	fmt.Fprintf(&b, "└ Время: %s", t)
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
