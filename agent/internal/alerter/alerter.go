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
	"log"
	"sort"
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

// Alerter wires aggregator output → AI → Telegram. Single instance.
type Alerter struct {
	serverName string
	provider   ai.Provider
	tg         *telegram.Client
}

// New constructs an alerter with the three required dependencies.
func New(serverName string, provider ai.Provider, tg *telegram.Client) *Alerter {
	return &Alerter{
		serverName: serverName,
		provider:   provider,
		tg:         tg,
	}
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
	if err := a.tg.Send(context.Background(), msg); err != nil {
		log.Printf("[alerter] instant telegram send failed: %v", err)
	}
}

// SendStartup posts a one-shot "agent online" notification with the list
// of active traps, the running version (so updates are visibly confirmed),
// and the file canaries created at boot. Called from main after traps and
// the watcher have finished initialising.
func (a *Alerter) SendStartup(version string, trapDescriptions, canaries []string) {
	msg := telegram.FormatAgentStartup(a.serverName, version, trapDescriptions, canaries)
	if err := a.tg.Send(context.Background(), msg); err != nil {
		log.Printf("[alerter] startup telegram send failed: %v", err)
	}
}

// sendUrgentBatch handles a 5-minute window flush. Calls AI only if
// TotalScore is above urgentAIThreshold to avoid LLM spend on background
// noise that happens to surface as urgent (shouldn't happen given the
// aggregator's own threshold, but cheap insurance).
func (a *Alerter) sendUrgentBatch(b aggregator.Batch) {
	summaries := summariesFromBatch(b)
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
	if err := a.tg.Send(context.Background(), msg); err != nil {
		log.Printf("[alerter] urgent batch telegram send failed: %v", err)
	}
}

// sendBackgroundDigest handles a 1-hour low-noise summary. Never calls AI.
func (a *Alerter) sendBackgroundDigest(b aggregator.Batch) {
	summaries := summariesFromBatch(b)
	windowMin := minutesBetween(b.StartedAt, b.ClosedAt)

	msg := telegram.FormatBackgroundDigest(a.serverName, b.EventCount, windowMin, summaries)
	if err := a.tg.Send(context.Background(), msg); err != nil {
		log.Printf("[alerter] background digest telegram send failed: %v", err)
	}
}

// summariesFromBatch flattens an aggregator.Batch into the simpler shape
// used by the Telegram formatter (just IP + counts, no full event slices).
func summariesFromBatch(b aggregator.Batch) []telegram.IPSummary {
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
		out = append(out, telegram.IPSummary{
			IP:         g.SourceIP,
			Score:      g.Score,
			EventCount: len(g.Events),
			Types:      types,
		})
	}
	return out
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
