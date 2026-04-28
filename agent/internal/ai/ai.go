// Package ai provides explanatory text for trap events via a configurable
// LLM provider (Anthropic / OpenAI / Google Gemini). All providers share
// a single Provider interface so the rest of the agent doesn't care which
// one the user picked.
//
// AI is optional. If the wizard left provider="", New returns a no-op
// Provider that always returns ("", nil) — events still flow to Telegram,
// just without the analysis paragraph.
package ai

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

// Provider is the common contract. AnalyzeEvent describes a single event;
// AnalyzeChain describes a sequence of related events from one IP;
// AnalyzeBatch describes an aggregated 5-minute window across many IPs
// (preferred entry point in the v0.2+ flow — one LLM call per window).
// All return a short Russian-language paragraph or "" if AI is disabled.
type Provider interface {
	AnalyzeEvent(ctx context.Context, ev protocol.EventRequest) (string, error)
	AnalyzeChain(ctx context.Context, sourceIP string, score int, events []protocol.EventRequest) (string, error)
	AnalyzeBatch(ctx context.Context, totalScore int, ipGroups []BatchGroup) (string, error)
}

// BatchGroup is one source-IP slice of an aggregated batch sent to AnalyzeBatch.
type BatchGroup struct {
	SourceIP string
	Score    int
	Events   []protocol.EventRequest
}

// New constructs a Provider from config. Unknown provider names return an
// error — the wizard validates before saving so this should never fire
// in normal operation.
func New(cfg config.AIConfig) (Provider, error) {
	switch cfg.Provider {
	case config.AIProviderNone:
		return noopProvider{}, nil
	case config.AIProviderAnthropic:
		return &anthropicProvider{apiKey: cfg.APIKey, model: defaulted(cfg.Model, "claude-sonnet-4-6"), client: defaultHTTP()}, nil
	case config.AIProviderOpenAI:
		return &openaiProvider{apiKey: cfg.APIKey, model: defaulted(cfg.Model, "gpt-4o-mini"), client: defaultHTTP()}, nil
	case config.AIProviderGemini:
		return &geminiProvider{apiKey: cfg.APIKey, model: defaulted(cfg.Model, "gemini-2.0-flash"), client: defaultHTTP()}, nil
	default:
		return nil, fmt.Errorf("unknown ai provider: %q", cfg.Provider)
	}
}

func defaulted(v, fallback string) string {
	if v == "" {
		return fallback
	}
	return v
}

func defaultHTTP() *http.Client {
	return &http.Client{Timeout: 30 * time.Second}
}

// ---------- prompts (shared across providers) ----------
//
// Style rules baked into every prompt:
//   - "Severity:" is the FIRST line. Telegram parses it to render a coloured
//     header. The model must output one of: КРИТИЧЕСКАЯ / ВЫСОКАЯ / СРЕДНЯЯ /
//     НИЗКАЯ — exact spelling matters.
//   - "Что делает атакующий" / "Что произошло" — one short sentence each.
//   - "Команды" — concrete shell one-liners the operator can copy-paste.
//     Bash, no placeholders the user has to fill in. If a command needs an
//     IP or path, take it from the event payload.
//   - No filler ("рекомендуется", "следует", "можно сказать").
//
// File canary write/remove from ANY source IP (including localhost) is
// treated as КРИТИЧЕСКАЯ — these files exist solely as bait, no legitimate
// process touches them. The model's natural instinct is to soften localhost
// events to "medium"; this prompt explicitly overrides that.

