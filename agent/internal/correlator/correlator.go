// Package correlator groups events from the same IP into "attack chains"
// and assigns each chain a 0-100 threat score based on event count,
// type diversity, time density and known multi-step patterns.
//
// All state lives in memory and ages out (default: 30 min window). Process
// is single-threaded behind a mutex — events arrive one at a time from
// trap/watcher callbacks, no need for sharding.
//
// Scoring is intentionally simple and deterministic so it is testable
// without an LLM and behaves predictably across releases.
package correlator

import (
	"sort"
	"sync"
	"time"

	"github.com/kitay-sudo/goronin/agent/pkg/protocol"
)

// AlertThreshold is the minimum chain score that should trigger an
// enhanced "ATTACK CHAIN" Telegram alert (vs. a per-event alert).
const AlertThreshold = 50

// Default window: events older than this are dropped from the chain.
const defaultWindow = 30 * time.Minute

// pattern is a known multi-step sequence; if a chain's events contain
// this subsequence (order-preserving), score gets a bonus.
type pattern struct {
	name  string
	seq   []string
	bonus int
}

var knownPatterns = []pattern{
	{name: "ssh_to_exfiltration", seq: []string{protocol.EventSSHTrap, protocol.EventFileCanary}, bonus: 50},
	{name: "web_to_db_to_exfil", seq: []string{protocol.EventHTTPTrap, protocol.EventDBTrap, protocol.EventFileCanary}, bonus: 50},
	{name: "ftp_to_exfil", seq: []string{protocol.EventFTPTrap, protocol.EventFileCanary}, bonus: 40},
}

// Chain is the result of correlating one IP's recent events.
type Chain struct {
	SourceIP string
	Events   []protocol.EventRequest
	Score    int
}

// Correlator holds chains keyed by source IP. Concurrent-safe.
type Correlator struct {
	mu     sync.Mutex
	chains map[string]*Chain
	window time.Duration
	now    func() time.Time // injected for tests
}

// New returns a correlator with the given window. Pass 0 for the default.
func New(window time.Duration) *Correlator {
	if window <= 0 {
		window = defaultWindow
	}
	return &Correlator{
		chains: make(map[string]*Chain),
		window: window,
		now:    time.Now,
	}
}

// Observe records an event and returns the updated chain. Returns nil if
// the event has no SourceIP or it's a localhost event (file canary triggers
// don't have a meaningful network IP, so they're attributed to localhost
// and get their own degenerate "chain").
func (c *Correlator) Observe(ev protocol.EventRequest) *Chain {
	if ev.SourceIP == "" {
		return nil
	}
	if ev.CreatedAt.IsZero() {
		ev.CreatedAt = c.now()
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	c.expireLocked()

	chain, ok := c.chains[ev.SourceIP]
	if !ok {
		chain = &Chain{SourceIP: ev.SourceIP}
		c.chains[ev.SourceIP] = chain
	}
	chain.Events = append(chain.Events, ev)
	chain.Score = CalculateScore(chain.Events)

	// Return a copy so callers can read it without holding the lock.
	cp := *chain
	cp.Events = append([]protocol.EventRequest{}, chain.Events...)
	return &cp
}

// expireLocked drops events outside the window and removes empty chains.
// Caller must hold c.mu.
func (c *Correlator) expireLocked() {
	cutoff := c.now().Add(-c.window)
	for ip, chain := range c.chains {
		fresh := chain.Events[:0]
		for _, ev := range chain.Events {
			if ev.CreatedAt.After(cutoff) {
				fresh = append(fresh, ev)
			}
		}
		if len(fresh) == 0 {
			delete(c.chains, ip)
			continue
		}
		chain.Events = fresh
		chain.Score = CalculateScore(fresh)
	}
}

// CalculateScore is the pure scoring function — easy to unit-test.
//
//	+10 per event
//	+20 per unique trap type
//	+30 if file_canary is present (data-exfil signal)
//	+20 if 3+ events in under 5 minutes
//	+pattern.bonus for the highest-matching known pattern (only one applied)
//	clamped to [0, 100]
func CalculateScore(events []protocol.EventRequest) int {
	if len(events) == 0 {
		return 0
	}
	score := len(events) * 10

	uniq := map[string]struct{}{}
	for _, ev := range events {
		uniq[ev.Type] = struct{}{}
	}
	score += len(uniq) * 20

	if _, hasCanary := uniq[protocol.EventFileCanary]; hasCanary {
		score += 30
	}

	if len(events) >= 3 {
		sorted := append([]protocol.EventRequest{}, events...)
		sort.Slice(sorted, func(i, j int) bool { return sorted[i].CreatedAt.Before(sorted[j].CreatedAt) })
		span := sorted[len(sorted)-1].CreatedAt.Sub(sorted[0].CreatedAt)
		if span < 5*time.Minute {
			score += 20
		}
	}

	// Bonus: highest-matching known pattern, applied once.
	ordered := append([]protocol.EventRequest{}, events...)
	sort.Slice(ordered, func(i, j int) bool { return ordered[i].CreatedAt.Before(ordered[j].CreatedAt) })
	types := make([]string, 0, len(ordered))
	for _, ev := range ordered {
		types = append(types, ev.Type)
	}
	for _, p := range knownPatterns {
		if containsSubsequence(types, p.seq) {
			score += p.bonus
			break
		}
	}

	if score > 100 {
		score = 100
	}
	if score < 0 {
		score = 0
	}
	return score
}

// containsSubsequence reports whether needle appears as an order-preserving
// (but not necessarily contiguous) subsequence inside haystack.
func containsSubsequence(haystack, needle []string) bool {
	i := 0
	for _, item := range haystack {
		if item == needle[i] {
			i++
			if i == len(needle) {
				return true
			}
		}
	}
	return false
}
