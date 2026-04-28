package watcher

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/fsnotify/fsnotify"
	"github.com/kitay-sudo/goronin/agent/pkg/protocol"
)

// canonicalizePath returns a stable, comparable form of p:
// expands ~ and env vars, makes it absolute, resolves symlinks if the file
// exists, and lowercases on case-insensitive filesystems.
// Used both when registering canaries and when matching fsnotify events,
// so user-supplied paths and OS-reported paths always normalize to the same key.
func canonicalizePath(p string) (string, error) {
	p = os.ExpandEnv(p)
	if strings.HasPrefix(p, "~") {
		home, err := os.UserHomeDir()
		if err == nil {
			p = filepath.Join(home, strings.TrimPrefix(p, "~"))
		}
	}

	abs, err := filepath.Abs(p)
	if err != nil {
		return "", err
	}

	// EvalSymlinks fails if the path does not exist; fall back to abs in that case
	// so we can still register canaries that will be created later.
	if resolved, err := filepath.EvalSymlinks(abs); err == nil {
		abs = resolved
	}

	if runtime.GOOS == "windows" {
		abs = strings.ToLower(abs)
	}
	return abs, nil
}

// EventCallback is called when a watched file is accessed
type EventCallback func(event protocol.EventRequest)

// Watcher monitors files for access
type Watcher struct {
	fsWatcher *fsnotify.Watcher
	callback  EventCallback
	watched   map[string]bool
	canaries  map[string]bool
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
		canaries:  make(map[string]bool),
	}, nil
}

// WatchFiles adds the given paths to the watch list
func (w *Watcher) WatchFiles(paths []string) {
	for _, p := range paths {
		if _, err := os.Stat(p); os.IsNotExist(err) {
			log.Printf("[watcher] Skipping non-existent: %s", p)
			continue
		}

		canon, err := canonicalizePath(p)
		if err != nil {
			log.Printf("[watcher] Failed to resolve %s: %v", p, err)
			continue
		}
		dir, err := canonicalizePath(filepath.Dir(canon))
		if err != nil {
			log.Printf("[watcher] Failed to resolve dir of %s: %v", canon, err)
			continue
		}
		if !w.watched[dir] {
			if err := w.fsWatcher.Add(dir); err != nil {
				log.Printf("[watcher] Failed to watch %s: %v", dir, err)
				continue
			}
			w.watched[dir] = true
		}
		w.canaries[canon] = true
		log.Printf("[watcher] Watching: %s", canon)
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

			canon, err := canonicalizePath(path)
			if err != nil {
				canon = path
			}
			created = append(created, canon)
			log.Printf("[watcher] Canary created: %s", canon)
		}
	}

	return created
}

// AutoDiscover finds sensitive files on the system.
// Excludes files mutated by system tooling (e.g. /etc/shadow on passwd/useradd)
// to avoid false positives.
func AutoDiscover() []string {
	patterns := []string{
		"/root/.env",
		"/home/*/.env",
		"/home/*/.ssh/id_rsa",
		"/var/www/*/.env",
		"/root/.ssh/id_rsa",
	}

	seen := make(map[string]bool)
	var found []string
	for _, pattern := range patterns {
		matches, err := filepath.Glob(pattern)
		if err != nil {
			continue
		}
		for _, m := range matches {
			canon, err := canonicalizePath(m)
			if err != nil {
				canon = m
			}
			if seen[canon] {
				continue
			}
			seen[canon] = true
			found = append(found, canon)
		}
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
				canon, err := canonicalizePath(event.Name)
				if err != nil {
					continue
				}
				if !w.canaries[canon] {
					continue
				}
				if event.Has(fsnotify.Write) || event.Has(fsnotify.Remove) {
					log.Printf("[watcher] Canary triggered: %s %s", event.Op, canon)
					w.callback(protocol.EventRequest{
						Type:     protocol.EventFileCanary,
						SourceIP: "localhost",
						Details: map[string]string{
							"file":      canon,
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