const eventSystemPrompt = `Ты — эксперт по реагированию на инциденты. На входе — событие с honeypot-ловушки.

Формат ответа (СТРОГО):
Severity: <КРИТИЧЕСКАЯ|ВЫСОКАЯ|СРЕДНЯЯ|НИЗКАЯ>
Что произошло: <одно предложение>
Что делать: <одно предложение>
Команды:
<2-4 готовых bash-команды, каждая с новой строки, без объяснений>

Правила severity:
- file_canary с операцией WRITE или REMOVE — ВСЕГДА КРИТИЧЕСКАЯ. Эти файлы только приманки, легитимного доступа быть не может, даже с localhost (если localhost — значит уже компрометация хоста).
- file_canary с READ — ВЫСОКАЯ.
- ssh_trap / ftp_trap / db_trap — СРЕДНЯЯ (массовый сканер) или ВЫСОКАЯ (если порт нестандартный — целевая разведка).
- http_trap — НИЗКАЯ или СРЕДНЯЯ.

Команды должны быть конкретные, готовые к копированию: проверка процессов (ps, lsof, ss), история (last, w, who, history), iptables-блок IP, проверка ssh-ключей. Без плейсхолдеров.`

const chainSystemPrompt = `Ты — эксперт по реагированию на инциденты. На входе — цепочка событий с honeypot-ловушек от одного IP. IP уже автоматически забанен в iptables.

Формат ответа (СТРОГО):
Severity: <КРИТИЧЕСКАЯ|ВЫСОКАЯ|СРЕДНЯЯ|НИЗКАЯ>
Атакующий: <одно предложение — что делает, на какой стадии: разведка / эксплуатация / закрепление>
Что делать: <одно предложение — что СВЕРХ автобана>
Команды:
<2-4 готовых bash-команды, каждая с новой строки>

Правила severity те же что и для одиночных событий. Если в цепочке есть file_canary write/remove — всегда КРИТИЧЕСКАЯ.`

const batchSystemPrompt = `Ты — эксперт по реагированию на инциденты. На входе — агрегированная активность за 5 минут: несколько IP, у каждого свои события. Все источники уже забанены.

Формат ответа (СТРОГО):
Severity: <КРИТИЧЕСКАЯ|ВЫСОКАЯ|СРЕДНЯЯ|НИЗКАЯ>
Характер: <одно предложение — массовый сканер / целевая атака / ботнет / шум>
Выделяется: <одно предложение или "нет" — есть ли IP/паттерн который требует внимания сверх автобана>
Что делать: <одно предложение>

Без команд (батч-сводка, конкретные действия — в per-event алертах). Не повторяй цифры — они уже в Telegram-сообщении.`

func eventUserPrompt(ev protocol.EventRequest) string {
	details, _ := json.Marshal(ev.Details)
	return fmt.Sprintf("Событие:\n- Тип: %s\n- IP: %s\n- Порт ловушки: %d\n- Детали: %s\n- Время: %s",
		ev.Type, ev.SourceIP, ev.TrapPort, string(details), ev.CreatedAt.Format(time.RFC3339))
}

func chainUserPrompt(sourceIP string, score int, events []protocol.EventRequest) string {
	sorted := make([]protocol.EventRequest, len(events))
	copy(sorted, events)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i].CreatedAt.Before(sorted[j].CreatedAt) })

	var b strings.Builder
	fmt.Fprintf(&b, "IP: %s\nОценка угрозы: %d/100\nКоличество событий: %d\n\nХронология:\n", sourceIP, score, len(sorted))
	for i, ev := range sorted {
		details, _ := json.Marshal(ev.Details)
		fmt.Fprintf(&b, "%d. %s — %s (порт=%d, детали=%s)\n",
			i+1, ev.CreatedAt.Format(time.RFC3339), ev.Type, ev.TrapPort, string(details))
	}
	return b.String()
}

// batchUserPrompt builds the LLM input for an aggregated 5-minute window.
// Each IP gets one section with the score and a per-type event count;
// individual event details are summarised, not enumerated, to keep token
// spend bounded even on large bursts.
func batchUserPrompt(totalScore int, groups []BatchGroup) string {
	var b strings.Builder
	fmt.Fprintf(&b, "Окно: 5 мин · IP: %d · Общая угроза: %d/100\n\n", len(groups), totalScore)
	for _, g := range groups {
		typeCount := map[string]int{}
		for _, ev := range g.Events {
			typeCount[ev.Type]++
		}
		var parts []string
		for t, n := range typeCount {
			parts = append(parts, fmt.Sprintf("%s×%d", t, n))
		}
		sort.Strings(parts)
		fmt.Fprintf(&b, "IP %s — score %d — %s\n", g.SourceIP, g.Score, strings.Join(parts, ", "))
	}
	return b.String()
}

