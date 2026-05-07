package kernel

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"rpc_plugin_system/internal/eventlog"
	"rpc_plugin_system/internal/runtime"
	"rpc_plugin_system/test/testpluginapi"
)

func TestMixedFailureStormMaintainsIsolationAndCleansArtifacts(t *testing.T) {
	echoPlugin := buildPlugin(t)
	failurePlugin := buildFailurePlugin(t)
	runtimeDir := t.TempDir()

	host, err := NewHost(HostConfig{
		RuntimeDir:     runtimeDir,
		DialTimeout:    2 * time.Second,
		CallTimeout:    120 * time.Millisecond,
		HeartbeatEvery: 50 * time.Millisecond,
		Plugins: []PluginConfig{
			{PluginID: "steady", PluginPath: echoPlugin},
			{PluginID: "chaos", PluginPath: failurePlugin},
		},
	})
	if err != nil {
		t.Fatalf("new host: %v", err)
	}
	defer host.Close()

	if err := host.StartAll(); err != nil {
		t.Fatalf("start all: %v", err)
	}

	chaos, err := host.Manager("chaos")
	if err != nil {
		t.Fatalf("manager chaos: %v", err)
	}
	steady, err := host.Manager("steady")
	if err != nil {
		t.Fatalf("manager steady: %v", err)
	}

	behaviorPath := writeBehaviorConfig(t, runtimeDir, testpluginapi.Env{CloseOnEcho: true})
	oldEnv := setEnvMap(t, map[string]string{testpluginapi.BehaviorConfigEnv: behaviorPath})
	if err := chaos.Restart(); err != nil {
		t.Fatalf("restart chaos into transport-break mode: %v", err)
	}
	if _, err := chaos.Echo("break-on-echo"); err == nil {
		t.Fatal("expected routed echo to break chaos transport")
	}
	if chaos.State().Healthy {
		t.Fatal("chaos should be unhealthy after transport break")
	}
	restoreEnvMap(oldEnv)
	if err := waitForHealthyManager(chaos, 2*time.Second); err != nil {
		t.Fatalf("recover chaos after transport break: %v", err)
	}

	behaviorPath = writeBehaviorConfig(t, runtimeDir, testpluginapi.Env{HeartbeatErrors: 2})
	oldEnv = setEnvMap(t, map[string]string{testpluginapi.BehaviorConfigEnv: behaviorPath})
	ctx, cancel := context.WithTimeout(context.Background(), 250*time.Millisecond)
	go chaos.MonitorLoop(ctx)
	<-ctx.Done()
	cancel()
	restoreEnvMap(oldEnv)
	if err := waitForHealthyManager(chaos, 2*time.Second); err != nil {
		t.Fatalf("recover chaos after heartbeat churn: %v", err)
	}

	if err := chaos.Sleep(300 * time.Millisecond); err == nil {
		t.Fatal("expected chaos sleep timeout after recovery")
	}
	if chaos.State().Healthy {
		t.Fatal("chaos should be unhealthy after timeout poison")
	}
	if err := waitForHealthyManager(chaos, 2*time.Second); err != nil {
		t.Fatalf("recover chaos after timeout: %v", err)
	}

	if _, err := host.Heartbeat("steady"); err != nil {
		t.Fatalf("steady heartbeat during chaos failures: %v", err)
	}
	msg, err := host.Echo("steady", "still-healthy")
	if err != nil {
		t.Fatalf("steady echo during chaos failures: %v", err)
	}
	if msg != "still-healthy" {
		t.Fatalf("steady echo message = %q want %q", msg, "still-healthy")
	}

	finalChaos := chaos.State()
	finalSteady := steady.State()
	if !finalSteady.Healthy {
		t.Fatalf("steady should remain healthy: %+v", finalSteady)
	}
	if !finalChaos.Healthy {
		t.Fatalf("chaos should be healthy after final restart: %+v", finalChaos)
	}
	if finalChaos.GenerationID < 4 {
		t.Fatalf("chaos generation = %d, want at least 4", finalChaos.GenerationID)
	}

	routes := host.Routes()
	if len(routes) != 2 {
		t.Fatalf("route count = %d, want 2", len(routes))
	}
	for _, route := range routes {
		switch route.PluginID {
		case "steady":
			if !route.Healthy {
				t.Fatalf("steady route should remain healthy: %+v", route)
			}
		case "chaos":
			if !route.Healthy {
				t.Fatalf("chaos route should recover by final state: %+v", route)
			}
		}
	}

	if err := chaos.Close(); err != nil {
		t.Fatalf("close chaos manager: %v", err)
	}
	assertNoRuntimeArtifacts(t, filepath.Join(runtimeDir, "chaos"), "chaos")

	chaosEvents := readEvents(t, filepath.Join(runtimeDir, "chaos", "events.jsonl"))
	if got := countEvent(chaosEvents, eventlog.EventRestartSucceeded, nil); got < 3 {
		t.Fatalf("chaos restart_succeeded count = %d, want at least 3", got)
	}
	if got := countEvent(chaosEvents, eventlog.EventRPCPoisoned, nil); got < 2 {
		t.Fatalf("chaos rpc_poisoned count = %d, want at least 2", got)
	}
	if got := countEvent(chaosEvents, eventlog.EventRuntimeCleanup, nil); got < 3 {
		t.Fatalf("chaos runtime_cleanup count = %d, want at least 3", got)
	}
}

