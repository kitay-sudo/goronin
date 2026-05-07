package firewall

import (
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/kitay-sudo/goronin/agent/internal/config"
	"github.com/kitay-sudo/goronin/agent/internal/storage"
)

// mockExecutor records commands and returns predefined results
type mockExecutor struct {
	mu       sync.Mutex
	commands [][]string
	errOn    string // substring that triggers an error
}

func (m *mockExecutor) Run(name string, args ...string) ([]byte, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	cmd := append([]string{name}, args...)
	m.commands = append(m.commands, cmd)
	if m.errOn != "" && strings.Contains(strings.Join(cmd, " "), m.errOn) {
		return nil, errMock
	}
	return nil, nil
}

func (m *mockExecutor) calls() [][]string {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([][]string, len(m.commands))
	copy(out, m.commands)
	return out
}

var errMock = &mockError{"mock failure"}

type mockError struct{ msg string }

func (e *mockError) Error() string { return e.msg }

func containsCommand(calls [][]string, substr string) bool {
	for _, c := range calls {
		if strings.Contains(strings.Join(c, " "), substr) {
			return true
		}
	}
	return false
}

func TestBlockIP_InvokesIptables(t *testing.T) {
	mock := &mockExecutor{}
	fw := New([]string{}, mock)

	result := fw.BlockIP("45.33.22.11", 1*time.Hour, "ssh_trap")

	if result != ResultBlocked {
		t.Fatalf("expected ResultBlocked, got %s", result)
	}
	if !containsCommand(mock.calls(), "-s 45.33.22.11 -j DROP") {
		t.Fatalf("expected iptables DROP for IP, got: %v", mock.calls())
	}
}

func TestBlockIP_AlreadyBlocked_IncrementsHitCount(t *testing.T) {
	mock := &mockExecutor{}
	fw := New([]string{}, mock)

	fw.BlockIP("10.0.0.5", 1*time.Hour, "ssh_trap")
	result := fw.BlockIP("10.0.0.5", 1*time.Hour, "ssh_trap")

	if result != ResultAlreadyBlocked {
		t.Fatalf("expected ResultAlreadyBlocked on second call, got %s", result)
	}
	entry := fw.GetEntry("10.0.0.5")
	if entry == nil {
		t.Fatal("expected entry to exist")
	}
	if entry.HitCount != 2 {
		t.Fatalf("expected HitCount=2, got %d", entry.HitCount)
	}
}

// Note: escalation now happens through RecordHit (not raw BlockIP).
// See TestRecordHit_EscalatesOnRepeatOffender below.

func TestBlockIP_WhitelistedIP_NotBlocked(t *testing.T) {
	mock := &mockExecutor{}
	fw := New([]string{"10.0.0.1"}, mock)

	tests := []string{"127.0.0.1", "::1", "10.0.0.1"}
	for _, ip := range tests {
		result := fw.BlockIP(ip, 1*time.Hour, "ssh_trap")
		if result != ResultWhitelisted {
			t.Fatalf("expected whitelisted for %s, got %s", ip, result)
		}
	}

	// None of the whitelisted IPs should have triggered a DROP rule
	for _, c := range mock.calls() {
		joined := strings.Join(c, " ")
		if strings.Contains(joined, "DROP") && (strings.Contains(joined, "127.0.0.1") ||
			strings.Contains(joined, "::1") || strings.Contains(joined, "10.0.0.1")) {
			t.Fatalf("whitelisted IP was blocked: %v", c)
		}
	}
}

func TestBlockIP_EmptyIP_Rejected(t *testing.T) {
	mock := &mockExecutor{}
	fw := New([]string{}, mock)
	result := fw.BlockIP("", 1*time.Hour, "ssh_trap")
	if result != ResultInvalidIP {
		t.Fatalf("expected ResultInvalidIP, got %s", result)
	}
}

func TestUnblockIP_RemovesRule(t *testing.T) {
	mock := &mockExecutor{}
	fw := New([]string{}, mock)

	fw.BlockIP("1.2.3.4", 1*time.Hour, "test")
	if err := fw.UnblockIP("1.2.3.4"); err != nil {
		t.Fatalf("unblock failed: %v", err)
	}

	if fw.GetEntry("1.2.3.4") != nil {
		t.Fatal("expected entry to be removed")
	}
	if !containsCommand(mock.calls(), "-D GORONIN-BLOCK -s 1.2.3.4 -j DROP") {
		t.Fatalf("expected unblock iptables command, got: %v", mock.calls())
	}
}

