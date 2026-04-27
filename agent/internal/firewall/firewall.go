// Package firewall manages IP blocks via iptables in a dedicated
// GORONIN-BLOCK chain.
//
// Two big behaviors that differ from a naïve "first hit = ban":
//
//  1. Threshold-based blocking. RecordHit increments a per-IP counter
//     (persisted via storage) and only blocks when count >= threshold
//     within a sliding window. Below threshold, returns ResultThreshold
//     so the alerter can still fire a "we saw this, didn't block" alert.
//
//  2. Mode awareness. config.AutoBan.Mode = "off" disables blocking
//     entirely; "alert_only" logs would-be blocks without touching
//     iptables (dry-run for the first 24h). "enforce" is the production
//     path.
//
// Persistent state (hits + active blocks) is stored via storage.Store so
// reboots and systemd restarts don't reset escalation counters or grant
// attackers a fresh window.
package firewall

import (
	"fmt"
	"log"
	"os/exec"
	"strings"
	"sync"
	"time"

	"github.com/kitay-sudo/goronin/agent/internal/config"
	"github.com/kitay-sudo/goronin/agent/internal/storage"
)

// ChainName is the dedicated iptables chain.
const ChainName = "GORONIN-BLOCK"

// Result describes what the firewall did.
type Result string

const (
	ResultBlocked        Result = "blocked"
	ResultAlreadyBlocked Result = "already_blocked"
	ResultWhitelisted    Result = "whitelisted"
	ResultInvalidIP      Result = "invalid_ip"
	ResultError          Result = "error"
	ResultThreshold      Result = "below_threshold" // hit recorded but block not yet triggered
	ResultDryRun         Result = "dry_run"         // alert_only mode
	ResultDisabled       Result = "disabled"        // mode=off
)

// escalationDuration is applied when an IP returns after a previous ban
// (HitCount >= threshold AND already had a block record).
const escalationDuration = 24 * time.Hour

// CommandExecutor abstracts running iptables so tests can mock it.
type CommandExecutor interface {
	Run(name string, args ...string) ([]byte, error)
}

// RealExecutor invokes commands via os/exec.
type RealExecutor struct{}

func (RealExecutor) Run(name string, args ...string) ([]byte, error) {
	return exec.Command(name, args...).CombinedOutput()
}

// BlockEntry tracks an active block (mirrors storage.BlockRecord but
// in-memory for fast reads).
type BlockEntry struct {
	IP        string
	Reason    string
	BlockedAt time.Time
	ExpiresAt time.Time
	HitCount  int
}

// Firewall manages blocks via iptables. Construct with New, then call
// InitChain (or InitChainAndRestore if a Store is attached) before Start.
type Firewall struct {
	mu        sync.Mutex
	blocked   map[string]*BlockEntry
	whitelist map[string]bool
	exec      CommandExecutor
	store     *storage.Store     // optional — nil = no persistence (tests)
	policy    config.AutoBanConfig // optional — zero value = enforce/3/5m/1h defaults applied in Decide
	stopCh    chan struct{}
	stopped   bool
}

// New creates a Firewall without persistence — used by tests and as the
// base constructor. The hardcoded localhost entries always count as
// whitelisted.
func New(extraWhitelist []string, exec CommandExecutor) *Firewall {
	wl := map[string]bool{
		"127.0.0.1": true,
		"::1":       true,
		"localhost": true,
	}
	for _, ip := range extraWhitelist {
		wl[strings.TrimSpace(ip)] = true
	}
	return &Firewall{
		blocked:   make(map[string]*BlockEntry),
		whitelist: wl,
		exec:      exec,
		stopCh:    make(chan struct{}),
	}
}

// WithStorage wires a bbolt store for persistent hits and blocks.
// Returns the same firewall for chaining.
func (f *Firewall) WithStorage(s *storage.Store) *Firewall {
	f.store = s
	return f
}

// WithPolicy configures threshold-based blocking from the user's config.
// Returns the same firewall for chaining.
func (f *Firewall) WithPolicy(p config.AutoBanConfig) *Firewall {
	f.policy = p
	return f
}

