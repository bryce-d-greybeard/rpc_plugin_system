// Package eventlog provides append-only JSONL event logging.
package eventlog

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
)

const (
	LevelDebug = "debug"
	LevelInfo  = "info"
	LevelWarn  = "warn"
	LevelError = "error"
)

const (
	ComponentKernel  = "kernel"
	ComponentRPC     = "rpc"
	ComponentAuth    = "auth"
	ComponentRuntime = "runtime"
)

const (
	DefaultMaxBytes   int64 = 8 << 20
	DefaultMaxBackups       = 5
)

// Event names used by the kernel lifecycle.
const (
	EventManagerInitialized   = "manager_initialized"
	EventPluginStartRequested = "plugin_start_requested"
	EventPluginStartFailed    = "plugin_start_failed"
	EventPluginStarted        = "plugin_started"
	EventPluginDialStarted    = "plugin_dial_started"
	EventPluginDialSucceeded  = "plugin_dial_succeeded"
	EventPluginDialFailed     = "plugin_dial_failed"
	EventAuthStarted          = "plugin_auth_started"
	EventAuthSucceeded        = "plugin_auth_succeeded"
	EventAuthFailed           = "plugin_auth_failed"
	EventCapabilitiesStarted  = "plugin_capabilities_started"
	EventCapabilitiesFailed   = "plugin_capabilities_failed"
	EventCapabilitiesLoaded   = "plugin_capabilities_loaded"
	EventHeartbeatHealthy     = "heartbeat_healthy"
	EventHeartbeatUnhealthy   = "heartbeat_unhealthy"
	EventHeartbeatFailed      = "heartbeat_failed"
	EventRPCStarted           = "rpc_call_started"
	EventRPCSucceeded         = "rpc_call_succeeded"
	EventRPCFailed            = "rpc_call_failed"
	EventRPCTimeout           = "rpc_call_timeout"
	EventRPCPoisoned          = "rpc_client_poisoned"
	EventRestartRequested     = "plugin_restart_requested"
	EventRestartSucceeded     = "plugin_restart_succeeded"
	EventRestartFailed        = "plugin_restart_failed"
	EventShutdownRequested    = "plugin_shutdown_requested"
	EventShutdownSucceeded    = "plugin_shutdown_succeeded"
	EventShutdownFailed       = "plugin_shutdown_failed"
	EventKillRequested        = "plugin_kill_requested"
	EventPluginKilled         = "plugin_killed"
	EventPluginStopped        = "plugin_stopped"
	EventRuntimeCleanup       = "runtime_cleanup_completed"
	EventRuntimeCleanupFailed = "runtime_cleanup_failed"
	EventLogRotated           = "log_rotated"
)

// Options configures log durability and retention.
type Options struct {
	MaxBytes   int64
	MaxBackups int
}

// Logger appends structured events to one JSONL file.
type Logger struct {
	mu         sync.Mutex
	path       string
	file       *os.File
	maxBytes   int64
	maxBackups int
}

// Event describes one recorded lifecycle event.
type Event struct {
	Time         time.Time      `json:"time"`
	Level        string         `json:"level"`
	Component    string         `json:"component"`
	Event        string         `json:"event"`
	PluginID     string         `json:"plugin_id,omitempty"`
	GenerationID uint64         `json:"generation_id,omitempty"`
	PID          int            `json:"pid,omitempty"`
	SocketPath   string         `json:"socket_path,omitempty"`
	Method       string         `json:"method,omitempty"`
	Message      string         `json:"message,omitempty"`
	Error        string         `json:"error,omitempty"`
	Reason       string         `json:"reason,omitempty"`
	Details      map[string]any `json:"details,omitempty"`
}

// New opens or creates an append-only event log file with default retention.
func New(path string) (*Logger, error) {
	return NewWithOptions(path, Options{})
}

