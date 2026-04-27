// Package protocol defines the data types passed between subsystems of
// the agent (traps, watcher, correlator, telegram, ai). Standalone build
// — no backend, all communication is in-process.
package protocol

import "time"

// EventRequest is emitted by traps and watchers when something interesting
// is observed. It is consumed by the correlator and the alerter.
type EventRequest struct {
	Type       string            `json:"type"`
	SourceIP   string            `json:"source_ip"`
	SourcePort int               `json:"source_port,omitempty"`
	TrapPort   int               `json:"trap_port,omitempty"`
	CreatedAt  time.Time         `json:"created_at"`
	Details    map[string]string `json:"details,omitempty"`
}

// Event types
const (
	EventSSHTrap    = "ssh_trap"
	EventHTTPTrap   = "http_trap"
	EventFTPTrap    = "ftp_trap"
	EventDBTrap     = "db_trap"
	EventFileCanary = "file_canary"
)

// Detail keys used in EventRequest.Details for active-defense metadata.
const (
	DetailActionTaken   = "action_taken"   // "blocked" | "already_blocked" | "whitelisted" | "invalid_ip" | "error" | "passive"
	DetailBlockDuration = "block_duration" // human-readable duration, e.g. "1h0m0s"
	DetailBlockReason   = "block_reason"
)
