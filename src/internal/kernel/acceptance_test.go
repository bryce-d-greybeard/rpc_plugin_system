package kernel

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"rpc_plugin_system/internal/auth"
)

func TestV010Acceptance(t *testing.T) {
	token, err := auth.NewToken()
	if err != nil {
		t.Fatalf("generate auth token: %v", err)
	}
	pluginBin := buildPlugin(t)
	runtimeDir := t.TempDir()
	logPath := filepath.Join(runtimeDir, "events.jsonl")

	manager, err := New(Config{
		RuntimeDir:     runtimeDir,
		PluginPath:     pluginBin,
		PluginID:       "echo",
		DialTimeout:    2 * time.Second,
		CallTimeout:    150 * time.Millisecond,
		HeartbeatEvery: 100 * time.Millisecond,
		EventLogPath:   logPath,
	})
	if err != nil {
		t.Fatalf("new manager: %v", err)
	}
	defer manager.Close()

	os.Setenv("RPC_PLUGIN_SYSTEM_AUTH_TOKEN_FILE", auth.Encode(token))
	defer os.Unsetenv("RPC_PLUGIN_SYSTEM_AUTH_TOKEN_FILE")

	if err := manager.Start(); err != nil {
		t.Fatalf("start manager: %v", err)
	}
	if _, err := manager.Heartbeat(); err != nil {
		t.Fatalf("heartbeat: %v", err)
	}
	msg, err := manager.Echo("acceptance")
	if err != nil || msg != "acceptance" {
		t.Fatalf("echo failed: msg=%q err=%v", msg, err)
	}
	if err := manager.Sleep(300 * time.Millisecond); err == nil {
		t.Fatal("expected timeout from sleep")
	}

	ctx, cancel := context.WithCancel(context.Background())
	go manager.MonitorLoop(ctx)
	if err := manager.Kill(); err != nil {
		cancel()
		t.Fatalf("kill: %v", err)
	}

	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		state := manager.State()
		if state.PID != 0 && state.Healthy {
			cancel()
			return
		}
		time.Sleep(100 * time.Millisecond)
	}
	cancel()
	t.Fatal("acceptance restart did not recover plugin")
}
