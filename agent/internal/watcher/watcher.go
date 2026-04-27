package watcher

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"

	"github.com/fsnotify/fsnotify"
	"github.com/kitay-sudo/goronin/agent/pkg/protocol"
)

// EventCallback is called when a watched file is accessed
type EventCallback func(event protocol.EventRequest)

// Watcher monitors files for access
type Watcher struct {
	fsWatcher *fsnotify.Watcher
	callback  EventCallback
	watched   map[string]bool
}

func New(callback EventCallback) (*Watcher, error) {
	fw, err := fsnotify.NewWatcher()
	if err != nil {
		return nil, fmt.Errorf("create watcher: %w", err)
	}

	return &Watcher{
		fsWatcher: fw,
		callback:  callback,
		watched:   make(map[string]bool),
	}, nil
}

// WatchFiles adds the given paths to the watch list
func (w *Watcher) WatchFiles(paths []string) {
	for _, p := range paths {
		if _, err := os.Stat(p); os.IsNotExist(err) {
			log.Printf("[watcher] Skipping non-existent: %s", p)
			continue
		}

		dir := filepath.Dir(p)
		if !w.watched[dir] {
			if err := w.fsWatcher.Add(dir); err != nil {
				log.Printf("[watcher] Failed to watch %s: %v", dir, err)
				continue
			}
			w.watched[dir] = true
		}
		log.Printf("[watcher] Watching: %s", p)
	}
}

// CreateCanaries creates decoy files in common directories
func (w *Watcher) CreateCanaries(dirs []string) []string {
	canaryNames := []string{
		"passwords_backup.txt",
		"database_credentials.txt",
		"admin_keys.json",
		".aws_credentials",
		"id_rsa_backup",
	}

	var created []string
	for _, dir := range dirs {
		if _, err := os.Stat(dir); os.IsNotExist(err) {
			continue
		}

		for _, name := range canaryNames {
			path := filepath.Join(dir, name)
			if _, err := os.Stat(path); err == nil {
				continue // already exists
			}

			content := generateFakeContent(name)
			if err := os.WriteFile(path, []byte(content), 0644); err != nil {
				log.Printf("[watcher] Failed to create canary %s: %v", path, err)
				continue
			}

			created = append(created, path)
			log.Printf("[watcher] Canary created: %s", path)
		}
	}

	return created
}

// AutoDiscover finds sensitive files on the system
func AutoDiscover() []string {
	patterns := []string{
		"/root/.env",
		"/home/*/.env",
		"/home/*/.ssh/id_rsa",
		"/var/www/*/.env",
		"/etc/shadow",
		"/root/.ssh/id_rsa",
	}

	var found []string
	for _, pattern := range patterns {
		matches, err := filepath.Glob(pattern)
		if err != nil {
			continue
		}
		found = append(found, matches...)
	}

	return found
}

// Start begins watching for file events
func (w *Watcher) Start() {
	go func() {
		for {
			select {
			case event, ok := <-w.fsWatcher.Events:
				if !ok {
					return
				}
				if event.Has(fsnotify.Write) || event.Has(fsnotify.Remove) || event.Has(fsnotify.Chmod) {
					log.Printf("[watcher] File event: %s %s", event.Op, event.Name)
					w.callback(protocol.EventRequest{
						Type:     protocol.EventFileCanary,
						SourceIP: "localhost",
						Details: map[string]string{
							"file":      event.Name,
							"operation": event.Op.String(),
						},
					})
				}
			case err, ok := <-w.fsWatcher.Errors:
				if !ok {
					return
				}
				log.Printf("[watcher] Error: %v", err)
			}
		}
	}()
}

func (w *Watcher) Stop() error {
	return w.fsWatcher.Close()
}

func generateFakeContent(name string) string {
	token := make([]byte, 16)
	rand.Read(token)
	hexToken := hex.EncodeToString(token)

	if strings.Contains(name, "password") {
		return fmt.Sprintf("# Internal credentials - DO NOT SHARE\nadmin_password=%s\ndb_password=%s\n", hexToken[:16], hexToken[16:])
	}
	if strings.Contains(name, "credential") {
		return fmt.Sprintf("DB_HOST=db-internal.prod\nDB_USER=admin\nDB_PASS=%s\n", hexToken)
	}
	if strings.Contains(name, "keys") {
		return fmt.Sprintf(`{"api_key": "%s", "secret": "%s"}`, hexToken[:16], hexToken[16:])
	}
	if strings.Contains(name, "aws") {
		return fmt.Sprintf("[default]\naws_access_key_id = AKIA%s\naws_secret_access_key = %s\n", hexToken[:16], hexToken)
	}
	return fmt.Sprintf("-----BEGIN RSA PRIVATE KEY-----\n%s\n-----END RSA PRIVATE KEY-----\n", hexToken)
}
