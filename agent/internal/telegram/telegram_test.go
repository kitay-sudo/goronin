package telegram

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

func TestSend_PostsJSONAndUsesChatID(t *testing.T) {
	var captured map[string]string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		json.Unmarshal(raw, &captured)
		w.Write([]byte(`{"ok":true}`))
	}))
	defer srv.Close()

	c := NewWithBaseURL(config.TelegramConfig{BotToken: "tok", ChatID: "555"}, srv.URL)
	if err := c.Send(context.Background(), "<b>hi</b>"); err != nil {
		t.Fatal(err)
	}
	if captured["chat_id"] != "555" {
		t.Errorf("chat_id: %q", captured["chat_id"])
	}
	if captured["text"] != "<b>hi</b>" {
		t.Errorf("text: %q", captured["text"])
	}
	if captured["parse_mode"] != "HTML" {
		t.Errorf("parse_mode: %q", captured["parse_mode"])
	}
}

func TestSend_ReturnsErrorOnHTTPFailure(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(429)
		io.WriteString(w, "rate limited")
	}))
	defer srv.Close()

	c := NewWithBaseURL(config.TelegramConfig{BotToken: "t", ChatID: "c"}, srv.URL)
	if err := c.Send(context.Background(), "x"); err == nil {
		t.Fatal("expected error on 429")
	}
}

func TestVerify_ReturnsBotUsername(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasSuffix(r.URL.Path, "/getMe") {
			t.Errorf("expected getMe, got %s", r.URL.Path)
		}
		w.Write([]byte(`{"ok":true,"result":{"username":"mybot"}}`))
	}))
	defer srv.Close()

	c := NewWithBaseURL(config.TelegramConfig{BotToken: "t", ChatID: "c"}, srv.URL)
	name, err := c.Verify(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if name != "mybot" {
		t.Errorf("got %q", name)
	}
}

func TestVerify_ErrorOnBadResponse(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"ok":false,"description":"invalid token"}`))
	}))
	defer srv.Close()

	c := NewWithBaseURL(config.TelegramConfig{BotToken: "bad", ChatID: "c"}, srv.URL)
	if _, err := c.Verify(context.Background()); err == nil {
		t.Fatal("expected error for ok:false")
	}
}

func TestFormatEventAlert_IncludesIPAndType(t *testing.T) {
	ev := protocol.EventRequest{
		Type:      protocol.EventSSHTrap,
		SourceIP:  "1.2.3.4",
		TrapPort:  22221,
		CreatedAt: time.Now(),
		Details:   map[string]string{protocol.DetailActionTaken: "blocked"},
	}
	out := FormatEventAlert("prod-1", ev, "анализ")
	for _, want := range []string{"prod-1", "1.2.3.4", "22221", "SSH", "blocked", "анализ"} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %q in:\n%s", want, out)
		}
	}
}

func TestFormatChainAlert_SortsByTimeAndCountsTypes(t *testing.T) {
	now := time.Now()
	events := []protocol.EventRequest{
		{Type: protocol.EventHTTPTrap, SourceIP: "9.9.9.9", CreatedAt: now.Add(2 * time.Second)},
		{Type: protocol.EventSSHTrap, SourceIP: "9.9.9.9", CreatedAt: now},
		{Type: protocol.EventFileCanary, SourceIP: "9.9.9.9", CreatedAt: now.Add(5 * time.Second), Details: map[string]string{"file": "/root/.env", protocol.DetailActionTaken: "blocked"}},
	}
	out := FormatChainAlert("srv", "9.9.9.9", 75, events, "")
	for _, want := range []string{"75/100", "ВЫСОКАЯ", "9.9.9.9", "Timeline", "/root/.env", "🛡"} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %q in:\n%s", want, out)
		}
	}
}

func TestFormatChainAlert_SeverityBuckets(t *testing.T) {
	now := time.Now()
	ev := []protocol.EventRequest{{Type: protocol.EventSSHTrap, SourceIP: "1.1.1.1", CreatedAt: now}}
	cases := []struct {
		score int
		want  string
	}{{30, "СРЕДНЯЯ"}, {65, "ВЫСОКАЯ"}, {85, "КРИТИЧЕСКАЯ"}}
	for _, c := range cases {
		out := FormatChainAlert("s", "1.1.1.1", c.score, ev, "")
		if !strings.Contains(out, c.want) {
			t.Errorf("score=%d expected %q in output", c.score, c.want)
		}
	}
}

func TestHTMLEscape_StripsDangerousChars(t *testing.T) {
	out := htmlEscape("<script>&\"")
	if strings.Contains(out, "<") || strings.Contains(out, "\"") {
		t.Errorf("unsafe chars in: %q", out)
	}
}
