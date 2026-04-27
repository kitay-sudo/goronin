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
// AnalyzeChain describes a sequence of related events from one IP.
// Both return a short Russian-language paragraph or "" if AI is disabled.
type Provider interface {
	AnalyzeEvent(ctx context.Context, ev protocol.EventRequest) (string, error)
	AnalyzeChain(ctx context.Context, sourceIP string, score int, events []protocol.EventRequest) (string, error)
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

const eventSystemPrompt = `Ты — эксперт по кибербезопасности. На входе — событие с honeypot-ловушки. Дай краткий разбор на русском (3-5 предложений): что произошло, уровень угрозы (низкий/средний/высокий) и что делать. Будь конкретен, без общих фраз.`

const chainSystemPrompt = `Ты — эксперт по кибербезопасности. На входе — цепочка связанных событий с honeypot'ов от одного IP. Дай оценку атаки на русском (5-7 предложений): что делает атакующий, на какой стадии, что удалось/не удалось, что сделать НЕМЕДЛЕННО (помимо автоблокировки IP). Будь краток.`

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

// ---------- noop ----------

type noopProvider struct{}

func (noopProvider) AnalyzeEvent(context.Context, protocol.EventRequest) (string, error) {
	return "", nil
}
func (noopProvider) AnalyzeChain(context.Context, string, int, []protocol.EventRequest) (string, error) {
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
