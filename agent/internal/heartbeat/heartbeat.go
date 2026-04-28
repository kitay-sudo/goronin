// Package heartbeat sends a periodic "agent still alive" message to
// Telegram so the operator can tell from their phone that the daemon is
// up — separate from the alert channel, which only fires when something
// happens. Interval is config.watch.heartbeat_hours; 0 disables the
// service entirely.
//
// The package owns no state of its own beyond the ticker — counters
// (ticks observed, alerts sent) live on the Alerter, and the formatter
// lives in the telegram package. This file is just the wiring.
package heartbeat

import (
	"context"
	"log"
	"time"

	"github.com/kitay-sudo/goronin/agent/internal/telegram"
)

// CounterReader is the slice of *alerter.Alerter we depend on. Defined as
// an interface so the test can drop in a fake without dragging the rest
// of alerter (and its AI/aggregator deps) into the test binary.
type CounterReader interface {
	Ticks() int64
	AlertsSinceHeartbeatReset() int64
}

// Sender is the slice of *telegram.Client we use. Same interface trick as
// CounterReader.
type Sender interface {
	Send(ctx context.Context, html string) error
}

// Service is a goroutine that ticks every Interval and posts a
// FormatHeartbeat message. Construction is via Start; Stop ends it.
type Service struct {
	serverName string
	interval   time.Duration
	startedAt  time.Time
	counters   CounterReader
	sender     Sender
	stop       chan struct{}
	done       chan struct{}
}

// Start launches the heartbeat goroutine. Returns nil if interval <= 0
// (the service is disabled). serverName/sender/counters must be non-nil
// when interval > 0; the daemon constructs them all before this point so
// it's a programming error to violate that.
//
// startedAt should be the daemon start time (so "Uptime watch" survives
// across heartbeat ticks rather than measuring time-since-last-tick).
func Start(serverName string, interval time.Duration, startedAt time.Time, counters CounterReader, sender Sender) *Service {
	if interval <= 0 {
		log.Printf("[heartbeat] disabled (heartbeat_hours=0)")
		return nil
	}
	s := &Service{
		serverName: serverName,
		interval:   interval,
		startedAt:  startedAt,
		counters:   counters,
		sender:     sender,
		stop:       make(chan struct{}),
		done:       make(chan struct{}),
	}
	go s.run()
	log.Printf("[heartbeat] enabled, interval=%s", interval)
	return s
}

// Stop signals the goroutine to exit and waits for it to finish. Safe to
// call on a nil receiver (matches Start's nil-on-disabled return).
func (s *Service) Stop() {
	if s == nil {
		return
	}
	close(s.stop)
	<-s.done
}

func (s *Service) run() {
	defer close(s.done)
	t := time.NewTicker(s.interval)
	defer t.Stop()
	for {
		select {
		case <-s.stop:
			return
		case <-t.C:
			s.tick()
		}
	}
}

// tick is the one-shot path called by run on every ticker beat. Pulled
// out so the test can drive it directly without sleeping for the
// configured interval.
func (s *Service) tick() {
	uptime := time.Since(s.startedAt)
	ticks := s.counters.Ticks()
	alerts := s.counters.AlertsSinceHeartbeatReset()
	msg := telegram.FormatHeartbeat(s.serverName, uptime, ticks, alerts)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := s.sender.Send(ctx, msg); err != nil {
		log.Printf("[heartbeat] send failed: %v", err)
	}
}
