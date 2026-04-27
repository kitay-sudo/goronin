package storage

import (
	"path/filepath"
	"testing"
	"time"
)

func openTemp(t *testing.T) *Store {
	t.Helper()
	s, err := Open(filepath.Join(t.TempDir(), "state.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}

func TestRecordHit_IncrementsAcrossCalls(t *testing.T) {
	s := openTemp(t)
	for i := 1; i <= 3; i++ {
		rec, err := s.RecordHit("1.2.3.4")
		if err != nil {
			t.Fatal(err)
		}
		if rec.Count != i {
			t.Errorf("call %d: want count=%d got %d", i, i, rec.Count)
		}
	}
}

func TestRecordHit_DistinctIPsTrackedSeparately(t *testing.T) {
	s := openTemp(t)
	s.RecordHit("1.1.1.1")
	s.RecordHit("2.2.2.2")
	s.RecordHit("1.1.1.1")

	a, _ := s.GetHit("1.1.1.1")
	b, _ := s.GetHit("2.2.2.2")
	if a == nil || a.Count != 2 {
		t.Errorf("1.1.1.1: %+v", a)
	}
	if b == nil || b.Count != 1 {
		t.Errorf("2.2.2.2: %+v", b)
	}
}

func TestBlocks_RoundTripAndDelete(t *testing.T) {
	s := openTemp(t)
	rec := BlockRecord{
		IP:        "9.9.9.9",
		Reason:    "ssh_trap",
		BlockedAt: time.Now(),
		ExpiresAt: time.Now().Add(time.Hour),
	}
	if err := s.SaveBlock(rec); err != nil {
		t.Fatal(err)
	}
	list, err := s.ListBlocks()
	if err != nil || len(list) != 1 {
		t.Fatalf("list: %v len=%d", err, len(list))
	}
	if list[0].IP != "9.9.9.9" || list[0].Reason != "ssh_trap" {
		t.Errorf("round-trip mismatch: %+v", list[0])
	}

	if err := s.DeleteBlock("9.9.9.9"); err != nil {
		t.Fatal(err)
	}
	list, _ = s.ListBlocks()
	if len(list) != 0 {
		t.Errorf("expected empty after delete, got %d", len(list))
	}
}

func TestMeta_RoundTrip(t *testing.T) {
	s := openTemp(t)
	if err := s.SetMeta("version", "1.2.3"); err != nil {
		t.Fatal(err)
	}
	v, err := s.GetMeta("version")
	if err != nil || v != "1.2.3" {
		t.Errorf("got %q err=%v", v, err)
	}
	missing, _ := s.GetMeta("nope")
	if missing != "" {
		t.Errorf("expected empty for missing key, got %q", missing)
	}
}

func TestPersistsAcrossReopen(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "state.db")

	s1, _ := Open(path)
	s1.RecordHit("5.5.5.5")
	s1.RecordHit("5.5.5.5")
	s1.SaveBlock(BlockRecord{IP: "5.5.5.5", Reason: "ssh", ExpiresAt: time.Now().Add(time.Hour)})
	s1.Close()

	s2, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer s2.Close()

	hit, _ := s2.GetHit("5.5.5.5")
	if hit == nil || hit.Count != 2 {
		t.Errorf("hit lost across reopen: %+v", hit)
	}
	blocks, _ := s2.ListBlocks()
	if len(blocks) != 1 {
		t.Errorf("blocks lost across reopen: %d", len(blocks))
	}
}
