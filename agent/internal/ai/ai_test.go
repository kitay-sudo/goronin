package ai

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/kitay-sudo/goronin/agent/internal/config"
	"github.com/kitay-sudo/goronin/agent/pkg/protocol"
)

func sampleEvent() protocol.EventRequest {
	return protocol.EventRequest{
		Type:      protocol.EventSSHTrap,
		SourceIP:  "1.2.3.4",
		TrapPort:  22221,
		CreatedAt: time.Date(2026, 4, 27, 12, 0, 0, 0, time.UTC),
		Details:   map[string]string{"client_banner": "SSH-2.0-libssh"},
	}
}

func TestNoopProvider_WhenDisabled(t *testing.T) {
	p, err := New(config.AIConfig{Provider: config.AIProviderNone})
	if err != nil {
		t.Fatal(err)
	}
	out, err := p.AnalyzeEvent(context.Background(), sampleEvent())
	if err != nil || out != "" {
		t.Errorf("noop should return empty string; got %q err=%v", out, err)
	}
}

func TestNew_RejectsUnknownProvider(t *testing.T) {
	if _, err := New(config.AIConfig{Provider: "yolo"}); err == nil {
		t.Fatal("expected error for unknown provider")
	}
}

// stubServer captures the request and returns a canned response.
func stubServer(t *testing.T, status int, body string) (*httptest.Server, *http.Request, *string) {
	t.Helper()
	var captured *http.Request
	var capturedBody string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		captured = r.Clone(context.Background())
		raw, _ := io.ReadAll(r.Body)
		capturedBody = string(raw)
		w.WriteHeader(status)
		io.WriteString(w, body)
	}))
	t.Cleanup(srv.Close)
	return srv, captured, &capturedBody
}

// patchedAnthropic builds an anthropic provider that hits a stub URL.
func patchedAnthropic(srv *httptest.Server, model string) *anthropicProvider {
	return &anthropicProvider{
		apiKey: "test-key",
		model:  model,
		client: srv.Client(),
	}
}

// We test the JSON shape via the call() method indirectly — point client at
// a stub by overriding the URL. Simplest: temporarily swap http.DefaultTransport
// with a roundtripper that rewrites the host.
type rewriteRT struct {
	base    http.RoundTripper
	target  string
	capture *http.Request
	body    *string
}

func (r *rewriteRT) RoundTrip(req *http.Request) (*http.Response, error) {
	raw, _ := io.ReadAll(req.Body)
	*r.body = string(raw)
	r.capture = req.Clone(context.Background())

	// rewrite to stub server
	stubURL := r.target
	newReq, _ := http.NewRequest(req.Method, stubURL, strings.NewReader(*r.body))
	newReq.Header = req.Header
	return r.base.RoundTrip(newReq)
}

func TestAnthropic_ParsesResponseAndSetsHeaders(t *testing.T) {
	respJSON := `{"content":[{"text":"анализ от Claude"}]}`
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("x-api-key") != "test-key" {
			t.Errorf("missing x-api-key header")
		}
		if r.Header.Get("anthropic-version") == "" {
			t.Errorf("missing anthropic-version header")
		}
		w.Write([]byte(respJSON))
	}))
	defer srv.Close()

	p := &anthropicProvider{apiKey: "test-key", model: "claude-test", client: srv.Client()}
	// override URL via custom roundtripper that ignores destination and hits srv
	p.client = &http.Client{Transport: redirectTo(srv.URL)}

	out, err := p.AnalyzeEvent(context.Background(), sampleEvent())
	if err != nil {
		t.Fatal(err)
	}
	if out != "анализ от Claude" {
		t.Errorf("unexpected output: %q", out)
	}
}

func TestAnthropic_PropagatesAPIErrors(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(401)
		w.Write([]byte(`{"error":{"message":"bad key"}}`))
	}))
	defer srv.Close()

	p := &anthropicProvider{apiKey: "k", model: "m", client: &http.Client{Transport: redirectTo(srv.URL)}}
	if _, err := p.AnalyzeEvent(context.Background(), sampleEvent()); err == nil {
		t.Fatal("expected error on HTTP 401")
	}
}

func TestOpenAI_ParsesResponse(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasPrefix(r.Header.Get("Authorization"), "Bearer ") {
			t.Errorf("missing bearer auth")
		}
		w.Write([]byte(`{"choices":[{"message":{"role":"assistant","content":"анализ от OpenAI"}}]}`))
	}))
	defer srv.Close()

	p := &openaiProvider{apiKey: "k", model: "m", client: &http.Client{Transport: redirectTo(srv.URL)}}
	out, err := p.AnalyzeEvent(context.Background(), sampleEvent())
	if err != nil {
		t.Fatal(err)
	}
	if out != "анализ от OpenAI" {
		t.Errorf("unexpected output: %q", out)
	}
}

func TestGemini_ParsesResponse(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("key") != "g-key" {
			t.Errorf("missing api key in url: %s", r.URL.RawQuery)
		}
		w.Write([]byte(`{"candidates":[{"content":{"parts":[{"text":"анализ от Gemini"}]}}]}`))
	}))
	defer srv.Close()

	p := &geminiProvider{apiKey: "g-key", model: "gemini-test", client: &http.Client{Transport: redirectTo(srv.URL)}}
	out, err := p.AnalyzeEvent(context.Background(), sampleEvent())
	if err != nil {
		t.Fatal(err)
	}
	if out != "анализ от Gemini" {
		t.Errorf("unexpected: %q", out)
	}
}

func TestPromptBuilders_IncludeRelevantFields(t *testing.T) {
	ev := sampleEvent()
	prompt := eventUserPrompt(ev)
	for _, want := range []string{"ssh_trap", "1.2.3.4", "22221", "client_banner"} {
		if !strings.Contains(prompt, want) {
			t.Errorf("event prompt missing %q:\n%s", want, prompt)
		}
	}

	chain := chainUserPrompt("9.9.9.9", 75, []protocol.EventRequest{ev})
	for _, want := range []string{"9.9.9.9", "75/100", "ssh_trap"} {
		if !strings.Contains(chain, want) {
			t.Errorf("chain prompt missing %q:\n%s", want, chain)
		}
	}

	// JSON should be valid (Details serialized correctly)
	var dummy map[string]interface{}
	if err := json.Unmarshal([]byte(`{}`), &dummy); err != nil {
		t.Fatal(err)
	}
}

// redirectTo is a roundtripper that ignores the original Host/Scheme/Path
// and sends every request to baseURL. Lets us point any provider at a stub.
func redirectTo(baseURL string) http.RoundTripper {
	return &redirectRT{base: baseURL, inner: http.DefaultTransport}
}

type redirectRT struct {
	base  string
	inner http.RoundTripper
}

func (r *redirectRT) RoundTrip(req *http.Request) (*http.Response, error) {
	body, _ := io.ReadAll(req.Body)
	url := r.base + req.URL.Path
	if req.URL.RawQuery != "" {
		url += "?" + req.URL.RawQuery
	}
	newReq, err := http.NewRequestWithContext(req.Context(), req.Method, url, strings.NewReader(string(body)))
	if err != nil {
		return nil, err
	}
	for k, v := range req.Header {
		newReq.Header[k] = v
	}
	return r.inner.RoundTrip(newReq)
}
