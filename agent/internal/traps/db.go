package traps

import (
	"fmt"
	"log"
	"net"

	"github.com/kitay-sudo/goronin/agent/pkg/protocol"
)

type DBTrap struct {
	listener net.Listener
	port     int
}

func NewDBTrap() *DBTrap {
	return &DBTrap{}
}

func (t *DBTrap) Type() string { return protocol.EventDBTrap }
func (t *DBTrap) Port() int    { return t.port }

func (t *DBTrap) Start(port int, callback EventCallback) error {
	ln, err := net.Listen("tcp", fmt.Sprintf(":%d", port))
	if err != nil {
		return err
	}
	t.listener = ln
	t.port = port

	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			go t.handleConn(conn, callback)
		}
	}()

	return nil
}

func (t *DBTrap) handleConn(conn net.Conn, callback EventCallback) {
	defer conn.Close()

	remoteAddr := conn.RemoteAddr().(*net.TCPAddr)

	// Send MySQL-like greeting
	greeting := []byte{
		0x4a, 0x00, 0x00, 0x00, // packet length + sequence
		0x0a, // protocol version 10
	}
	greeting = append(greeting, []byte("5.7.42-log")...)
	greeting = append(greeting, 0x00) // null terminator
	conn.Write(greeting)

	// Read client response (up to 1024 bytes)
	buf := make([]byte, 1024)
	n, _ := conn.Read(buf)

	log.Printf("[db_trap] Connection from %s (read %d bytes)", remoteAddr.IP, n)

	callback(protocol.EventRequest{
		Type:       protocol.EventDBTrap,
		SourceIP:   remoteAddr.IP.String(),
		SourcePort: remoteAddr.Port,
		TrapPort:   t.port,
		Details:    map[string]string{"protocol": "mysql_like"},
	})

	// Send access denied
	errMsg := fmt.Sprintf("\x15\x04#28000Access denied for user 'root'@'%s'", remoteAddr.IP)
	conn.Write([]byte(errMsg))
}

func (t *DBTrap) Stop() error {
	if t.listener != nil {
		return t.listener.Close()
	}
	return nil
}
