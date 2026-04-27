package traps

import (
	"bufio"
	"fmt"
	"net"
	"sync"
	"testing"
	"time"

	"github.com/kitay-sudo/goronin/agent/pkg/protocol"
)

func TestSSHTrap(t *testing.T) {
	var mu sync.Mutex
	var events []protocol.EventRequest
	callback := func(e protocol.EventRequest) {
		mu.Lock()
		events = append(events, e)
		mu.Unlock()
	}

	trap := NewSSHTrap()
	port := 19222
	if err := trap.Start(port, callback); err != nil {
		t.Fatalf("start: %v", err)
	}
	defer trap.Stop()

	// Connect to trap
	conn, err := net.DialTimeout("tcp", fmt.Sprintf("127.0.0.1:%d", port), 2*time.Second)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}

	// Read server banner
	scanner := bufio.NewScanner(conn)
	if scanner.Scan() {
		banner := scanner.Text()
		if banner == "" {
			t.Error("expected SSH banner")
		}
		t.Logf("banner: %s", banner)
	}

	// Send client banner
	fmt.Fprintf(conn, "SSH-2.0-TestClient\r\n")
	conn.Close()

	time.Sleep(100 * time.Millisecond)

	mu.Lock()
	defer mu.Unlock()
	if len(events) == 0 {
		t.Error("expected at least one event")
	} else {
		if events[0].Type != protocol.EventSSHTrap {
			t.Errorf("expected ssh_trap, got %s", events[0].Type)
		}
		if events[0].Details["client_banner"] != "SSH-2.0-TestClient" {
			t.Errorf("expected client banner, got %s", events[0].Details["client_banner"])
		}
	}
}

func TestHTTPTrap(t *testing.T) {
	var mu sync.Mutex
	var events []protocol.EventRequest
	callback := func(e protocol.EventRequest) {
		mu.Lock()
		events = append(events, e)
		mu.Unlock()
	}

	trap := NewHTTPTrap()
	port := 19280
	if err := trap.Start(port, callback); err != nil {
		t.Fatalf("start: %v", err)
	}
	defer trap.Stop()

	// Make HTTP request
	conn, err := net.DialTimeout("tcp", fmt.Sprintf("127.0.0.1:%d", port), 2*time.Second)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	fmt.Fprintf(conn, "GET /admin HTTP/1.1\r\nHost: localhost\r\nUser-Agent: TestBot\r\n\r\n")

	// Read response
	scanner := bufio.NewScanner(conn)
	if scanner.Scan() {
		t.Logf("response: %s", scanner.Text())
	}
	conn.Close()

	time.Sleep(100 * time.Millisecond)

	mu.Lock()
	defer mu.Unlock()
	if len(events) == 0 {
		t.Error("expected at least one event")
	} else {
		if events[0].Type != protocol.EventHTTPTrap {
			t.Errorf("expected http_trap, got %s", events[0].Type)
		}
	}
}

func TestFTPTrap(t *testing.T) {
	var mu sync.Mutex
	var events []protocol.EventRequest
	callback := func(e protocol.EventRequest) {
		mu.Lock()
		events = append(events, e)
		mu.Unlock()
	}

	trap := NewFTPTrap()
	port := 19221
	if err := trap.Start(port, callback); err != nil {
		t.Fatalf("start: %v", err)
	}
	defer trap.Stop()

	conn, err := net.DialTimeout("tcp", fmt.Sprintf("127.0.0.1:%d", port), 2*time.Second)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}

	scanner := bufio.NewScanner(conn)
	// Read welcome
	scanner.Scan()
	t.Logf("welcome: %s", scanner.Text())

	// Send USER
	fmt.Fprintf(conn, "USER admin\r\n")
	scanner.Scan() // 331

	// Send PASS
	fmt.Fprintf(conn, "PASS secret123\r\n")
	scanner.Scan() // 530
	conn.Close()

	time.Sleep(100 * time.Millisecond)

	mu.Lock()
	defer mu.Unlock()
	if len(events) == 0 {
		t.Error("expected at least one event")
	} else {
		if events[0].Details["username"] != "admin" {
			t.Errorf("expected username admin, got %s", events[0].Details["username"])
		}
	}
}

func TestDBTrap(t *testing.T) {
	var mu sync.Mutex
	var events []protocol.EventRequest
	callback := func(e protocol.EventRequest) {
		mu.Lock()
		events = append(events, e)
		mu.Unlock()
	}

	trap := NewDBTrap()
	port := 19306
	if err := trap.Start(port, callback); err != nil {
		t.Fatalf("start: %v", err)
	}
	defer trap.Stop()

	conn, err := net.DialTimeout("tcp", fmt.Sprintf("127.0.0.1:%d", port), 2*time.Second)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}

	// Read greeting
	buf := make([]byte, 256)
	n, _ := conn.Read(buf)
	t.Logf("greeting: %d bytes", n)

	// Send some data
	conn.Write([]byte{0x00, 0x01, 0x02})
	conn.Close()

	time.Sleep(100 * time.Millisecond)

	mu.Lock()
	defer mu.Unlock()
	if len(events) == 0 {
		t.Error("expected at least one event")
	} else {
		if events[0].Type != protocol.EventDBTrap {
			t.Errorf("expected db_trap, got %s", events[0].Type)
		}
	}
}

func TestManager(t *testing.T) {
	var mu sync.Mutex
	eventCount := 0
	callback := func(e protocol.EventRequest) {
		mu.Lock()
		eventCount++
		mu.Unlock()
	}

	manager := NewManager(callback)
	if err := manager.StartTraps(true, true, false, false); err != nil {
		t.Fatalf("start traps: %v", err)
	}

	// Should have 2 traps started
	if len(manager.traps) != 2 {
		t.Errorf("expected 2 traps, got %d", len(manager.traps))
	}

	manager.StopAll()

	if len(manager.traps) != 0 {
		t.Error("expected 0 traps after stop")
	}
}
