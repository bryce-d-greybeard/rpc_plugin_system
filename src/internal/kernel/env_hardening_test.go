package kernel

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestManagerStartDoesNotInheritDaemonEnvironment(t *testing.T) {
	pluginBin := buildPlugin(t)
	runtimeDir := t.TempDir()
	logPath := filepath.Join(runtimeDir, "events.jsonl")

	manager, err := New(Config{
		RuntimeDir:   runtimeDir,
		PluginPath:   pluginBin,
		PluginID:     "echo",
		DialTimeout:  2 * time.Second,
		CallTimeout:  200 * time.Millisecond,
		EventLogPath: logPath,
	})
	if err != nil {
		t.Fatalf("new manager: %v", err)
	}
	defer manager.Close()

	old, had := os.LookupEnv("RPC_PLUGIN_SYSTEM_PLUGIN_BEHAVIOR_ID")
	if err := os.Setenv("RPC_PLUGIN_SYSTEM_PLUGIN_BEHAVIOR_ID", "leaked-from-daemon"); err != nil {
		t.Fatalf("set env: %v", err)
	}
	defer func() {
		if had {
			_ = os.Setenv("RPC_PLUGIN_SYSTEM_PLUGIN_BEHAVIOR_ID", old)
		} else {
			_ = os.Unsetenv("RPC_PLUGIN_SYSTEM_PLUGIN_BEHAVIOR_ID")
		}
	}()

	if err := manager.Start(); err != nil {
		t.Fatalf("start manager: %v", err)
	}
	hb, err := manager.Heartbeat()
	if err != nil {
		t.Fatalf("heartbeat: %v", err)
	}
	if hb.PluginID != "echo" {
		t.Fatalf("plugin inherited daemon env, got plugin id %q want %q", hb.PluginID, "echo")
	}
}