func TestRestartStormLeavesNoStaleArtifactsBetweenGenerations(t *testing.T) {
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
	assertSocketPresent(t, runtimeDir, "echo")
	assertAuthAbsent(t, runtimeDir, "echo")

	for i := 0; i < 5; i++ {
		if err := manager.Restart(); err != nil {
			t.Fatalf("restart %d: %v", i+1, err)
		}
		assertSocketPresent(t, runtimeDir, "echo")
		assertAuthAbsent(t, runtimeDir, "echo")
	}

	if err := manager.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}
	assertNoRuntimeArtifacts(t, runtimeDir, "echo")

	events := readEvents(t, logPath)
	if got := countEvent(events, eventlog.EventRuntimeCleanup, nil); got < 6 {
		t.Fatalf("runtime_cleanup count = %d, want at least 6", got)
	}
}

func waitForHealthyManager(manager *Manager, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		state := manager.State()
		if state.Healthy && state.PID != 0 {
			if _, err := manager.Heartbeat(); err == nil {
				return nil
			}
		}
		if err := manager.Restart(); err == nil {
			state = manager.State()
			if state.Healthy && state.PID != 0 {
				if _, err := manager.Heartbeat(); err == nil {
					return nil
				}
			}
		}
		time.Sleep(100 * time.Millisecond)
	}
	return context.DeadlineExceeded
}

func assertSocketPresent(t *testing.T, runtimeDir, pluginID string) {
	t.Helper()
	path := runtime.SocketPath(runtimeDir, pluginID)
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("expected socket present at %s: %v", path, err)
	}
}

func assertAuthAbsent(t *testing.T, runtimeDir, pluginID string) {
	t.Helper()
	path := runtime.AuthPath(runtimeDir, pluginID)
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("expected auth token file absent at %s: err=%v", path, err)
	}
}

func assertNoRuntimeArtifacts(t *testing.T, runtimeDir, pluginID string) {
	t.Helper()
	for _, path := range []string{
		runtime.SocketPath(runtimeDir, pluginID),
		runtime.AuthPath(runtimeDir, pluginID),
	} {
		if _, err := os.Stat(path); !os.IsNotExist(err) {
			t.Fatalf("runtime artifact still present: %s err=%v", path, err)
		}
	}
}

func TestMixedFailureStormSteadyPluginStateStaysHealthy(t *testing.T) {
	echoPlugin := buildPlugin(t)
	failurePlugin := buildFailurePlugin(t)
	runtimeDir := t.TempDir()
	behaviorPath := writeBehaviorConfig(t, runtimeDir, testpluginapi.Env{CloseOnEcho: true})
	oldEnv := setEnvMap(t, map[string]string{testpluginapi.BehaviorConfigEnv: behaviorPath})
	defer restoreEnvMap(oldEnv)

	host, err := NewHost(HostConfig{
		RuntimeDir:     runtimeDir,
		DialTimeout:    2 * time.Second,
		CallTimeout:    150 * time.Millisecond,
		HeartbeatEvery: 100 * time.Millisecond,
		Plugins: []PluginConfig{
			{PluginID: "steady", PluginPath: echoPlugin},
			{PluginID: "chaos", PluginPath: failurePlugin},
		},
	})
	if err != nil {
		t.Fatalf("new host: %v", err)
	}
	defer host.Close()

	if err := host.StartAll(); err != nil {
		t.Fatalf("start all: %v", err)
	}

	if _, err := host.Echo("chaos", "break"); err == nil {
		t.Fatal("expected chaos echo failure")
	}
	steadyState := host.State()
	for _, plugin := range steadyState.Plugins {
		if plugin.PluginID == "steady" {
			if !plugin.Healthy || plugin.GenerationID == 0 || plugin.PID == 0 {
				t.Fatalf("steady plugin state should stay coherent: %+v", plugin)
			}
		}
	}

	hb, err := host.Heartbeat("steady")
	if err != nil {
		t.Fatalf("steady heartbeat: %v", err)
	}
	if hb.PluginID != "steady" || hb.Status != testpluginapi.StatusHealthy {
		t.Fatalf("unexpected steady heartbeat: %+v", hb)
	}
}
