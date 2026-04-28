package heartbeat

import (
	"context"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

type fakeCounters struct {
	ticks   int64
	alerts  int64
	resetCh chan struct{}
}

func (f *fakeCounters) Ticks() int64 { return atomic.LoadInt64(&f.ticks) }

func (f *fakeCounters) AlertsSinceHeartbeatReset() int64 {
	v := atomic.SwapInt64(&f.alerts, 0)
	if f.resetCh != nil {
		select {
		case f.resetCh <- struct{}{}:
		default:
		}
	}
	return v
}

type fakeSender struct {
	mu   sync.Mutex
	msgs []string
	ch   chan string
}

func (f *fakeSender) Send(_ context.Context, msg string) error {
	f.mu.Lock()
	f.msgs = append(f.msgs, msg)
	f.mu.Unlock()
	if f.ch != nil {
		select {
		case f.ch <- msg:
		default:
		}
	}
	return nil
}

func TestStart_DisabledWhenIntervalZero(t *testing.T) {
	sender := &fakeSender{}
	s := Start("srv", 0, time.Now(), &fakeCounters{}, sender)
	if s != nil {
		t.Fatal("Start with interval=0 should return nil (disabled)")
	}
	// nil-safe Stop is part of the contract.
	s.Stop()

	time.Sleep(20 * time.Millisecond)
	sender.mu.Lock()
	defer sender.mu.Unlock()
	if len(sender.msgs) != 0 {
		t.Errorf("disabled service should not send, got %d msgs", len(sender.msgs))
	}
}

func TestService_TickFormatsAndResetsAlerts(t *testing.T) {
	counters := &fakeCounters{ticks: 287, alerts: 5, resetCh: make(chan struct{}, 1)}
	sender := &fakeSender{ch: make(chan string, 1)}
	// Construct manually so we can drive tick() directly without waiting
	// on the wall-clock ticker.
	s := &Service{
		serverName: "prod-1",
		interval:   time.Hour,
		startedAt:  time.Now().Add(-12*time.Hour - 12*time.Minute),
		counters:   counters,
		sender:     sender,
		stop:       make(chan struct{}),
		done:       make(chan struct{}),
	}
	s.tick()

	if len(sender.msgs) != 1 {
		t.Fatalf("expected 1 send, got %d", len(sender.msgs))
	}
	msg := sender.msgs[0]
	for _, want := range []string{"GORONIN жив", "prod-1", "Тиков: 287", "Алертов: 5"} {
		if !strings.Contains(msg, want) {
			t.Errorf("missing %q in heartbeat:\n%s", want, msg)
		}
	}

	select {
	case <-counters.resetCh:
	default:
		t.Error("AlertsSinceHeartbeatReset was not called")
	}
	if got := atomic.LoadInt64(&counters.alerts); got != 0 {
		t.Errorf("alerts not reset, still %d", got)
	}
}

func TestService_StartTicksThenStops(t *testing.T) {
	counters := &fakeCounters{ticks: 1}
	sender := &fakeSender{ch: make(chan string, 4)}
	s := Start("srv", 20*time.Millisecond, time.Now(), counters, sender)
	if s == nil {
		t.Fatal("Start returned nil for positive interval")
	}
	defer s.Stop()

	select {
	case <-sender.ch:
		// got at least one tick
	case <-time.After(500 * time.Millisecond):
		t.Fatal("ticker did not fire within 500ms")
	}
}

