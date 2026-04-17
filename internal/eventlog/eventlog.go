// Package eventlog provides append-only JSONL event logging.
package eventlog

import (
	"encoding/json"
	"fmt"
	"os"
	"sync"
	"time"
)

// Logger appends structured events to one JSONL file.
type Logger struct {
	mu   sync.Mutex
	file *os.File
}

// Event describes one recorded lifecycle event.
type Event struct {
	Time    time.Time `json:"time"`
	Type    string    `json:"type"`
	Plugin  string    `json:"plugin,omitempty"`
	Message string    `json:"message,omitempty"`
}

// New opens or creates an append-only event log file.
func New(path string) (*Logger, error) {
	f, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		return nil, fmt.Errorf("open event log: %w", err)
	}
	return &Logger{file: f}, nil
}

// Close closes the underlying log file.
func (l *Logger) Close() error {
	return l.file.Close()
}

// Write appends one structured event to the JSONL log.
func (l *Logger) Write(event Event) error {
	l.mu.Lock()
	defer l.mu.Unlock()
	if event.Time.IsZero() {
		event.Time = time.Now().UTC()
	}
	data, err := json.Marshal(event)
	if err != nil {
		return fmt.Errorf("marshal event: %w", err)
	}
	if _, err := l.file.Write(append(data, '\n')); err != nil {
		return fmt.Errorf("append event: %w", err)
	}
	return nil
}