// ---------- noop ----------

type noopProvider struct{}

func (noopProvider) AnalyzeEvent(context.Context, protocol.EventRequest) (string, error) {
	return "", nil
}
func (noopProvider) AnalyzeChain(context.Context, string, int, []protocol.EventRequest) (string, error) {
	return "", nil
}
func (noopProvider) AnalyzeBatch(context.Context, int, []BatchGroup) (string, error) {
	return "", nil
}

// ---------- Anthropic ----------

type anthropicProvider struct {
	apiKey string
	model  string
	client *http.Client
}

type anthropicMessageReq struct {
	Model     string             `json:"model"`
	MaxTokens int                `json:"max_tokens"`
	System    string             `json:"system"`
	Messages  []anthropicMessage `json:"messages"`
}
type anthropicMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}
type anthropicMessageResp struct {
	Content []struct {
		Text string `json:"text"`
	} `json:"content"`
	Error *struct {
		Message string `json:"message"`
	} `json:"error,omitempty"`
}

func (p *anthropicProvider) call(ctx context.Context, system, user string, maxTok int) (string, error) {
	body, _ := json.Marshal(anthropicMessageReq{
		Model:     p.model,
		MaxTokens: maxTok,
		System:    system,
		Messages:  []anthropicMessage{{Role: "user", Content: user}},
	})
	req, _ := http.NewRequestWithContext(ctx, "POST", "https://api.anthropic.com/v1/messages", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("x-api-key", p.apiKey)
	req.Header.Set("anthropic-version", "2023-06-01")

	resp, err := p.client.Do(req)
	if err != nil {
		return "", fmt.Errorf("anthropic: %w", err)
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= 400 {
		return "", fmt.Errorf("anthropic HTTP %d: %s", resp.StatusCode, string(raw))
	}
	var parsed anthropicMessageResp
	if err := json.Unmarshal(raw, &parsed); err != nil {
		return "", err
	}
	if parsed.Error != nil {
		return "", fmt.Errorf("anthropic api: %s", parsed.Error.Message)
	}
	if len(parsed.Content) == 0 {
		return "", nil
	}
	return parsed.Content[0].Text, nil
}

func (p *anthropicProvider) AnalyzeEvent(ctx context.Context, ev protocol.EventRequest) (string, error) {
	return p.call(ctx, eventSystemPrompt, eventUserPrompt(ev), 300)
}
func (p *anthropicProvider) AnalyzeChain(ctx context.Context, ip string, score int, events []protocol.EventRequest) (string, error) {
	return p.call(ctx, chainSystemPrompt, chainUserPrompt(ip, score, events), 500)
}
func (p *anthropicProvider) AnalyzeBatch(ctx context.Context, totalScore int, groups []BatchGroup) (string, error) {
	return p.call(ctx, batchSystemPrompt, batchUserPrompt(totalScore, groups), 400)
}

// ---------- OpenAI ----------

type openaiProvider struct {
	apiKey string
	model  string
	client *http.Client
}

type openaiChatReq struct {
	Model     string          `json:"model"`
	Messages  []openaiMessage `json:"messages"`
	MaxTokens int             `json:"max_tokens"`
}
type openaiMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}
type openaiChatResp struct {
	Choices []struct {
		Message openaiMessage `json:"message"`
	} `json:"choices"`
	Error *struct {
		Message string `json:"message"`
	} `json:"error,omitempty"`
}

func (p *openaiProvider) call(ctx context.Context, system, user string, maxTok int) (string, error) {
	body, _ := json.Marshal(openaiChatReq{
		Model: p.model,
		Messages: []openaiMessage{
			{Role: "system", Content: system},
			{Role: "user", Content: user},
		},
		MaxTokens: maxTok,
	})
	req, _ := http.NewRequestWithContext(ctx, "POST", "https://api.openai.com/v1/chat/completions", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+p.apiKey)

	resp, err := p.client.Do(req)
	if err != nil {
		return "", fmt.Errorf("openai: %w", err)
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= 400 {
		return "", fmt.Errorf("openai HTTP %d: %s", resp.StatusCode, string(raw))
	}
	var parsed openaiChatResp
	if err := json.Unmarshal(raw, &parsed); err != nil {
		return "", err
	}
	if parsed.Error != nil {
		return "", fmt.Errorf("openai api: %s", parsed.Error.Message)
	}
	if len(parsed.Choices) == 0 {
		return "", nil
	}
	return parsed.Choices[0].Message.Content, nil
}

func (p *openaiProvider) AnalyzeEvent(ctx context.Context, ev protocol.EventRequest) (string, error) {
	return p.call(ctx, eventSystemPrompt, eventUserPrompt(ev), 300)
}
func (p *openaiProvider) AnalyzeChain(ctx context.Context, ip string, score int, events []protocol.EventRequest) (string, error) {
	return p.call(ctx, chainSystemPrompt, chainUserPrompt(ip, score, events), 500)
}
func (p *openaiProvider) AnalyzeBatch(ctx context.Context, totalScore int, groups []BatchGroup) (string, error) {
	return p.call(ctx, batchSystemPrompt, batchUserPrompt(totalScore, groups), 400)
}

// ---------- Gemini ----------

type geminiProvider struct {
	apiKey string
	model  string
	client *http.Client
}

type geminiReq struct {
	SystemInstruction *geminiContent  `json:"systemInstruction,omitempty"`
	Contents          []geminiContent `json:"contents"`
	GenerationConfig  geminiGenConfig `json:"generationConfig"`
}
type geminiContent struct {
	Role  string       `json:"role,omitempty"`
	Parts []geminiPart `json:"parts"`
}
type geminiPart struct {
	Text string `json:"text"`
}
type geminiGenConfig struct {
	MaxOutputTokens int `json:"maxOutputTokens"`
}
type geminiResp struct {
	Candidates []struct {
		Content geminiContent `json:"content"`
	} `json:"candidates"`
	Error *struct {
		Message string `json:"message"`
	} `json:"error,omitempty"`
}

func (p *geminiProvider) call(ctx context.Context, system, user string, maxTok int) (string, error) {
	body, _ := json.Marshal(geminiReq{
		SystemInstruction: &geminiContent{Parts: []geminiPart{{Text: system}}},
		Contents:          []geminiContent{{Role: "user", Parts: []geminiPart{{Text: user}}}},
		GenerationConfig:  geminiGenConfig{MaxOutputTokens: maxTok},
	})
	url := fmt.Sprintf("https://generativelanguage.googleapis.com/v1beta/models/%s:generateContent?key=%s", p.model, p.apiKey)
	req, _ := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")

	resp, err := p.client.Do(req)
	if err != nil {
		return "", fmt.Errorf("gemini: %w", err)
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= 400 {
		return "", fmt.Errorf("gemini HTTP %d: %s", resp.StatusCode, string(raw))
	}
	var parsed geminiResp
	if err := json.Unmarshal(raw, &parsed); err != nil {
		return "", err
	}
	if parsed.Error != nil {
		return "", fmt.Errorf("gemini api: %s", parsed.Error.Message)
	}
	if len(parsed.Candidates) == 0 || len(parsed.Candidates[0].Content.Parts) == 0 {
		return "", nil
	}
	return parsed.Candidates[0].Content.Parts[0].Text, nil
}

func (p *geminiProvider) AnalyzeEvent(ctx context.Context, ev protocol.EventRequest) (string, error) {
	return p.call(ctx, eventSystemPrompt, eventUserPrompt(ev), 300)
}
func (p *geminiProvider) AnalyzeChain(ctx context.Context, ip string, score int, events []protocol.EventRequest) (string, error) {
	return p.call(ctx, chainSystemPrompt, chainUserPrompt(ip, score, events), 500)
}
func (p *geminiProvider) AnalyzeBatch(ctx context.Context, totalScore int, groups []BatchGroup) (string, error) {
	return p.call(ctx, batchSystemPrompt, batchUserPrompt(totalScore, groups), 400)
}
