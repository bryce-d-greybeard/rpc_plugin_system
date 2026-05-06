package kernel

import (
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"rpc_plugin_system/control-plane-ops/event-log-write/eventlog"
)

func TestManagerVerifiesPeerCredOnStartup(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("peercred verification is linux-only")
	}

	pluginBin := buildPlugin(t)
	runtimeDir := t.TempDir()
	logPath := filepath.Join(runtimeDir, "events.jsonl")

	manager, err := New(Config{
		RuntimeDir:     runtimeDir,
		PluginPath:     pluginBin,
		PluginID:       "echo",
		DialTimeout:    2 * time.Second,
		CallTimeout:    200 * time.Millisecond,
		HeartbeatEvery: 100 * time.Millisecond,
		EventLogPath:   logPath,
	})
	if err != nil {
		t.Fatalf("new manager: %v", err)
	}
	defer manager.Close()

	if err := manager.Start(); err != nil {
		t.Fatalf("start: %v", err)
	}

	events := readEvents(t, logPath)
	assertEvent(t, events, eventlog.EventPeerCredVerified, func(event eventlog.Event) bool {
		return event.PluginID == "echo" && event.PID > 0
	})
}