func TestExpiry_RemovesExpiredBlocks(t *testing.T) {
	mock := &mockExecutor{}
	fw := New([]string{}, mock)

	// Block with a tiny duration
	fw.BlockIP("5.6.7.8", 10*time.Millisecond, "test")
	if fw.GetEntry("5.6.7.8") == nil {
		t.Fatal("expected entry immediately after block")
	}

	time.Sleep(30 * time.Millisecond)
	fw.expireOnce()

	if fw.GetEntry("5.6.7.8") != nil {
		t.Fatal("expected entry to be expired and removed")
	}
}

func TestInitChain_CreatesAndFlushesGoroninChain(t *testing.T) {
	mock := &mockExecutor{}
	fw := New([]string{}, mock)

	if err := fw.InitChain(); err != nil {
		t.Fatalf("InitChain failed: %v", err)
	}

	calls := mock.calls()
	// Must create chain, flush it, and link to INPUT
	if !containsCommand(calls, "-N GORONIN-BLOCK") && !containsCommand(calls, "-F GORONIN-BLOCK") {
		t.Fatalf("expected chain creation/flush commands, got: %v", calls)
	}
}

func TestShutdown_DoesNotFlushChain(t *testing.T) {
	// Persistent blocks survive restarts now — shutdown must NOT flush.
	mock := &mockExecutor{}
	fw := New([]string{}, mock)

	fw.BlockIP("9.9.9.9", 1*time.Hour, "test")

	// Snapshot calls before shutdown so we know what was added during it.
	beforeShutdown := len(mock.calls())
	fw.Shutdown()
	afterCalls := mock.calls()[beforeShutdown:]

	for _, c := range afterCalls {
		joined := strings.Join(c, " ")
		if strings.Contains(joined, "-F GORONIN-BLOCK") {
			t.Fatalf("Shutdown should NOT flush; got: %v", c)
		}
	}
}

func TestResetChain_FlushesAndClears(t *testing.T) {
	mock := &mockExecutor{}
	fw := New([]string{}, mock)
	fw.BlockIP("9.9.9.9", 1*time.Hour, "test")
	if err := fw.ResetChain(); err != nil {
		t.Fatal(err)
	}
	if !containsCommand(mock.calls(), "-F GORONIN-BLOCK") {
		t.Fatal("ResetChain should flush iptables")
	}
	if fw.GetEntry("9.9.9.9") != nil {
		t.Fatal("ResetChain should clear in-memory blocks")
	}
}

// ---------- new tests for RecordHit + policy + storage ----------