// NewWithOptions opens or creates an append-only event log file with retention settings.
func NewWithOptions(path string, opts Options) (*Logger, error) {
	if opts.MaxBytes <= 0 {
		opts.MaxBytes = DefaultMaxBytes
	}
	if opts.MaxBackups <= 0 {
		opts.MaxBackups = DefaultMaxBackups
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		return nil, fmt.Errorf("open event log: %w", err)
	}
	return &Logger{path: path, file: f, maxBytes: opts.MaxBytes, maxBackups: opts.MaxBackups}, nil
}

// Close closes the underlying log file.
func (l *Logger) Close() error {
	return l.file.Close()
}

// Path returns the current canonical log path.
func (l *Logger) Path() string {
	if l == nil {
		return ""
	}
	return l.path
}

// Write appends one structured event to the JSONL log.
func (l *Logger) Write(event Event) error {
	l.mu.Lock()
	defer l.mu.Unlock()
	if event.Time.IsZero() {
		event.Time = time.Now().UTC()
	}
	if event.Level == "" {
		event.Level = LevelInfo
	}
	if event.Component == "" {
		event.Component = ComponentKernel
	}
	data, err := json.Marshal(event)
	if err != nil {
		return fmt.Errorf("marshal event: %w", err)
	}
	data = append(data, '\n')
	if err := l.rotateIfNeeded(int64(len(data))); err != nil {
		return err
	}
	if _, err := l.file.Write(data); err != nil {
		return fmt.Errorf("append event: %w", err)
	}
	if err := l.file.Sync(); err != nil {
		return fmt.Errorf("sync event log: %w", err)
	}
	return nil
}

func (l *Logger) rotateIfNeeded(nextWriteBytes int64) error {
	if l.maxBytes <= 0 || l.path == "" {
		return nil
	}
	info, err := l.file.Stat()
	if err != nil {
		return fmt.Errorf("stat event log: %w", err)
	}
	if info.Size()+nextWriteBytes <= l.maxBytes {
		return nil
	}
	if err := l.file.Close(); err != nil {
		return fmt.Errorf("close event log before rotate: %w", err)
	}
	for i := l.maxBackups; i >= 1; i-- {
		oldPath := backupPath(l.path, i)
		if i == l.maxBackups {
			_ = os.Remove(oldPath)
			continue
		}
		newPath := backupPath(l.path, i+1)
		if _, err := os.Stat(oldPath); err == nil {
			if err := os.Rename(oldPath, newPath); err != nil {
				return reopenWithErr(l, fmt.Errorf("rotate backup %s -> %s: %w", oldPath, newPath, err))
			}
		}
	}
	if _, err := os.Stat(l.path); err == nil {
		if err := os.Rename(l.path, backupPath(l.path, 1)); err != nil {
			return reopenWithErr(l, fmt.Errorf("rotate current log: %w", err))
		}
	}
	file, err := os.OpenFile(l.path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		return fmt.Errorf("open rotated event log: %w", err)
	}
	l.file = file
	rotationEvent := Event{
		Time:      time.Now().UTC(),
		Level:     LevelInfo,
		Component: ComponentRuntime,
		Event:     EventLogRotated,
		Message:   "event log rotated",
		Details: map[string]any{
			"path":        l.path,
			"max_bytes":   l.maxBytes,
			"max_backups": l.maxBackups,
		},
	}
	payload, err := json.Marshal(rotationEvent)
	if err != nil {
		return fmt.Errorf("marshal rotation event: %w", err)
	}
	if _, err := l.file.Write(append(payload, '\n')); err != nil {
		return fmt.Errorf("write rotation event: %w", err)
	}
	if err := l.file.Sync(); err != nil {
		return fmt.Errorf("sync rotation event: %w", err)
	}
	return nil
}

func backupPath(path string, n int) string {
	ext := filepath.Ext(path)
	base := path[:len(path)-len(ext)]
	if ext == "" {
		return fmt.Sprintf("%s.%d", path, n)
	}
	return fmt.Sprintf("%s.%d%s", base, n, ext)
}

func reopenWithErr(l *Logger, cause error) error {
	file, err := os.OpenFile(l.path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
	if err == nil {
		l.file = file
	}
	return cause
}
