package plugin

import (
	"fmt"
	"os"
	"path/filepath"

	"rpc_plugin_system/control-plane-ops/event-log-write/eventlog"
)

const (
	DefaultLogFileName       = "events.jsonl"
	DefaultPluginLogFileName = "plugin-events.jsonl"
)

// Logger is the shared plugin logging interface used by SDK-backed plugins.
type Logger struct {
	pluginID     string
	generationID uint64
	pid          int
	path         string
	logger       *eventlog.Logger
}

// LogEvent is the shared event shape for plugin-side logging.
type LogEvent = eventlog.Event

const (
	LogLevelDebug = eventlog.LevelDebug
	LogLevelInfo  = eventlog.LevelInfo
	LogLevelWarn  = eventlog.LevelWarn
	LogLevelError = eventlog.LevelError
)

const (
	LogComponentPlugin = "plugin"
)

const (
	EventPluginBootStarted      = "plugin_boot_started"
	EventPluginBootFailed       = "plugin_boot_failed"
	EventPluginConfigLoaded     = "plugin_config_loaded"
	EventPluginListenerStarted  = "plugin_listener_started"
	EventPluginServeStopped     = "plugin_serve_stopped"
	EventPluginAuthAttempt      = "plugin_auth_attempt"
	EventPluginAuthAccepted     = "plugin_auth_accepted"
	EventPluginAuthRejected     = "plugin_auth_rejected"
	EventPluginCapabilitiesRead = "plugin_capabilities_reported"
	EventPluginShutdownCalled   = "plugin_shutdown_called"
	EventPluginRequestHandled   = "plugin_request_handled"
	EventPluginRequestFailed    = "plugin_request_failed"
)

// NewLogger opens the plugin-owned append-only event log in the runtime dir.
func NewLogger(cfg Config) (*Logger, error) {
	path := filepath.Join(filepath.Dir(cfg.SocketPath), DefaultPluginLogFileName)
	base, err := eventlog.New(path)
	if err != nil {
		return nil, fmt.Errorf("open plugin event log: %w", err)
	}
	return &Logger{
		pluginID:     cfg.PluginID,
		generationID: cfg.GenerationID,
		pid:          os.Getpid(),
		path:         path,
		logger:       base,
	}, nil
}

// Path returns the log file path used by the plugin logger.
func (l *Logger) Path() string {
	if l == nil {
		return ""
	}
	return l.path
}

// Close closes the underlying plugin event log.
func (l *Logger) Close() error {
	if l == nil || l.logger == nil {
		return nil
	}
	return l.logger.Close()
}

// Event writes one shared-schema event for the current plugin generation.
func (l *Logger) Event(event LogEvent) error {
	if l == nil || l.logger == nil {
		return nil
	}
	if event.Component == "" {
		event.Component = LogComponentPlugin
	}
	if event.PluginID == "" {
		event.PluginID = l.pluginID
	}
	if event.GenerationID == 0 {
		event.GenerationID = l.generationID
	}
	if event.PID == 0 {
		event.PID = l.pid
	}
	if event.Level == "" {
		event.Level = LogLevelInfo
	}
	return l.logger.Write(event)
}
