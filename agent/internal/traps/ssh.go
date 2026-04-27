package traps

import (
	"bufio"
	"fmt"
	"log"
	"net"
	"strings"

	"github.com/kitay-sudo/goronin/agent/pkg/protocol"
)

type SSHTrap struct {
	listener net.Listener
	port     int
}

func NewSSHTrap() *SSHTrap {
	return &SSHTrap{}
}

func (t *SSHTrap) Type() string { return protocol.EventSSHTrap }
func (t *SSHTrap) Port() int    { return t.port }

func (t *SSHTrap) Start(port int, callback EventCallback) error {
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
				return // listener closed
			}
			go t.handleConn(conn, callback)
		}
	}()

	return nil
}

func (t *SSHTrap) handleConn(conn net.Conn, callback EventCallback) {
	defer conn.Close()

	remoteAddr := conn.RemoteAddr().(*net.TCPAddr)

	// Send fake SSH banner
	fmt.Fprintf(conn, "SSH-2.0-OpenSSH_8.9p1 Ubuntu-3ubuntu0.4\r\n")

	// Read client banner
	scanner := bufio.NewScanner(conn)
	clientBanner := ""
	if scanner.Scan() {
		clientBanner = strings.TrimSpace(scanner.Text())
	}

	log.Printf("[ssh_trap] Connection from %s, banner: %s", remoteAddr.IP, clientBanner)

	callback(protocol.EventRequest{
		Type:       protocol.EventSSHTrap,
		SourceIP:   remoteAddr.IP.String(),
		SourcePort: remoteAddr.Port,
		TrapPort:   t.port,
		Details:    map[string]string{"client_banner": clientBanner},
	})
}

func (t *SSHTrap) Stop() error {
	if t.listener != nil {
		return t.listener.Close()
	}
	return nil
}
