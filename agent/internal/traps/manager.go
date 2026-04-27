package traps

import (
	"fmt"
	"log"
	"math/rand"
	"net"
	"sync"

	"github.com/kitay-sudo/goronin/agent/pkg/protocol"
)

// EventCallback is called when a trap catches activity
type EventCallback func(event protocol.EventRequest)

// Trap interface for all trap types
type Trap interface {
	Start(port int, callback EventCallback) error
	Stop() error
	Type() string
	Port() int
}

// TrapInfo describes one running trap; used for startup messages and status.
type TrapInfo struct {
	Type string
	Port int
}

// Manager manages all active traps
type Manager struct {
	traps    []Trap
	mu       sync.Mutex
	callback EventCallback
}

func NewManager(callback EventCallback) *Manager {
	return &Manager{
		callback: callback,
	}
}

func (m *Manager) StartTraps(ssh, http, ftp, db bool) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	trapConfigs := []struct {
		enabled bool
		trap    Trap
	}{
		{ssh, NewSSHTrap()},
		{http, NewHTTPTrap()},
		{ftp, NewFTPTrap()},
		{db, NewDBTrap()},
	}

	for _, tc := range trapConfigs {
		if !tc.enabled {
			continue
		}

		port := randomHighPort()
		if err := tc.trap.Start(port, m.callback); err != nil {
			log.Printf("[traps] Failed to start %s trap on port %d: %v", tc.trap.Type(), port, err)
			continue
		}

		m.traps = append(m.traps, tc.trap)
		log.Printf("[traps] %s trap listening on port %d", tc.trap.Type(), port)
	}

	return nil
}

// RunningTraps returns a snapshot of currently active traps and their ports.
// Used by the alerter for the startup message.
func (m *Manager) RunningTraps() []TrapInfo {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]TrapInfo, 0, len(m.traps))
	for _, t := range m.traps {
		out = append(out, TrapInfo{Type: t.Type(), Port: t.Port()})
	}
	return out
}

func (m *Manager) StopAll() {
	m.mu.Lock()
	defer m.mu.Unlock()

	for _, t := range m.traps {
		if err := t.Stop(); err != nil {
			log.Printf("[traps] Error stopping %s: %v", t.Type(), err)
		}
	}
	m.traps = nil
}

// randomHighPort returns a random port between 10000-60000
func randomHighPort() int {
	for i := 0; i < 100; i++ {
		port := 10000 + rand.Intn(50000)
		ln, err := net.Listen("tcp", fmt.Sprintf(":%d", port))
		if err == nil {
			ln.Close()
			return port
		}
	}
	return 10000 + rand.Intn(50000)
}
