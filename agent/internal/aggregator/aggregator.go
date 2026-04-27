// Package aggregator implements two-bucket batching of trap events to keep
// Telegram noise and LLM token spend under control.
//
// The model:
//
//   - Every event Observe()'d goes into the urgent bucket.
//   - On the first event in an empty urgent bucket, a 5-minute timer starts.
//   - When that timer fires, the bucket is "swept": its events are grouped
//     by source_ip, an aggregate score is computed for each group, and the
//     whole batch is handed to the user-supplied flusher.
//   - The flusher decides what to do with it (Telegram + AI in production).
//   - If at sweep time the entire batch's combined score is below the
//     "interesting" threshold (default 30), the events are MOVED to the
//     background bucket instead of being sent — they accumulate quietly.
//   - The background bucket has its own 1-hour timer; on fire it produces
//     a single low-noise digest "N events from M IPs over the last hour".
//
// File-canary writes/removes bypass aggregation entirely — the alerter
// observes them directly without going through the aggregator (see main.go).
//
// Concurrency: Observe and the flusher callback run on different goroutines,
// guarded by a single mutex. Sweep callbacks are invoked OUTSIDE the lock so
// they can do network I/O without blocking incoming events.
package aggregator

import (
	"sort"
	"sync"
	"time"

	"github.com/kitay-sudo/goronin/agent/internal/correlator"
	"github.com/kitay-sudo/goronin/agent/pkg/protocol"
)

// Defaults — tuned for "small-to-medium production server" workloads.
// Override via Config when constructing.
const (
	DefaultUrgentWindow      = 5 * time.Minute
	DefaultBackgroundWindow  = 1 * time.Hour
	DefaultInterestThreshold = 30 // batches with combined score below this go to background
)

// IPGroup is the per-IP slice of a Batch.
type IPGroup struct {
	SourceIP string
	Events   []protocol.EventRequest
	Score    int // 0–100, computed by correlator.CalculateScore-equivalent
}

// Batch is what the flusher receives. It is read-only from the flusher's
// perspective — Aggregator builds it under lock and hands it off.
type Batch struct {
	// Window the batch covers (StartedAt = first event, ClosedAt = sweep time).
	StartedAt time.Time
	ClosedAt  time.Time

	// Per-IP slices, sorted by Score desc (most interesting first).
	Groups []IPGroup

	// Sum of all per-group scores, clamped to [0, 100]. Used by the flusher
	// to decide whether to call the LLM.
	TotalScore int

	// Total events across all groups (= sum of len(g.Events) for g in Groups).
	EventCount int

	// True if this batch came from the urgent bucket (5-min sweep), false
	// if from the background digest (1-hour sweep). The flusher uses this
	// to choose the right Telegram template and decide on AI usage.
	IsBackground bool
}

// FlushFunc is invoked when a bucket's timer fires and produces a non-empty
// batch. The aggregator does NOT block on it — flushing happens in a
// goroutine. The flusher is responsible for AI calls, Telegram, error logs.
type FlushFunc func(Batch)

// Config holds tunables. Zero values mean "use the defaults above".
type Config struct {
	UrgentWindow      time.Duration
	BackgroundWindow  time.Duration
	InterestThreshold int
}

func (c Config) withDefaults() Config {
	if c.UrgentWindow <= 0 {
		c.UrgentWindow = DefaultUrgentWindow
	}
	if c.BackgroundWindow <= 0 {
		c.BackgroundWindow = DefaultBackgroundWindow
	}
	if c.InterestThreshold <= 0 {
		c.InterestThreshold = DefaultInterestThreshold
	}
	return c
}

// Aggregator is the live batching state. Single instance per agent.
type Aggregator struct {
	cfg   Config
	flush FlushFunc

	mu sync.Mutex
	// urgent and background hold pending events keyed by source IP.
	urgent     map[string][]protocol.EventRequest
	background map[string][]protocol.EventRequest
	urgentStartedAt     time.Time
	backgroundStartedAt time.Time

	// Active timers (nil when bucket is empty). Replaced under lock.
	urgentTimer     *time.Timer
	backgroundTimer *time.Timer

	// Override for tests — both default to time.Now.
	now func() time.Time

	stopped bool
}

// New constructs an aggregator. flush is called (in its own goroutine)
// every time a bucket sweep produces a non-empty Batch.
func New(cfg Config, flush FlushFunc) *Aggregator {
	return &Aggregator{
		cfg:        cfg.withDefaults(),
		flush:      flush,
		urgent:     make(map[string][]protocol.EventRequest),
		background: make(map[string][]protocol.EventRequest),
		now:        time.Now,
	}
}

