package traps

import (
	"bufio"
	"fmt"
	"log"
	"net"
	"strings"

	"github.com/kitay-sudo/goronin/agent/pkg/protocol"
)

type FTPTrap struct {
	listener net.Listener
	port     int
}

func NewFTPTrap() *FTPTrap {
	return &FTPTrap{}
}

func (t *FTPTrap) Type() string { return protocol.EventFTPTrap }
func (t *FTPTrap) Port() int    { return t.port }

func (t *FTPTrap) Start(port int, callback EventCallback) error {
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

func (t *FTPTrap) handleConn(conn net.Conn, callback EventCallback) {
	defer conn.Close()

	remoteAddr := conn.RemoteAddr().(*net.TCPAddr)

	// FTP welcome banner
	fmt.Fprintf(conn, "220 (vsFTPd 3.0.5)\r\n")

	scanner := bufio.NewScanner(conn)
	username := ""
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		parts := strings.SplitN(line, " ", 2)
		cmd := strings.ToUpper(parts[0])

		switch cmd {
		case "USER":
			if len(parts) > 1 {
				username = parts[1]
			}
			fmt.Fprintf(conn, "331 Please specify the password.\r\n")
		case "PASS":
			log.Printf("[ftp_trap] Login attempt from %s, user=%s", remoteAddr.IP, username)
			callback(protocol.EventRequest{
				Type:       protocol.EventFTPTrap,
				SourceIP:   remoteAddr.IP.String(),
				SourcePort: remoteAddr.Port,
				TrapPort:   t.port,
				Details:    map[string]string{"username": username},
			})
			fmt.Fprintf(conn, "530 Login incorrect.\r\n")
			return
		case "QUIT":
			fmt.Fprintf(conn, "221 Goodbye.\r\n")
			return
		default:
			fmt.Fprintf(conn, "500 Unknown command.\r\n")
		}
	}
}

func (t *FTPTrap) Stop() error {
	if t.listener != nil {
		return t.listener.Close()
	}
	return nil
}
