// Package alerter is the routing layer between traps/watcher and the
// outside world. For every event it:
//  1. updates the correlator and gets back the current chain + score,
//  2. fires a per-event Telegram alert (with optional AI analysis),
//  3. if the chain crossed the alert threshold AND we haven't already
//     alerted on it within the cooldown, fires a chain Telegram alert.
//
// Cooldown prevents the same chain from generating an alert per event
// once it's "hot" — a chain at 70/100 with 5 events should produce one
// chain alert, not five.
package alerter

import (
	"context"
	"log"
	"sync"
	"time"

	"github.com/kitay-sudo/goronin/agent/internal/ai"
	"github.com/kitay-sudo/goronin/agent/internal/correlator"
	"github.com/kitay-sudo/goronin/agent/internal/telegram"
	"github.com/kitay-sudo/goronin/agent/pkg/protocol"
)

// chainCooldown is how long after a chain alert before we alert again on
// the same source IP. 30 min matches the default correlator window.
const chainCooldown = 30 * time.Minute

// aiTimeout caps how long we wait for an AI provider per analysis.
// LLM calls can hang under load — we don't want to block trap callbacks.
const aiTimeout = 25 * time.Second

// Alerter wires correlator + ai + telegram. Single instance per agent.
type Alerter struct {
	serverName string
	corr       *correlator.Correlator
	provider   ai.Provider
	tg         *telegram.Client

	mu              sync.Mutex
	lastChainAlerts map[string]time.Time // sourceIP -> last chain alert time
}

// New constructs an alerter. All three dependencies are required;
// pass an ai.noopProvider via ai.New(config.AIConfig{Provider: ""}) to
// disable AI analysis.
func New(serverName string, corr *correlator.Correlator, provider ai.Provider, tg *telegram.Client) *Alerter {
	return &Alerter{
		serverName:      serverName,
		corr:            corr,
		provider:        provider,
		tg:              tg,
		lastChainAlerts: make(map[string]time.Time),
	}
}

// Handle is the EventCallback wired into traps and the file watcher.
// It runs synchronously (so the trap handler returns quickly only
// because the work itself is fast); AI/Telegram are network I/O and
// run in their own context with timeouts.
func (a *Alerter) Handle(ev protocol.EventRequest) {
	if ev.CreatedAt.IsZero() {
		ev.CreatedAt = time.Now()
	}

	chain := a.corr.Observe(ev)

	// Per-event Telegram alert (always, regardless of chain score).
	go a.sendEventAlert(ev)

	// Chain alert when threshold crossed AND not in cooldown.
	if chain != nil && chain.Score >= correlator.AlertThreshold && a.shouldChainAlert(ev.SourceIP) {
		go a.sendChainAlert(ev.SourceIP, chain.Score, chain.Events)
	}
}

func (a *Alerter) shouldChainAlert(ip string) bool {
	a.mu.Lock()
	defer a.mu.Unlock()
	last, seen := a.lastChainAlerts[ip]
	if seen && time.Since(last) < chainCooldown {
		return false
	}
	a.lastChainAlerts[ip] = time.Now()
	return true
}

func (a *Alerter) sendEventAlert(ev protocol.EventRequest) {
	ctx, cancel := context.WithTimeout(context.Background(), aiTimeout)
	defer cancel()

	analysis, err := a.provider.AnalyzeEvent(ctx, ev)
	if err != nil {
		log.Printf("[alerter] event AI failed (sending without analysis): %v", err)
	}

	msg := telegram.FormatEventAlert(a.serverName, ev, analysis)
	if err := a.tg.Send(context.Background(), msg); err != nil {
		log.Printf("[alerter] event telegram send failed: %v", err)
	}
}

func (a *Alerter) sendChainAlert(ip string, score int, events []protocol.EventRequest) {
	ctx, cancel := context.WithTimeout(context.Background(), aiTimeout)
	defer cancel()

	analysis, err := a.provider.AnalyzeChain(ctx, ip, score, events)
	if err != nil {
		log.Printf("[alerter] chain AI failed (sending without analysis): %v", err)
	}

	msg := telegram.FormatChainAlert(a.serverName, ip, score, events, analysis)
	if err := a.tg.Send(context.Background(), msg); err != nil {
		log.Printf("[alerter] chain telegram send failed: %v", err)
	}
}

// SendStartup posts a one-shot "agent online" notification with the list
// of active traps. Called from main after traps start successfully.
func (a *Alerter) SendStartup(trapDescriptions []string) {
	msg := telegram.FormatAgentStartup(a.serverName, trapDescriptions)
	if err := a.tg.Send(context.Background(), msg); err != nil {
		log.Printf("[alerter] startup telegram send failed: %v", err)
	}
}
