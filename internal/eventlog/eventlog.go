// Package eventlog provides append-only JSONL event logging.
package eventlog

import (
	"encoding/json"
	"fmt"
	"os"
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
)

// Logger appends structured events to one JSONL file.
type Logger struct {
	mu   sync.Mutex
	file *os.File
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
	if _, err := l.file.Write(append(data, '\n')); err != nil {
		return fmt.Errorf("append event: %w", err)
	}
	if err := l.file.Sync(); err != nil {
		return fmt.Errorf("sync event log: %w", err)
	}
	return nil
}