// Observe records an event. It returns immediately — actual work happens
// when a bucket timer fires. Empty SourceIP / "localhost" events are
// dropped (they have no meaningful aggregation key).
func (a *Aggregator) Observe(ev protocol.EventRequest) {
	if ev.SourceIP == "" || ev.SourceIP == "localhost" {
		return
	}
	if ev.CreatedAt.IsZero() {
		ev.CreatedAt = a.now()
	}

	a.mu.Lock()
	defer a.mu.Unlock()

	if a.stopped {
		return
	}

	// First event after empty urgent bucket → start the urgent timer.
	if len(a.urgent) == 0 {
		a.urgentStartedAt = ev.CreatedAt
		a.urgentTimer = time.AfterFunc(a.cfg.UrgentWindow, a.sweepUrgent)
	}
	a.urgent[ev.SourceIP] = append(a.urgent[ev.SourceIP], ev)
}

// Stop cancels pending timers. Any events still in buckets are dropped —
// callers should call FlushNow first if they want to drain.
func (a *Aggregator) Stop() {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.stopped = true
	if a.urgentTimer != nil {
		a.urgentTimer.Stop()
		a.urgentTimer = nil
	}
	if a.backgroundTimer != nil {
		a.backgroundTimer.Stop()
		a.backgroundTimer = nil
	}
}

// sweepUrgent runs when the urgent timer fires. It builds a Batch from
// the urgent bucket, decides whether to flush it as urgent or move to
// background, and resets the urgent state.
func (a *Aggregator) sweepUrgent() {
	a.mu.Lock()
	if a.stopped || len(a.urgent) == 0 {
		a.urgent = make(map[string][]protocol.EventRequest)
		a.urgentTimer = nil
		a.mu.Unlock()
		return
	}

	batch := buildBatch(a.urgentStartedAt, a.now(), a.urgent, false)

	// Reset urgent state regardless of where the batch goes next.
	a.urgent = make(map[string][]protocol.EventRequest)
	a.urgentTimer = nil
	a.urgentStartedAt = time.Time{}

	// Below threshold → move to background, don't flush yet.
	if batch.TotalScore < a.cfg.InterestThreshold {
		a.absorbIntoBackgroundLocked(batch)
		a.mu.Unlock()
		return
	}

	a.mu.Unlock()
	go a.flush(batch)
}

// absorbIntoBackgroundLocked moves a low-score urgent batch into the
// background bucket. Caller must hold a.mu.
func (a *Aggregator) absorbIntoBackgroundLocked(b Batch) {
	if len(a.background) == 0 {
		// First content in background → start the hour timer and remember
		// when this digest period started (use the urgent batch's StartedAt
		// so the digest reflects "the last hour" accurately).
		a.backgroundStartedAt = b.StartedAt
		a.backgroundTimer = time.AfterFunc(a.cfg.BackgroundWindow, a.sweepBackground)
	}
	for _, g := range b.Groups {
		a.background[g.SourceIP] = append(a.background[g.SourceIP], g.Events...)
	}
}

// sweepBackground runs when the background timer fires. Produces a single
// low-noise digest batch.
func (a *Aggregator) sweepBackground() {
	a.mu.Lock()
	if a.stopped || len(a.background) == 0 {
		a.background = make(map[string][]protocol.EventRequest)
		a.backgroundTimer = nil
		a.mu.Unlock()
		return
	}

	batch := buildBatch(a.backgroundStartedAt, a.now(), a.background, true)
	a.background = make(map[string][]protocol.EventRequest)
	a.backgroundTimer = nil
	a.backgroundStartedAt = time.Time{}
	a.mu.Unlock()

	go a.flush(batch)
}

// buildBatch is a pure helper: turns a per-IP map into a sorted, scored
// Batch. Exposed at package level so tests can exercise it directly.
func buildBatch(startedAt, closedAt time.Time, byIP map[string][]protocol.EventRequest, isBackground bool) Batch {
	groups := make([]IPGroup, 0, len(byIP))
	totalScore := 0
	totalEvents := 0

	for ip, evs := range byIP {
		score := correlator.CalculateScore(evs)
		groups = append(groups, IPGroup{SourceIP: ip, Events: evs, Score: score})
		totalScore += score
		totalEvents += len(evs)
	}

	if totalScore > 100 {
		totalScore = 100
	}

	// Most dangerous IP first; ties broken by event count desc, then IP for
	// determinism (tests rely on stable order).
	sort.Slice(groups, func(i, j int) bool {
		if groups[i].Score != groups[j].Score {
			return groups[i].Score > groups[j].Score
		}
		if len(groups[i].Events) != len(groups[j].Events) {
			return len(groups[i].Events) > len(groups[j].Events)
		}
		return groups[i].SourceIP < groups[j].SourceIP
	})

	return Batch{
		StartedAt:    startedAt,
		ClosedAt:     closedAt,
		Groups:       groups,
		TotalScore:   totalScore,
		EventCount:   totalEvents,
		IsBackground: isBackground,
	}
}
