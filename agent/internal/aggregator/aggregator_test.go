package aggregator

import (
	"sync"
	"testing"
	"time"

	"github.com/kitay-sudo/goronin/agent/pkg/protocol"
)

// captureFlush returns a flush func that appends batches to a slice and
// signals on a channel after each call. Tests wait on the channel rather
// than sleeping, so they don't race against the timer goroutine.
func captureFlush() (*[]Batch, *sync.Mutex, chan struct{}, FlushFunc) {
	var (
		mu       sync.Mutex
		batches  []Batch
		signal   = make(chan struct{}, 16)
	)
	flush := func(b Batch) {
		mu.Lock()
		batches = append(batches, b)
		mu.Unlock()
		signal <- struct{}{}
	}
	return &batches, &mu, signal, flush
}

func waitForFlush(t *testing.T, ch chan struct{}) {
	t.Helper()
	select {
	case <-ch:
	case <-time.After(2 * time.Second):
		t.Fatal("expected flush within 2s, got nothing")
	}
}

func ev(typ, ip string, at time.Time) protocol.EventRequest {
	return protocol.EventRequest{Type: typ, SourceIP: ip, CreatedAt: at}
}

func TestObserve_DropsEmptyAndLocalhost(t *testing.T) {
	batches, _, _, flush := captureFlush()
	a := New(Config{UrgentWindow: 10 * time.Millisecond}, flush)
	defer a.Stop()

	a.Observe(ev(protocol.EventSSHTrap, "", time.Now()))
	a.Observe(ev(protocol.EventSSHTrap, "localhost", time.Now()))

	time.Sleep(50 * time.Millisecond)
	if len(*batches) != 0 {
		t.Errorf("expected no flush, got %d", len(*batches))
	}
}

func TestUrgentSweep_HighScoreBatch_Flushes(t *testing.T) {
	batches, mu, signal, flush := captureFlush()
	a := New(Config{
		UrgentWindow:      30 * time.Millisecond,
		InterestThreshold: 30,
	}, flush)
	defer a.Stop()

	now := time.Now()
	// 3 SSH events from one IP → score = 30+20 = 50, above threshold.
	a.Observe(ev(protocol.EventSSHTrap, "1.1.1.1", now))
	a.Observe(ev(protocol.EventSSHTrap, "1.1.1.1", now.Add(time.Second)))
	a.Observe(ev(protocol.EventSSHTrap, "1.1.1.1", now.Add(2*time.Second)))

	waitForFlush(t, signal)

	mu.Lock()
	defer mu.Unlock()
	if len(*batches) != 1 {
		t.Fatalf("want 1 batch, got %d", len(*batches))
	}
	b := (*batches)[0]
	if b.IsBackground {
		t.Error("urgent sweep flagged as background")
	}
	if b.EventCount != 3 {
		t.Errorf("EventCount=%d, want 3", b.EventCount)
	}
	if len(b.Groups) != 1 || b.Groups[0].SourceIP != "1.1.1.1" {
		t.Errorf("groups: %+v", b.Groups)
	}
	if b.TotalScore < 30 {
		t.Errorf("TotalScore=%d, want >= 30", b.TotalScore)
	}
}

func TestUrgentSweep_LowScore_MovesToBackground(t *testing.T) {
	batches, mu, signal, flush := captureFlush()
	a := New(Config{
		UrgentWindow:      20 * time.Millisecond,
		BackgroundWindow:  60 * time.Millisecond,
		InterestThreshold: 50, // very high — single event won't reach it
	}, flush)
	defer a.Stop()

	a.Observe(ev(protocol.EventSSHTrap, "1.1.1.1", time.Now()))
	// urgent fires after 20ms — should NOT flush (score below threshold)
	time.Sleep(40 * time.Millisecond)

	mu.Lock()
	if len(*batches) != 0 {
		mu.Unlock()
		t.Fatalf("urgent should have absorbed into background, got %d urgent flushes", len(*batches))
	}
	mu.Unlock()

	// background fires ~60ms after first event landed there
	waitForFlush(t, signal)

	mu.Lock()
	defer mu.Unlock()
	if len(*batches) != 1 {
		t.Fatalf("want 1 background batch, got %d", len(*batches))
	}
	b := (*batches)[0]
	if !b.IsBackground {
		t.Error("expected IsBackground=true")
	}
	if b.EventCount != 1 {
		t.Errorf("EventCount=%d, want 1", b.EventCount)
	}
}

func TestUrgentSweep_GroupsByIP(t *testing.T) {
	batches, mu, signal, flush := captureFlush()
	a := New(Config{
		UrgentWindow:      30 * time.Millisecond,
		InterestThreshold: 0, // always flush as urgent
	}, flush)
	defer a.Stop()

	now := time.Now()
	// 3 distinct IPs.
	a.Observe(ev(protocol.EventSSHTrap, "1.1.1.1", now))
	a.Observe(ev(protocol.EventHTTPTrap, "2.2.2.2", now))
	a.Observe(ev(protocol.EventSSHTrap, "3.3.3.3", now))
	a.Observe(ev(protocol.EventSSHTrap, "1.1.1.1", now.Add(time.Second))) // 1.1.1.1 has 2 events

	waitForFlush(t, signal)

	mu.Lock()
	defer mu.Unlock()
	b := (*batches)[0]
	if len(b.Groups) != 3 {
		t.Fatalf("want 3 groups, got %d: %+v", len(b.Groups), b.Groups)
	}
	if b.EventCount != 4 {
		t.Errorf("EventCount=%d, want 4", b.EventCount)
	}
	// 1.1.1.1 has 2 events → score 20+20=40, others have 1 event → score 30 each.
	// Sort by score desc, ties by event count → 1.1.1.1 first.
	if b.Groups[0].SourceIP != "1.1.1.1" {
		t.Errorf("first group should be most active IP, got %s", b.Groups[0].SourceIP)
	}
}