// InitChain creates the GORONIN-BLOCK chain and links it into INPUT.
// Unlike the old behavior, it does NOT flush — existing blocks survive
// agent restarts. Use ResetChain to wipe.
func (f *Firewall) InitChain() error {
	_, _ = f.exec.Run("iptables", "-N", ChainName)

	// Link only if not already linked.
	if _, err := f.exec.Run("iptables", "-C", "INPUT", "-j", ChainName); err != nil {
		if _, err := f.exec.Run("iptables", "-I", "INPUT", "-j", ChainName); err != nil {
			return fmt.Errorf("link %s to INPUT: %w", ChainName, err)
		}
	}
	return nil
}

// RestoreFromStorage reads persisted blocks and re-applies any that
// haven't expired yet. Called after InitChain on agent start.
func (f *Firewall) RestoreFromStorage() error {
	if f.store == nil {
		return nil
	}
	records, err := f.store.ListBlocks()
	if err != nil {
		return err
	}
	now := time.Now()
	restored := 0
	for _, rec := range records {
		if rec.ExpiresAt.Before(now) {
			_ = f.store.DeleteBlock(rec.IP)
			continue
		}
		if _, err := f.exec.Run("iptables", "-A", ChainName, "-s", rec.IP, "-j", "DROP"); err != nil {
			log.Printf("[firewall] Failed to restore block for %s: %v", rec.IP, err)
			continue
		}
		f.blocked[rec.IP] = &BlockEntry{
			IP:        rec.IP,
			Reason:    rec.Reason,
			BlockedAt: rec.BlockedAt,
			ExpiresAt: rec.ExpiresAt,
		}
		restored++
	}
	if restored > 0 {
		log.Printf("[firewall] Restored %d active block(s) from storage", restored)
	}
	return nil
}

// Start runs the expiry goroutine.
func (f *Firewall) Start() { go f.expiryLoop() }

// Shutdown stops the expiry loop. Does NOT flush iptables — blocks
// survive restarts intentionally.
func (f *Firewall) Shutdown() {
	f.mu.Lock()
	if !f.stopped {
		close(f.stopCh)
		f.stopped = true
	}
	f.mu.Unlock()
}

// ResetChain flushes GORONIN-BLOCK and clears persisted state. Used by
// `goronin reset` for full cleanup.
func (f *Firewall) ResetChain() error {
	if _, err := f.exec.Run("iptables", "-F", ChainName); err != nil {
		return fmt.Errorf("flush: %w", err)
	}
	f.mu.Lock()
	f.blocked = make(map[string]*BlockEntry)
	f.mu.Unlock()
	if f.store != nil {
		for _, rec := range mustList(f.store) {
			_ = f.store.DeleteBlock(rec.IP)
		}
	}
	return nil
}

func mustList(s *storage.Store) []storage.BlockRecord {
	out, _ := s.ListBlocks()
	return out
}

// RecordHit increments the persisted counter for an IP and decides
// whether to block based on policy. This is the entry point the alerter
// should call instead of BlockIP.
//
// Returns:
//   - ResultBlocked       — IP newly blocked
//   - ResultAlreadyBlocked— IP already in active block
//   - ResultThreshold     — hit recorded, threshold not yet reached
//   - ResultDryRun        — would block, mode=alert_only
//   - ResultDisabled      — mode=off
//   - ResultWhitelisted   — IP on whitelist
//   - ResultInvalidIP     — empty IP
func (f *Firewall) RecordHit(ip, reason string) Result {
	ip = strings.TrimSpace(ip)
	if ip == "" {
		return ResultInvalidIP
	}
	if f.whitelist[ip] {
		return ResultWhitelisted
	}

	mode := f.policy.Mode
	if mode == "" {
		mode = "enforce"
	}
	if mode == "off" {
		return ResultDisabled
	}

	threshold := f.policy.Threshold
	if threshold <= 0 {
		threshold = 3
	}
	duration := f.policy.BlockDuration
	if duration <= 0 {
		duration = 1 * time.Hour
	}

	// Track hits in storage (if available) so escalation survives restarts.
	count := 1
	if f.store != nil {
		hit, err := f.store.RecordHit(ip)
		if err != nil {
			log.Printf("[firewall] RecordHit storage error: %v", err)
		} else {
			count = hit.Count
		}
	}

	if count < threshold {
		return ResultThreshold
	}

	// Threshold reached. Escalate duration on repeat offenders (count > threshold).
	if count > threshold {
		duration = escalationDuration
	}

	if mode == "alert_only" {
		log.Printf("[firewall] DRY-RUN would block %s for %v (hits=%d, reason=%s)", ip, duration, count, reason)
		return ResultDryRun
	}
	return f.BlockIP(ip, duration, reason)
}

