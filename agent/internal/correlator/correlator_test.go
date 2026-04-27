package correlator

import (
	"testing"
	"time"

	"github.com/kitay-sudo/goronin/agent/pkg/protocol"
)

func ev(typ, ip string, t time.Time) protocol.EventRequest {
	return protocol.EventRequest{Type: typ, SourceIP: ip, CreatedAt: t}
}

func TestObserve_GroupsByIP(t *testing.T) {
	c := New(time.Hour)
	now := time.Now()
	c.Observe(ev(protocol.EventSSHTrap, "1.1.1.1", now))
	c.Observe(ev(protocol.EventHTTPTrap, "1.1.1.1", now.Add(time.Second)))
	chain := c.Observe(ev(protocol.EventSSHTrap, "2.2.2.2", now))

	if chain.SourceIP != "2.2.2.2" || len(chain.Events) != 1 {
		t.Errorf("isolation broken: %+v", chain)
	}
}

func TestScore_BasicCount(t *testing.T) {
	now := time.Now()
	events := []protocol.EventRequest{
		ev(protocol.EventSSHTrap, "1.1.1.1", now),
		ev(protocol.EventSSHTrap, "1.1.1.1", now.Add(time.Second)),
	}
	score := calculateScore(events)
	// 2*10 (count) + 1*20 (one unique type) = 40
	if score != 40 {
		t.Errorf("want 40, got %d", score)
	}
}

func TestScore_FileCanaryBonus(t *testing.T) {
	now := time.Now()
	events := []protocol.EventRequest{
		ev(protocol.EventFileCanary, "1.1.1.1", now),
	}
	// 1*10 + 1*20 + 30 (canary) = 60
	if got := calculateScore(events); got != 60 {
		t.Errorf("want 60, got %d", got)
	}
}

func TestScore_KnownPatternBonus(t *testing.T) {
	now := time.Now()
	events := []protocol.EventRequest{
		ev(protocol.EventSSHTrap, "1.1.1.1", now),
		ev(protocol.EventFileCanary, "1.1.1.1", now.Add(time.Minute)),
	}
	// 2*10 + 2*20 + 30 (canary) + 50 (ssh→canary pattern) = 140 → clamped to 100
	if got := calculateScore(events); got != 100 {
		t.Errorf("want 100 (clamped), got %d", got)
	}
}

func TestScore_RapidBurstBonus(t *testing.T) {
	now := time.Now()
	events := []protocol.EventRequest{
		ev(protocol.EventSSHTrap, "1.1.1.1", now),
		ev(protocol.EventHTTPTrap, "1.1.1.1", now.Add(10*time.Second)),
		ev(protocol.EventDBTrap, "1.1.1.1", now.Add(20*time.Second)),
	}
	// 3*10 + 3*20 + 20 (rapid burst) = 110 → clamped to 100
	if got := calculateScore(events); got != 100 {
		t.Errorf("want 100, got %d", got)
	}
}

func TestObserve_DropsEventsOutsideWindow(t *testing.T) {
	c := New(5 * time.Minute)
	now := time.Now()
	c.now = func() time.Time { return now }

	c.Observe(ev(protocol.EventSSHTrap, "1.1.1.1", now.Add(-10*time.Minute)))

	c.now = func() time.Time { return now.Add(time.Second) }
	chain := c.Observe(ev(protocol.EventSSHTrap, "1.1.1.1", now))
	if len(chain.Events) != 1 {
		t.Errorf("expected old event dropped, chain has %d events", len(chain.Events))
	}
}

func TestObserve_SkipsEmptyIP(t *testing.T) {
	c := New(time.Hour)
	if got := c.Observe(ev(protocol.EventSSHTrap, "", time.Now())); got != nil {
		t.Errorf("expected nil for empty IP, got %+v", got)
	}
}

func TestObserve_SetsCreatedAtIfMissing(t *testing.T) {
	c := New(time.Hour)
	chain := c.Observe(protocol.EventRequest{Type: protocol.EventSSHTrap, SourceIP: "1.1.1.1"})
	if chain.Events[0].CreatedAt.IsZero() {
		t.Error("CreatedAt should be auto-filled")
	}
}

func TestContainsSubsequence(t *testing.T) {
	cases := []struct {
		hay, needle []string
		want        bool
	}{
		{[]string{"a", "b", "c"}, []string{"a", "c"}, true},
		{[]string{"a", "b", "c"}, []string{"c", "a"}, false},
		{[]string{"x", "a", "y", "b", "z"}, []string{"a", "b"}, true},
		{[]string{"a"}, []string{"a", "b"}, false},
	}
	for _, c := range cases {
		if got := containsSubsequence(c.hay, c.needle); got != c.want {
			t.Errorf("hay=%v needle=%v: got %v want %v", c.hay, c.needle, got, c.want)
		}
	}
}

func TestObserve_ReturnsCopyNotInternalSlice(t *testing.T) {
	c := New(time.Hour)
	now := time.Now()
	chain := c.Observe(ev(protocol.EventSSHTrap, "1.1.1.1", now))
	chain.Events[0].Type = "TAMPERED"

	chain2 := c.Observe(ev(protocol.EventHTTPTrap, "1.1.1.1", now.Add(time.Second)))
	if chain2.Events[0].Type == "TAMPERED" {
		t.Error("internal state was mutated through returned chain")
	}
}