func TestBackground_AccumulatesAcrossUrgentWindows(t *testing.T) {
	batches, mu, signal, flush := captureFlush()
	a := New(Config{
		UrgentWindow:      15 * time.Millisecond,
		BackgroundWindow:  100 * time.Millisecond,
		InterestThreshold: 100, // never flush as urgent
	}, flush)
	defer a.Stop()

	// Event in window 1.
	a.Observe(ev(protocol.EventSSHTrap, "1.1.1.1", time.Now()))
	time.Sleep(30 * time.Millisecond) // urgent fires, absorbs into background

	// Event in window 2 — different IP.
	a.Observe(ev(protocol.EventSSHTrap, "2.2.2.2", time.Now()))
	time.Sleep(30 * time.Millisecond) // urgent fires, absorbs into background

	// Event in window 3 — same IP as window 1.
	a.Observe(ev(protocol.EventHTTPTrap, "1.1.1.1", time.Now()))

	// Background should fire after total elapsed >= 100ms from FIRST event.
	waitForFlush(t, signal)

	mu.Lock()
	defer mu.Unlock()
	if len(*batches) != 1 {
		t.Fatalf("want 1 background batch, got %d", len(*batches))
	}
	b := (*batches)[0]
	if !b.IsBackground {
		t.Error("expected background batch")
	}
	if b.EventCount != 3 {
		t.Errorf("EventCount=%d, want 3 (1+1+1 across windows)", b.EventCount)
	}
	if len(b.Groups) != 2 {
		t.Errorf("want 2 distinct IPs, got %d", len(b.Groups))
	}
}

func TestStop_PreventsFurtherFlushes(t *testing.T) {
	batches, _, _, flush := captureFlush()
	a := New(Config{UrgentWindow: 30 * time.Millisecond}, flush)

	a.Observe(ev(protocol.EventSSHTrap, "1.1.1.1", time.Now()))
	a.Stop() // before timer fires

	time.Sleep(60 * time.Millisecond)
	if len(*batches) != 0 {
		t.Errorf("Stop should cancel pending sweep, got %d batches", len(*batches))
	}

	// Observe after Stop should be no-op.
	a.Observe(ev(protocol.EventSSHTrap, "2.2.2.2", time.Now()))
	time.Sleep(60 * time.Millisecond)
	if len(*batches) != 0 {
		t.Errorf("Observe after Stop should not enqueue, got %d batches", len(*batches))
	}
}

func TestBuildBatch_SortsBySoreDescThenCount(t *testing.T) {
	now := time.Now()
	byIP := map[string][]protocol.EventRequest{
		"low":     {ev(protocol.EventSSHTrap, "low", now)},                                 // 30
		"highest": {ev(protocol.EventSSHTrap, "highest", now), ev(protocol.EventHTTPTrap, "highest", now.Add(time.Second))}, // 60
		"mid":     {ev(protocol.EventSSHTrap, "mid", now), ev(protocol.EventSSHTrap, "mid", now.Add(time.Second))},          // 40
	}
	b := buildBatch(now, now.Add(time.Minute), byIP, false)

	if len(b.Groups) != 3 {
		t.Fatalf("want 3 groups, got %d", len(b.Groups))
	}
	if b.Groups[0].SourceIP != "highest" {
		t.Errorf("first should be 'highest' (highest score), got %s", b.Groups[0].SourceIP)
	}
	if b.Groups[2].SourceIP != "low" {
		t.Errorf("last should be 'low', got %s", b.Groups[2].SourceIP)
	}
}

func TestBuildBatch_TotalScoreClampedTo100(t *testing.T) {
	now := time.Now()
	byIP := map[string][]protocol.EventRequest{
		"a": {ev(protocol.EventSSHTrap, "a", now), ev(protocol.EventSSHTrap, "a", now.Add(time.Second)), ev(protocol.EventSSHTrap, "a", now.Add(2*time.Second))},
		"b": {ev(protocol.EventHTTPTrap, "b", now), ev(protocol.EventHTTPTrap, "b", now.Add(time.Second)), ev(protocol.EventHTTPTrap, "b", now.Add(2*time.Second))},
		"c": {ev(protocol.EventDBTrap, "c", now), ev(protocol.EventDBTrap, "c", now.Add(time.Second)), ev(protocol.EventDBTrap, "c", now.Add(2*time.Second))},
	}
	b := buildBatch(now, now.Add(time.Minute), byIP, false)
	if b.TotalScore != 100 {
		t.Errorf("expected TotalScore clamped to 100, got %d", b.TotalScore)
	}
}