// BlockIP adds a DROP rule. Lower-level than RecordHit — bypasses policy
// checks, used for canary writes (instant ban, no threshold) and for
// manual `goronin ban`.
func (f *Firewall) BlockIP(ip string, duration time.Duration, reason string) Result {
	ip = strings.TrimSpace(ip)
	if ip == "" {
		return ResultInvalidIP
	}
	if f.whitelist[ip] {
		return ResultWhitelisted
	}

	f.mu.Lock()
	defer f.mu.Unlock()

	now := time.Now()
	if entry, exists := f.blocked[ip]; exists {
		entry.HitCount++
		newExpiry := now.Add(duration)
		if newExpiry.After(entry.ExpiresAt) {
			entry.ExpiresAt = newExpiry
			f.persistBlockLocked(entry)
		}
		return ResultAlreadyBlocked
	}

	if _, err := f.exec.Run("iptables", "-A", ChainName, "-s", ip, "-j", "DROP"); err != nil {
		log.Printf("[firewall] Failed to block %s: %v", ip, err)
		return ResultError
	}
	entry := &BlockEntry{
		IP:        ip,
		Reason:    reason,
		BlockedAt: now,
		ExpiresAt: now.Add(duration),
		HitCount:  1,
	}
	f.blocked[ip] = entry
	f.persistBlockLocked(entry)
	log.Printf("[firewall] Blocked %s for %v (reason: %s)", ip, duration, reason)
	return ResultBlocked
}

// UnblockIP removes the DROP rule and the persisted block.
func (f *Firewall) UnblockIP(ip string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.unblockLocked(ip)
}

func (f *Firewall) unblockLocked(ip string) error {
	if _, err := f.exec.Run("iptables", "-D", ChainName, "-s", ip, "-j", "DROP"); err != nil {
		log.Printf("[firewall] Unblock %s: %v", ip, err)
	}
	delete(f.blocked, ip)
	if f.store != nil {
		_ = f.store.DeleteBlock(ip)
	}
	return nil
}

func (f *Firewall) persistBlockLocked(e *BlockEntry) {
	if f.store == nil {
		return
	}
	_ = f.store.SaveBlock(storage.BlockRecord{
		IP:        e.IP,
		Reason:    e.Reason,
		BlockedAt: e.BlockedAt,
		ExpiresAt: e.ExpiresAt,
	})
}

// GetEntry returns a copy of the BlockEntry for an IP, or nil.
func (f *Firewall) GetEntry(ip string) *BlockEntry {
	f.mu.Lock()
	defer f.mu.Unlock()
	if e, ok := f.blocked[ip]; ok {
		cp := *e
		return &cp
	}
	return nil
}

// ListBlocked returns a snapshot of all active blocks.
func (f *Firewall) ListBlocked() []BlockEntry {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make([]BlockEntry, 0, len(f.blocked))
	for _, e := range f.blocked {
		out = append(out, *e)
	}
	return out
}

// expiryLoop periodically removes expired blocks.
func (f *Firewall) expiryLoop() {
	ticker := time.NewTicker(60 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-f.stopCh:
			return
		case <-ticker.C:
			f.expireOnce()
		}
	}
}

func (f *Firewall) expireOnce() {
	f.mu.Lock()
	defer f.mu.Unlock()
	now := time.Now()
	for ip, entry := range f.blocked {
		if now.After(entry.ExpiresAt) {
			_ = f.unblockLocked(ip)
		}
	}
}
