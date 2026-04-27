package traps

import (
	"fmt"
	"log"
	"net"
	"net/http"

	"github.com/kitay-sudo/goronin/agent/pkg/protocol"
)

type HTTPTrap struct {
	listener net.Listener
	port     int
}

func NewHTTPTrap() *HTTPTrap {
	return &HTTPTrap{}
}

func (t *HTTPTrap) Type() string { return protocol.EventHTTPTrap }
func (t *HTTPTrap) Port() int    { return t.port }

func (t *HTTPTrap) Start(port int, callback EventCallback) error {
	ln, err := net.Listen("tcp", fmt.Sprintf(":%d", port))
	if err != nil {
		return err
	}
	t.listener = ln
	t.port = port

	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		remoteIP, _, _ := net.SplitHostPort(r.RemoteAddr)

		log.Printf("[http_trap] %s %s from %s", r.Method, r.URL.Path, remoteIP)

		callback(protocol.EventRequest{
			Type:     protocol.EventHTTPTrap,
			SourceIP: remoteIP,
			TrapPort: t.port,
			Details: map[string]string{
				"method":     r.Method,
				"path":       r.URL.Path,
				"user_agent": r.UserAgent(),
			},
		})

		w.Header().Set("Server", "Apache/2.4.52 (Ubuntu)")
		w.WriteHeader(http.StatusOK)
		fmt.Fprint(w, "<html><body><h1>It works!</h1></body></html>")
	})

	go http.Serve(ln, mux)

	return nil
}

func (t *HTTPTrap) Stop() error {
	if t.listener != nil {
		return t.listener.Close()
	}
	return nil
}
