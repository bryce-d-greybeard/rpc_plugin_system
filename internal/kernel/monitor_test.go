package kernel

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"rpc_plugin_system/internal/auth"
)

func TestMonitorLoopRestartsAfterKill(t *testing.T) {
	token, err := auth.NewToken()
	if err != nil {
		t.Fatalf("generate auth token: %v", err)
	}
	pluginBin := buildPlugin(t)
	runtimeDir := t.TempDir()
	logPath := filepath.Join(runtimeDir, "events.jsonl")

	manager, err := New(Config{
		RuntimeDir:       runtimeDir,
		PluginPath:       pluginBin,
		PluginID:         "echo",
		DialTimeout:      2 * time.Second,
		CallTimeout:      150 * time.Millisecond,
		HeartbeatEvery:   100 * time.Millisecond,
		EventLogPath:     logPath,
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
	firstPID := manager.State().PID

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go manager.MonitorLoop(ctx)

	if err := manager.Kill(); err != nil {
		t.Fatalf("kill manager: %v", err)
	}

	waitForHealthyRestart(t, manager, firstPID)
}

func TestMonitorLoopRestartsAfterPluginCrash(t *testing.T) {
	token, err := auth.NewToken()
	if err != nil {
		t.Fatalf("generate auth token: %v", err)
	}
	pluginBin := buildPlugin(t)
	runtimeDir := t.TempDir()
	logPath := filepath.Join(runtimeDir, "events.jsonl")

	manager, err := New(Config{
		RuntimeDir:       runtimeDir,
		PluginPath:       pluginBin,
		PluginID:         "echo",
		DialTimeout:      2 * time.Second,
		CallTimeout:      150 * time.Millisecond,
		HeartbeatEvery:   100 * time.Millisecond,
		EventLogPath:     logPath,
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
	firstPID := manager.State().PID
	firstGeneration := manager.State().GenerationID

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go manager.MonitorLoop(ctx)

	if err := manager.Crash(1); err == nil {
		t.Fatal("expected crash call to fail after plugin exits")
	}

	waitForHealthyRestart(t, manager, firstPID)
	if got := manager.State().GenerationID; got <= firstGeneration {
		t.Fatalf("generation did not advance after crash restart: got %d want > %d", got, firstGeneration)
	}
}

func TestMonitorLoopSurvivesRepeatedKillRestarts(t *testing.T) {
	token, err := auth.NewToken()
	if err != nil {
		t.Fatalf("generate auth token: %v", err)
	}
	pluginBin := buildPlugin(t)
	runtimeDir := t.TempDir()
	logPath := filepath.Join(runtimeDir, "events.jsonl")

	manager, err := New(Config{
		RuntimeDir:       runtimeDir,
		PluginPath:       pluginBin,
		PluginID:         "echo",
		DialTimeout:      2 * time.Second,
		CallTimeout:      150 * time.Millisecond,
		HeartbeatEvery:   100 * time.Millisecond,
		EventLogPath:     logPath,
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

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go manager.MonitorLoop(ctx)

	lastState := manager.State()
	for i := 0; i < 5; i++ {
		if err := manager.Kill(); err != nil {
			t.Fatalf("kill cycle %d: %v", i+1, err)
		}

		state := waitForHealthyRestart(t, manager, lastState.PID)
		if state.GenerationID <= lastState.GenerationID {
			t.Fatalf("generation did not advance on cycle %d: got %d want > %d", i+1, state.GenerationID, lastState.GenerationID)
		}
		msg := "cycle"
		if got, err := manager.Echo(msg); err != nil {
			t.Fatalf("echo after restart cycle %d: %v", i+1, err)
		} else if got != msg {
			t.Fatalf("echo mismatch after restart cycle %d: got %q want %q", i+1, got, msg)
		}
		lastState = state
	}
}

func waitForHealthyRestart(t *testing.T, manager *Manager, oldPID int) State {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if state := manager.State(); state.PID != 0 && state.PID != oldPID && state.Healthy {
			return state
		}
		time.Sleep(100 * time.Millisecond)
	}
	t.Fatal("monitor loop did not restart plugin")
	return State{}
}
