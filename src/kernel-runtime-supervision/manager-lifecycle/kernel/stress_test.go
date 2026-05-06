package kernel

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"rpc_plugin_system/control-plane-ops/event-log-write/eventlog"
	"rpc_plugin_system/plugin-authoring-sdk/test-plugin-api/testpluginapi"
)

func TestRestartStormMaintainsMonotonicGenerationAndHealthyState(t *testing.T) {
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

	last := manager.State()
	for i := 0; i < 10; i++ {
		if err := manager.Restart(); err != nil {
			t.Fatalf("restart %d: %v", i+1, err)
		}
		state := manager.State()
		if !state.Healthy {
			t.Fatalf("restart %d returned unhealthy state", i+1)
		}
		if state.GenerationID <= last.GenerationID {
			t.Fatalf("restart %d generation did not advance: got %d want > %d", i+1, state.GenerationID, last.GenerationID)
		}
		if state.PID == 0 {
			t.Fatalf("restart %d returned zero pid", i+1)
		}
		last = state
	}
}

func TestTimeoutStormLeavesNoTrustedStaleClient(t *testing.T) {
	pluginBin := buildPlugin(t)
	runtimeDir := t.TempDir()
	logPath := filepath.Join(runtimeDir, "events.jsonl")

	manager, err := New(Config{
		RuntimeDir:     runtimeDir,
		PluginPath:     pluginBin,
		PluginID:       "echo",
		DialTimeout:    2 * time.Second,
		CallTimeout:    100 * time.Millisecond,
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

	for i := 0; i < 3; i++ {
		if err := manager.Sleep(300 * time.Millisecond); err == nil {
			t.Fatalf("timeout storm iteration %d expected timeout", i+1)
		}
		if manager.State().Healthy {
			t.Fatalf("timeout storm iteration %d left manager healthy", i+1)
		}
		if _, err := manager.Heartbeat(); err == nil {
			t.Fatalf("timeout storm iteration %d heartbeat should fail until restart", i+1)
		}
		if err := manager.Restart(); err != nil {
			t.Fatalf("timeout storm iteration %d restart: %v", i+1, err)
		}
	}

	events := readEvents(t, logPath)
	if got := countEvent(events, eventlog.EventRPCTimeout, nil); got < 3 {
		t.Fatalf("timeout event count = %d, want at least 3", got)
	}
	if got := countEvent(events, eventlog.EventRPCPoisoned, nil); got < 3 {
		t.Fatalf("poison event count = %d, want at least 3", got)
	}
}

func TestTransportBreakStormRecoversCleanly(t *testing.T) {
	pluginBin := buildFailurePlugin(t)
	runtimeDir := t.TempDir()
	logPath := filepath.Join(runtimeDir, "events.jsonl")
	behaviorPath := writeBehaviorConfig(t, runtimeDir, testpluginapi.Env{CloseOnEcho: true})
	oldEnv := setEnvMap(t, map[string]string{testpluginapi.BehaviorConfigEnv: behaviorPath})
	defer restoreEnvMap(oldEnv)

	manager, err := New(Config{
		RuntimeDir:     runtimeDir,
		PluginPath:     pluginBin,
		PluginID:       "failure",
		DialTimeout:    2 * time.Second,
		CallTimeout:    150 * time.Millisecond,
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

	for i := 0; i < 3; i++ {
		if _, err := manager.Echo("break"); err == nil {
			t.Fatalf("transport break iteration %d expected echo failure", i+1)
		}
		if manager.State().Healthy {
			t.Fatalf("transport break iteration %d left manager healthy", i+1)
		}
		if err := manager.Restart(); err != nil {
			t.Fatalf("transport break iteration %d restart: %v", i+1, err)
		}
	}

	events := readEvents(t, logPath)
	if got := countEvent(events, eventlog.EventRPCPoisoned, nil); got < 3 {
		t.Fatalf("transport poison count = %d, want at least 3", got)
	}
}

func TestMonitorLoopSurvivesRepeatedFailureRecovery(t *testing.T) {
	pluginBin := buildFailurePlugin(t)
	runtimeDir := t.TempDir()
	logPath := filepath.Join(runtimeDir, "events.jsonl")
	behaviorPath := writeBehaviorConfig(t, runtimeDir, testpluginapi.Env{HeartbeatErrors: 10})
	oldEnv := setEnvMap(t, map[string]string{testpluginapi.BehaviorConfigEnv: behaviorPath})
	defer restoreEnvMap(oldEnv)

	manager, err := New(Config{
		RuntimeDir:     runtimeDir,
		PluginPath:     pluginBin,
		PluginID:       "failure",
		DialTimeout:    2 * time.Second,
		CallTimeout:    150 * time.Millisecond,
		HeartbeatEvery: 50 * time.Millisecond,
		EventLogPath:   logPath,
	})
	if err != nil {
		t.Fatalf("new manager: %v", err)
	}
	defer manager.Close()

	if err := manager.Start(); err != nil {
		t.Fatalf("start: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 700*time.Millisecond)
	defer cancel()
	go manager.MonitorLoop(ctx)
	<-ctx.Done()

	events := readEvents(t, logPath)
	if got := countEvent(events, eventlog.EventRestartRequested, nil); got == 0 {
		t.Fatal("expected monitor loop to request restarts")
	}
	if got := countEvent(events, eventlog.EventRestartSucceeded, nil); got == 0 {
		t.Fatal("expected monitor loop recovery restarts to succeed")
	}
}
