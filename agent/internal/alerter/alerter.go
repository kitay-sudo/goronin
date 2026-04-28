// Package alerter is the routing layer between the aggregator's batched
// sweeps and the outside world (AI + Telegram).
//
// In v0.2+ the model is "batch-first":
//   - traps and watcher hand events to the aggregator (5-min urgent window
//     with 1-hour background fallback for low-score noise);
//   - aggregator calls FlushBatch when a sweep produces a non-empty Batch;
//   - this package decides AI/Telegram strategy based on Batch.IsBackground
//     and Batch.TotalScore.
//
// File-canary write/remove events bypass the aggregator entirely (see
// HandleInstant) — those are 100% real attacks, no point waiting 5 min.
//
// AI thresholds (defaults in code, overridable via config later):
//   - urgent batch with TotalScore < urgentAIThreshold → no AI (cheap path)
//   - urgent batch with TotalScore ≥ urgentAIThreshold → AnalyzeBatch
//   - background digest → never AI (it's the cheap-by-design path)
//   - instant file-canary → AnalyzeEvent (single event)
package alerter

import (
	"context"
	"fmt"
	"log"
	"sort"
	"sync/atomic"
	"time"

	"github.com/kitay-sudo/goronin/agent/internal/aggregator"
	"github.com/kitay-sudo/goronin/agent/internal/ai"
	"github.com/kitay-sudo/goronin/agent/internal/telegram"
	"github.com/kitay-sudo/goronin/agent/pkg/protocol"
)

// urgentAIThreshold: TotalScore below this means the batch is just routine
// scanning noise — Telegram still gets a short note, but we skip the LLM
// call to save tokens. 30 matches the aggregator's interest threshold so
// that batches reaching urgent flush are also above AI threshold.
const urgentAIThreshold = 30

// aiTimeout caps per-AI-call latency. LLM endpoints can hang under load.
const aiTimeout = 25 * time.Second

// FirewallStatus reports whether an IP is currently blocked and for how long.
// Implemented by *firewall.Firewall — kept as an interface here to avoid a
// circular dep and to keep this package easy to test.
type FirewallStatus interface {
	BlockInfo(ip string) (blocked bool, remaining time.Duration)
}

// Alerter wires aggregator output → AI → Telegram. Single instance.
type Alerter struct {
	serverName string
	provider   ai.Provider
	tg         *telegram.Client
	fw         FirewallStatus // optional — nil means "don't annotate blocks"

	// ticks counts every event observed by the daemon (trap or canary),
	// regardless of whether it ends up in an alert. Used by the heartbeat
	// service as a "system is seeing traffic" pulse. Monotonic since start.
	ticks atomic.Int64

	// alertsSinceHeartbeat counts successful alert-channel Telegram sends
	// (per-event, batch, digest, instant). Reset by the heartbeat service
	// after each tick so the heartbeat reports "alerts since last beat".
	alertsSinceHeartbeat atomic.Int64
}

// New constructs an alerter with the three required dependencies.
func New(serverName string, provider ai.Provider, tg *telegram.Client) *Alerter {
	return &Alerter{
		serverName: serverName,
		provider:   provider,
		tg:         tg,
	}
}

// WithFirewall attaches a firewall status reader so batch alerts can show
// "🛡 заблокирован (1ч)" markers. Returns the alerter for chaining.
func (a *Alerter) WithFirewall(fw FirewallStatus) *Alerter {
	a.fw = fw
	return a
}

// ObserveTick is called by the daemon for every trap/canary event so the
// heartbeat can report "the system saw N events since last beat". Counts
// are incremented before any filtering or AI/Telegram work, so they
// reflect what the daemon actually observed, not what it ended up sending.
func (a *Alerter) ObserveTick() { a.ticks.Add(1) }

// Ticks returns the running event-observed counter. Monotonic from start.
func (a *Alerter) Ticks() int64 { return a.ticks.Load() }

// AlertsSinceHeartbeatReset reads the alert counter and atomically zeroes
// it. Called by the heartbeat service immediately before formatting the
// "still alive" message so each heartbeat reports the count for its own
// window.
func (a *Alerter) AlertsSinceHeartbeatReset() int64 {
	return a.alertsSinceHeartbeat.Swap(0)
}

// sendAlert wraps tg.Send for paths that should bump the alert counter
// (per-event, batch, digest, instant). Startup and heartbeat sends bypass
// this and call tg.Send directly so they don't pollute the operator's
// "alerts since last beat" number.
func (a *Alerter) sendAlert(ctx context.Context, msg string) error {
	if err := a.tg.Send(ctx, msg); err != nil {
		return err
	}
	a.alertsSinceHeartbeat.Add(1)
	return nil
}

// FlushBatch is the aggregator.FlushFunc. Routes urgent vs background to
// different format/AI paths.
func (a *Alerter) FlushBatch(b aggregator.Batch) {
	if b.IsBackground {
		a.sendBackgroundDigest(b)
		return
	}
	a.sendUrgentBatch(b)
}

