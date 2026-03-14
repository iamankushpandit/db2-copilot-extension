package audit

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// Logger is an append-only JSONL audit logger with daily file rotation.
// It is safe for concurrent use.
type Logger struct {
	dir     string
	mu      sync.Mutex
	file    *os.File
	day     string // YYYY-MM-DD of the currently open file
	enabled bool
}

// NewLogger creates an audit Logger that writes to dir.
// If enabled is false all Log calls are no-ops.
func NewLogger(dir string, enabled bool) (*Logger, error) {
	if !enabled {
		return &Logger{enabled: false}, nil
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, fmt.Errorf("creating audit directory %s: %w", dir, err)
	}
	l := &Logger{dir: dir, enabled: true}
	if err := l.openFile(time.Now()); err != nil {
		return nil, err
	}
	return l, nil
}

// Close flushes and closes the underlying log file.
func (l *Logger) Close() error {
	if !l.enabled {
		return nil
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.file != nil {
		return l.file.Close()
	}
	return nil
}

// Log writes a single audit event to the current daily JSONL file.
// Never logs actual query result data — only row counts and summary stats.
func (l *Logger) Log(eventType EventType, githubUser, githubUID string, details any) {
	if !l.enabled {
		return
	}

	ev := Event{
		Timestamp:  time.Now().UTC(),
		Type:       eventType,
		GitHubUser: githubUser,
		GitHubUID:  githubUID,
		Details:    details,
	}

	data, err := json.Marshal(ev)
	if err != nil {
		log.Printf("WARN audit marshal error: %v", err)
		return
	}

	l.mu.Lock()
	defer l.mu.Unlock()

	// Rotate file if day has changed.
	today := time.Now().UTC().Format("2006-01-02")
	if today != l.day {
		if closeErr := l.file.Close(); closeErr != nil {
			log.Printf("WARN audit file close error: %v", closeErr)
		}
		if openErr := l.openFileLocked(time.Now()); openErr != nil {
			log.Printf("WARN audit could not open new log file: %v", openErr)
			return
		}
	}

	if _, writeErr := fmt.Fprintf(l.file, "%s\n", data); writeErr != nil {
		log.Printf("WARN audit write error: %v", writeErr)
	}
}

// openFile opens (or creates) the log file for the given time.
// Must NOT be called while the mutex is held.
func (l *Logger) openFile(t time.Time) error {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.openFileLocked(t)
}

// openFileLocked opens (or creates) the log file for the given time.
// Caller must hold l.mu.
func (l *Logger) openFileLocked(t time.Time) error {
	day := t.UTC().Format("2006-01-02")
	path := filepath.Join(l.dir, fmt.Sprintf("audit-%s.jsonl", day))

	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return fmt.Errorf("opening audit file %s: %w", path, err)
	}

	l.file = f
	l.day = day
	return nil
}