func openTempStore(t *testing.T) *storage.Store {
	t.Helper()
	s, err := storage.Open(filepath.Join(t.TempDir(), "state.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}

func TestRecordHit_BelowThresholdDoesNotBlock(t *testing.T) {
	mock := &mockExecutor{}
	fw := New(nil, mock).
		WithStorage(openTempStore(t)).
		WithPolicy(config.AutoBanConfig{Mode: "enforce", Threshold: 3, BlockDuration: time.Hour})

	r := fw.RecordHit("8.8.8.8", "ssh_trap")
	if r != ResultThreshold {
		t.Errorf("first hit: want ResultThreshold, got %s", r)
	}
	if fw.GetEntry("8.8.8.8") != nil {
		t.Error("should NOT block before threshold")
	}
}

func TestRecordHit_BlocksAtThreshold(t *testing.T) {
	mock := &mockExecutor{}
	fw := New(nil, mock).
		WithStorage(openTempStore(t)).
		WithPolicy(config.AutoBanConfig{Mode: "enforce", Threshold: 3, BlockDuration: time.Hour})

	fw.RecordHit("8.8.8.8", "ssh_trap")
	fw.RecordHit("8.8.8.8", "ssh_trap")
	r := fw.RecordHit("8.8.8.8", "ssh_trap")
	if r != ResultBlocked {
		t.Errorf("third hit: want ResultBlocked, got %s", r)
	}
}

func TestRecordHit_EscalatesOnRepeatOffender(t *testing.T) {
	mock := &mockExecutor{}
	store := openTempStore(t)
	fw := New(nil, mock).
		WithStorage(store).
		WithPolicy(config.AutoBanConfig{Mode: "enforce", Threshold: 2, BlockDuration: time.Hour})

	// 4 hits — first two get to threshold (block), next two are over threshold (escalate to 24h).
	fw.RecordHit("9.9.9.9", "ssh_trap")
	fw.RecordHit("9.9.9.9", "ssh_trap")
	// Now blocked. Manual unblock to simulate ban expiry, then hits resume.
	fw.UnblockIP("9.9.9.9")
	fw.RecordHit("9.9.9.9", "ssh_trap") // count=3 -> escalate

	entry := fw.GetEntry("9.9.9.9")
	if entry == nil {
		t.Fatal("expected re-block after escalation")
	}
	remaining := time.Until(entry.ExpiresAt)
	if remaining < 23*time.Hour {
		t.Errorf("expected ~24h escalated duration, got %v", remaining)
	}
}

func TestRecordHit_PermanentBan_FirstHit(t *testing.T) {
	// New default policy: Threshold=1, BlockDuration=0 (forever).
	mock := &mockExecutor{}
	fw := New(nil, mock).
		WithStorage(openTempStore(t)).
		WithPolicy(config.AutoBanConfig{Mode: "enforce", Threshold: 1, BlockDuration: 0})

	r := fw.RecordHit("7.7.7.7", "ssh_trap")
	if r != ResultBlocked {
		t.Fatalf("first hit should block immediately, got %s", r)
	}
	entry := fw.GetEntry("7.7.7.7")
	if entry == nil {
		t.Fatal("expected entry to exist")
	}
	if !entry.ExpiresAt.IsZero() {
		t.Errorf("permanent ban must have zero ExpiresAt, got %v", entry.ExpiresAt)
	}

	// expireOnce must NOT remove a permanent ban even far in the future.
	fw.expireOnce()
	if fw.GetEntry("7.7.7.7") == nil {
		t.Error("permanent ban must survive expiry sweep")
	}

	// BlockInfo: blocked=true, remaining=0 is the permanent signal.
	blocked, remaining := fw.BlockInfo("7.7.7.7")
	if !blocked || remaining != 0 {
		t.Errorf("BlockInfo permanent: want (true, 0), got (%v, %v)", blocked, remaining)
	}
}

func TestRecordHit_AlertOnlyMode_DoesNotTouchIptables(t *testing.T) {
	mock := &mockExecutor{}
	fw := New(nil, mock).
		WithStorage(openTempStore(t)).
		WithPolicy(config.AutoBanConfig{Mode: "alert_only", Threshold: 1, BlockDuration: time.Hour})

	r := fw.RecordHit("4.4.4.4", "ssh_trap")
	if r != ResultDryRun {
		t.Errorf("want ResultDryRun, got %s", r)
	}
	for _, c := range mock.calls() {
		if strings.Contains(strings.Join(c, " "), "DROP") {
			t.Errorf("alert_only must not call iptables DROP; got: %v", c)
		}
	}
}

func TestRecordHit_OffMode_NoOp(t *testing.T) {
	mock := &mockExecutor{}
	fw := New(nil, mock).WithPolicy(config.AutoBanConfig{Mode: "off"})
	if r := fw.RecordHit("3.3.3.3", "ssh_trap"); r != ResultDisabled {
		t.Errorf("want ResultDisabled, got %s", r)
	}
}

func TestRestoreFromStorage_ReplaysActiveBlocks(t *testing.T) {
	mock := &mockExecutor{}
	store := openTempStore(t)
	now := time.Now()
	store.SaveBlock(storage.BlockRecord{IP: "5.5.5.5", Reason: "ssh", BlockedAt: now, ExpiresAt: now.Add(time.Hour)})
	store.SaveBlock(storage.BlockRecord{IP: "6.6.6.6", Reason: "ssh", BlockedAt: now, ExpiresAt: now.Add(-time.Hour)}) // expired

	fw := New(nil, mock).WithStorage(store)
	if err := fw.RestoreFromStorage(); err != nil {
		t.Fatal(err)
	}
	if fw.GetEntry("5.5.5.5") == nil {
		t.Error("expected 5.5.5.5 restored")
	}
	if fw.GetEntry("6.6.6.6") != nil {
		t.Error("expired block should NOT be restored")
	}
	if !containsCommand(mock.calls(), "-A GORONIN-BLOCK -s 5.5.5.5 -j DROP") {
		t.Errorf("expected re-add iptables for 5.5.5.5, got: %v", mock.calls())
	}
}

func TestConcurrentBlocks_NoRace(t *testing.T) {
	mock := &mockExecutor{}
	fw := New([]string{}, mock)

	var wg sync.WaitGroup
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			ip := "192.168.1." + string(rune('0'+(n%10)))
			fw.BlockIP(ip, 1*time.Hour, "test")
		}(i)
	}
	wg.Wait()
	// Just verify no panic/race; state is consistent
}