// HandleInstant is the bypass path for events that should NOT go through
// aggregation (file canary writes/removes). Sends a per-event Telegram
// alert immediately, with AI if available.
func (a *Alerter) HandleInstant(ev protocol.EventRequest) {
	if ev.CreatedAt.IsZero() {
		ev.CreatedAt = time.Now()
	}

	ctx, cancel := context.WithTimeout(context.Background(), aiTimeout)
	defer cancel()

	analysis, err := a.provider.AnalyzeEvent(ctx, ev)
	if err != nil {
		log.Printf("[alerter] instant AI failed: %v", err)
	}

	msg := telegram.FormatEventAlert(a.serverName, ev, analysis)
	if err := a.sendAlert(context.Background(), msg); err != nil {
		log.Printf("[alerter] instant telegram send failed: %v", err)
	}
}

// SendStartup posts a one-shot "agent online" notification with the list
// of active traps, the running version (so updates are visibly confirmed),
// the file canaries currently being watched, and any canaries that failed
// to be created. Called from main after traps and watcher init.
func (a *Alerter) SendStartup(version string, trapDescriptions, canaries, canariesFailed []string) {
	msg := telegram.FormatAgentStartup(a.serverName, version, trapDescriptions, canaries, canariesFailed)
	if err := a.tg.Send(context.Background(), msg); err != nil {
		log.Printf("[alerter] startup telegram send failed: %v", err)
	}
}

// sendUrgentBatch handles a 5-minute window flush. Calls AI only if
// TotalScore is above urgentAIThreshold to avoid LLM spend on background
// noise that happens to surface as urgent (shouldn't happen given the
// aggregator's own threshold, but cheap insurance).
func (a *Alerter) sendUrgentBatch(b aggregator.Batch) {
	summaries := a.summariesFromBatch(b)
	windowMin := minutesBetween(b.StartedAt, b.ClosedAt)

	var analysis string
	if b.TotalScore >= urgentAIThreshold {
		ctx, cancel := context.WithTimeout(context.Background(), aiTimeout)
		defer cancel()

		groups := make([]ai.BatchGroup, 0, len(b.Groups))
		for _, g := range b.Groups {
			groups = append(groups, ai.BatchGroup{
				SourceIP: g.SourceIP,
				Score:    g.Score,
				Events:   g.Events,
			})
		}
		var err error
		analysis, err = a.provider.AnalyzeBatch(ctx, b.TotalScore, groups)
		if err != nil {
			log.Printf("[alerter] urgent batch AI failed (sending without): %v", err)
		}
	}

	msg := telegram.FormatBatchAlert(a.serverName, b.TotalScore, b.EventCount, windowMin, summaries, analysis)
	if err := a.sendAlert(context.Background(), msg); err != nil {
		log.Printf("[alerter] urgent batch telegram send failed: %v", err)
	}
}

// sendBackgroundDigest handles a 1-hour low-noise summary. Never calls AI.
func (a *Alerter) sendBackgroundDigest(b aggregator.Batch) {
	summaries := a.summariesFromBatch(b)
	windowMin := minutesBetween(b.StartedAt, b.ClosedAt)

	msg := telegram.FormatBackgroundDigest(a.serverName, b.EventCount, windowMin, summaries)
	if err := a.sendAlert(context.Background(), msg); err != nil {
		log.Printf("[alerter] background digest telegram send failed: %v", err)
	}
}

// summariesFromBatch flattens an aggregator.Batch into the simpler shape
// used by the Telegram formatter (just IP + counts, no full event slices).
// If a firewall is attached, blocked-state and duration are filled in.
func (a *Alerter) summariesFromBatch(b aggregator.Batch) []telegram.IPSummary {
	out := make([]telegram.IPSummary, 0, len(b.Groups))
	for _, g := range b.Groups {
		typeSet := map[string]struct{}{}
		for _, ev := range g.Events {
			typeSet[shortType(ev.Type)] = struct{}{}
		}
		types := make([]string, 0, len(typeSet))
		for t := range typeSet {
			types = append(types, t)
		}
		sort.Strings(types)

		s := telegram.IPSummary{
			IP:         g.SourceIP,
			Score:      g.Score,
			EventCount: len(g.Events),
			Types:      types,
		}
		if a.fw != nil {
			if blocked, remaining := a.fw.BlockInfo(g.SourceIP); blocked {
				s.Blocked = true
				s.BlockDuration = humanDuration(remaining)
			}
		}
		out = append(out, s)
	}
	return out
}

// humanDuration renders a remaining-block window like "1ч", "24ч", "30мин".
// Aimed at quick-glance Telegram messages, not precision.
func humanDuration(d time.Duration) string {
	if d <= 0 {
		return ""
	}
	if d < time.Hour {
		mins := int(d.Round(time.Minute).Minutes())
		if mins < 1 {
			mins = 1
		}
		return fmt.Sprintf("%dмин", mins)
	}
	hours := int(d.Round(time.Hour).Hours())
	if hours < 1 {
		hours = 1
	}
	return fmt.Sprintf("%dч", hours)
}

func shortType(t string) string {
	switch t {
	case protocol.EventSSHTrap:
		return "SSH"
	case protocol.EventHTTPTrap:
		return "HTTP"
	case protocol.EventFTPTrap:
		return "FTP"
	case protocol.EventDBTrap:
		return "DB"
	case protocol.EventFileCanary:
		return "file"
	}
	return t
}

func minutesBetween(start, end time.Time) int {
	if start.IsZero() || end.IsZero() {
		return 0
	}
	d := end.Sub(start)
	if d < time.Minute {
		return 1
	}
	return int(d.Round(time.Minute).Minutes())
}
