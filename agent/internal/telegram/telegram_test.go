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
	// Tree-style output: server name + tree rows + AI body parsed from
	// "Severity: ... \n Что произошло: ... \n Команды: ..."
	ai := "Severity: ВЫСОКАЯ\nЧто произошло: тест\nКоманды:\nps aux"
	out := FormatEventAlert("prod-1", ev, ai)
	for _, want := range []string{"prod-1", "1.2.3.4", "22221", "SSH", "blocked", "ВЫСОКАЯ", "Что произошло", "ps aux"} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %q in:\n%s", want, out)
		}
	}
}

func TestFormatEventAlert_FallbackSeverity_FileCanaryWriteIsCritical(t *testing.T) {
	ev := protocol.EventRequest{
		Type:      protocol.EventFileCanary,
		SourceIP:  "localhost",
		CreatedAt: time.Now(),
		Details:   map[string]string{"file": "/root/passwords_backup.txt", "operation": "WRITE"},
	}
	// No AI analysis → must fall back to КРИТИЧЕСКАЯ for canary write.
	out := FormatEventAlert("srv", ev, "")
	if !strings.Contains(out, "КРИТИЧЕСКАЯ") {
		t.Errorf("canary WRITE should fall back to КРИТИЧЕСКАЯ, got:\n%s", out)
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
	// Score-based fallback: 75 → ВЫСОКАЯ. Timeline section must include
	// the file path and a [blocked] marker on the canary row.
	for _, want := range []string{"75/100", "ВЫСОКАЯ", "9.9.9.9", "Timeline", "/root/.env", "[blocked]"} {
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

func TestFormatBatchAlert_StructureAndAI(t *testing.T) {
	summaries := []IPSummary{
		{IP: "1.1.1.1", Score: 70, EventCount: 4, Types: []string{"SSH", "HTTP"}},
		{IP: "2.2.2.2", Score: 30, EventCount: 1, Types: []string{"FTP"}},
	}
	// AI returns "Severity:" first line — header should pick that up.
	ai := "Severity: КРИТИЧЕСКАЯ\nХарактер: целевая атака\nЧто делать: проверить ssh-ключи"
	out := FormatBatchAlert("prod-1", 80, 5, 5, summaries, ai)
	for _, want := range []string{
		"prod-1",
		"Окно: 5 мин",
		"5 с 2 IP",
		"80/100",
		"КРИТИЧЕСКАЯ",
		"1.1.1.1",
		"2.2.2.2",
		"SSH/HTTP",
		"Характер",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %q in:\n%s", want, out)
		}
	}
}

func TestFormatBatchAlert_NoAIWhenEmpty(t *testing.T) {
	summaries := []IPSummary{{IP: "1.1.1.1", Score: 20, EventCount: 1, Types: []string{"SSH"}}}
	out := FormatBatchAlert("srv", 20, 1, 5, summaries, "")
	// No AI body keywords should leak in when analysis is empty.
	if strings.Contains(out, "Характер") || strings.Contains(out, "Что делать") {
		t.Errorf("AI body should be omitted when analysis empty:\n%s", out)
	}
}

func TestFormatBatchAlert_SeverityBuckets(t *testing.T) {
	s := []IPSummary{{IP: "1.1.1.1", Score: 50, EventCount: 1, Types: []string{"SSH"}}}
	cases := []struct {
		score int
		want  string
	}{
		{30, "СРЕДНЯЯ"}, {65, "ВЫСОКАЯ"}, {85, "КРИТИЧЕСКАЯ"},
	}
	for _, c := range cases {
		out := FormatBatchAlert("s", c.score, 1, 5, s, "")
		if !strings.Contains(out, c.want) {
			t.Errorf("score=%d expected %q in output", c.score, c.want)
		}
	}
}

func TestFormatBackgroundDigest_TopIPsLimited(t *testing.T) {
	summaries := []IPSummary{
		{IP: "1.1.1.1", Types: []string{"SSH"}, EventCount: 5},
		{IP: "2.2.2.2", Types: []string{"HTTP"}, EventCount: 3},
		{IP: "3.3.3.3", Types: []string{"FTP"}, EventCount: 2},
		{IP: "4.4.4.4", Types: []string{"DB"}, EventCount: 1},
		{IP: "5.5.5.5", Types: []string{"SSH"}, EventCount: 1},
		{IP: "6.6.6.6", Types: []string{"SSH"}, EventCount: 1},
		{IP: "7.7.7.7", Types: []string{"SSH"}, EventCount: 1},
	}
	out := FormatBackgroundDigest("srv", 14, 60, summaries)
	for _, want := range []string{
		"Фоновый дайджест",
		"srv",
		"60 мин",
		"14 событий",
		"7 IP",
		"1.1.1.1",
		"5.5.5.5",
		"ещё 2 IP",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %q in:\n%s", want, out)
		}
	}
	if strings.Contains(out, "6.6.6.6") || strings.Contains(out, "7.7.7.7") {
		t.Errorf("entries beyond top 5 should not be listed individually")
	}
}

func TestFormatBackgroundDigest_NoIPSection_WhenEmpty(t *testing.T) {
	out := FormatBackgroundDigest("srv", 0, 60, nil)
	if strings.Contains(out, "Топ IP") {
		t.Error("no Top IPs section expected when summaries is empty")
	}
}

func TestExtractSeverity(t *testing.T) {
	cases := []struct {
		in           string
		wantSeverity string
		wantBody     string
	}{
		{"Severity: КРИТИЧЕСКАЯ\nЧто произошло: x", "КРИТИЧЕСКАЯ", "Что произошло: x"},
		{"Severity:ВЫСОКАЯ", "ВЫСОКАЯ", ""},
		{"no severity here", "", "no severity here"},
		{"", "", ""},
	}
	for _, c := range cases {
		gotSev, gotBody := extractSeverity(c.in)
		if gotSev != c.wantSeverity || gotBody != c.wantBody {
			t.Errorf("extractSeverity(%q) = (%q, %q), want (%q, %q)",
				c.in, gotSev, gotBody, c.wantSeverity, c.wantBody)
		}
	}
}

func TestFormatAgentStartup_Compact(t *testing.T) {
	out := FormatAgentStartup("srv", "v0.6.0", []string{"ssh:1", "http:2", "ftp:3", "db:4"}, []string{"/root/a"}, nil)
	// Compact format: counts, not full port list — and no per-port enumeration.
	for _, want := range []string{"GORONIN запущен", "srv", "v0.6.0", "Ловушки: 4", "Канарейки: 1"} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %q in:\n%s", want, out)
		}
	}
	if strings.Contains(out, "ssh:1") || strings.Contains(out, "44219") {
		t.Errorf("startup should not list ports individually:\n%s", out)
	}
}

func TestHTMLEscape_StripsDangerousChars(t *testing.T) {
	out := htmlEscape("<script>&\"")
	if strings.Contains(out, "<") || strings.Contains(out, "\"") {
		t.Errorf("unsafe chars in: %q", out)
	}
}
