package watcher

import (
	"os"
	"path/filepath"
	"runtime"
	"sync"
	"testing"
	"time"

	"github.com/kitay-sudo/goronin/agent/pkg/protocol"
)

func TestCanonicalizePath_SameKeyForVariants(t *testing.T) {
	tmp := t.TempDir()
	real := filepath.Join(tmp, "secret.env")
	if err := os.WriteFile(real, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}

	canonReal, err := canonicalizePath(real)
	if err != nil {
		t.Fatalf("canonicalize real: %v", err)
	}

	// Relative path that points to the same file should canonicalize the same.
	cwd, _ := os.Getwd()
	defer os.Chdir(cwd)
	if err := os.Chdir(tmp); err != nil {
		t.Fatal(err)
	}
	canonRel, err := canonicalizePath("./secret.env")
	if err != nil {
		t.Fatalf("canonicalize rel: %v", err)
	}
	if canonRel != canonReal {
		t.Fatalf("relative %q != absolute %q", canonRel, canonReal)
	}
}

func TestCanonicalizePath_ResolvesSymlink(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlinks on windows require elevation")
	}
	tmp := t.TempDir()
	real := filepath.Join(tmp, "real.env")
	link := filepath.Join(tmp, "link.env")
	if err := os.WriteFile(real, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(real, link); err != nil {
		t.Fatal(err)
	}

	canonReal, _ := canonicalizePath(real)
	canonLink, _ := canonicalizePath(link)
	if canonReal != canonLink {
		t.Fatalf("symlink %q did not resolve to target %q", canonLink, canonReal)
	}
}

func TestCanonicalizePath_NonexistentStillAbsolute(t *testing.T) {
	tmp := t.TempDir()
	missing := filepath.Join(tmp, "not-yet.env")
	canon, err := canonicalizePath(missing)
	if err != nil {
		t.Fatalf("canonicalize missing: %v", err)
	}
	if !filepath.IsAbs(canon) {
		t.Fatalf("expected absolute path, got %q", canon)
	}
}

// TestWatcher_OnlyTriggersOnRegisteredCanary is the regression test for the
// noise bug: writes to non-canary files in a watched directory must not call
// the callback. This was the cause of the runc-process /tmp spam.
func TestWatcher_OnlyTriggersOnRegisteredCanary(t *testing.T) {
	tmp := t.TempDir()
	canary := filepath.Join(tmp, "passwords_backup.txt")
	noise := filepath.Join(tmp, "runc-process12345")
	if err := os.WriteFile(canary, []byte("fake"), 0o600); err != nil {
		t.Fatal(err)
	}

	var (
		mu        sync.Mutex
		triggered []string
	)
	cb := func(e protocol.EventRequest) {
		mu.Lock()
		defer mu.Unlock()
		triggered = append(triggered, e.Details["file"])
	}

	w, err := New(cb)
	if err != nil {
		t.Fatal(err)
	}
	defer w.Stop()
	w.WatchFiles([]string{canary})
	w.Start()

	// Create + write noise file in the same dir — must NOT trigger.
	if err := os.WriteFile(noise, []byte("garbage"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(noise); err != nil {
		t.Fatal(err)
	}

	// Touch the canary — must trigger.
	if err := os.WriteFile(canary, []byte("touched"), 0o600); err != nil {
		t.Fatal(err)
	}

	time.Sleep(300 * time.Millisecond)

	mu.Lock()
	defer mu.Unlock()
	if len(triggered) == 0 {
		t.Fatalf("canary write did not trigger callback")
	}
	for _, f := range triggered {
		if filepath.Base(f) != "passwords_backup.txt" {
			t.Fatalf("non-canary file triggered callback: %s", f)
		}
	}
}
