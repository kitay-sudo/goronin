// Package storage wraps bbolt to persist agent state across restarts.
//
// Three buckets:
//   - "firewall_hits"  — IP -> serialized HitRecord (auto-ban escalation counter,
//     so a malicious IP that returns after iptables flush still escalates).
//   - "blocks"         — IP -> serialized BlockRecord (active bans with expiry,
//     replayed on startup so reboots don't drop existing protection).
//   - "meta"           — agent metadata (last_start, version).
//
// All values are JSON-encoded for forward-compat (a future agent version
// can read older records and ignore unknown fields).
package storage

import (
	"encoding/json"
	"fmt"
	"time"

	bolt "go.etcd.io/bbolt"
)

// Bucket names. Exported so tests can verify directly.
const (
	BucketFirewallHits = "firewall_hits"
	BucketBlocks       = "blocks"
	BucketMeta         = "meta"
)

// HitRecord tracks how often an IP has tripped traps. Used by the firewall
// to escalate block duration on repeat offenders.
type HitRecord struct {
	IP        string    `json:"ip"`
	Count     int       `json:"count"`
	FirstSeen time.Time `json:"first_seen"`
	LastSeen  time.Time `json:"last_seen"`
}

// BlockRecord is an active iptables ban. Replayed on startup so reboots
// don't grant attackers a fresh start.
type BlockRecord struct {
	IP        string    `json:"ip"`
	Reason    string    `json:"reason"`
	BlockedAt time.Time `json:"blocked_at"`
	ExpiresAt time.Time `json:"expires_at"`
}

// Store is a thin wrapper over a bolt DB. Safe for concurrent use — bbolt
// handles its own locking.
type Store struct {
	db *bolt.DB
}

// Open creates or opens the on-disk database, ensuring required buckets exist.
// Path is typically /var/lib/goronin/state.db with mode 0600.
func Open(path string) (*Store, error) {
	db, err := bolt.Open(path, 0600, &bolt.Options{Timeout: 2 * time.Second})
	if err != nil {
		return nil, fmt.Errorf("open bolt %s: %w", path, err)
	}
	err = db.Update(func(tx *bolt.Tx) error {
		for _, name := range []string{BucketFirewallHits, BucketBlocks, BucketMeta} {
			if _, e := tx.CreateBucketIfNotExists([]byte(name)); e != nil {
				return e
			}
		}
		return nil
	})
	if err != nil {
		db.Close()
		return nil, err
	}
	return &Store{db: db}, nil
}

// Close releases the database file lock.
func (s *Store) Close() error { return s.db.Close() }

// RecordHit increments (or creates) the hit counter for an IP and returns the
// updated record. Atomic — safe to call from concurrent event handlers.
func (s *Store) RecordHit(ip string) (*HitRecord, error) {
	now := time.Now()
	var out HitRecord
	err := s.db.Update(func(tx *bolt.Tx) error {
		b := tx.Bucket([]byte(BucketFirewallHits))
		if existing := b.Get([]byte(ip)); existing != nil {
			if err := json.Unmarshal(existing, &out); err != nil {
				return err
			}
			out.Count++
			out.LastSeen = now
		} else {
			out = HitRecord{IP: ip, Count: 1, FirstSeen: now, LastSeen: now}
		}
		data, err := json.Marshal(out)
		if err != nil {
			return err
		}
		return b.Put([]byte(ip), data)
	})
	if err != nil {
		return nil, err
	}
	return &out, nil
}

// GetHit returns the current hit record for an IP or nil if unknown.
func (s *Store) GetHit(ip string) (*HitRecord, error) {
	var out *HitRecord
	err := s.db.View(func(tx *bolt.Tx) error {
		raw := tx.Bucket([]byte(BucketFirewallHits)).Get([]byte(ip))
		if raw == nil {
			return nil
		}
		var rec HitRecord
		if err := json.Unmarshal(raw, &rec); err != nil {
			return err
		}
		out = &rec
		return nil
	})
	return out, err
}

// SaveBlock persists an active block. Replaces any prior record for the same IP.
func (s *Store) SaveBlock(rec BlockRecord) error {
	return s.db.Update(func(tx *bolt.Tx) error {
		data, err := json.Marshal(rec)
		if err != nil {
			return err
		}
		return tx.Bucket([]byte(BucketBlocks)).Put([]byte(rec.IP), data)
	})
}

// DeleteBlock removes the block record (called on unban or expiry).
func (s *Store) DeleteBlock(ip string) error {
	return s.db.Update(func(tx *bolt.Tx) error {
		return tx.Bucket([]byte(BucketBlocks)).Delete([]byte(ip))
	})
}

// ListBlocks returns all active blocks. Used on startup to restore iptables.
func (s *Store) ListBlocks() ([]BlockRecord, error) {
	var out []BlockRecord
	err := s.db.View(func(tx *bolt.Tx) error {
		return tx.Bucket([]byte(BucketBlocks)).ForEach(func(_, v []byte) error {
			var rec BlockRecord
			if err := json.Unmarshal(v, &rec); err != nil {
				return err
			}
			out = append(out, rec)
			return nil
		})
	})
	return out, err
}

// SetMeta writes a string metadata value. Used for version stamps and timestamps.
func (s *Store) SetMeta(key, value string) error {
	return s.db.Update(func(tx *bolt.Tx) error {
		return tx.Bucket([]byte(BucketMeta)).Put([]byte(key), []byte(value))
	})
}

// GetMeta returns the string value for a meta key, or empty string if absent.
func (s *Store) GetMeta(key string) (string, error) {
	var out string
	err := s.db.View(func(tx *bolt.Tx) error {
		v := tx.Bucket([]byte(BucketMeta)).Get([]byte(key))
		if v != nil {
			out = string(v)
		}
		return nil
	})
	return out, err
}
