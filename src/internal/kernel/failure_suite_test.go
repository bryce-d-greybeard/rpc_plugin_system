package kernel

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"rpc_plugin_system/internal/auth"
	"rpc_plugin_system/test/testpluginapi"
	"rpc_plugin_system/test/testroot"
)

var (
	failurePluginBuildPath string
	failurePluginBuildErr  error
)

func buildFailurePlugin(t *testing.T) string {
	t.Helper()
	pluginBuildOnce.Do(func() {})
	if failurePluginBuildPath != "" || failurePluginBuildErr != nil {
		if failurePluginBuildErr != nil {
			t.Fatal(failurePluginBuildErr)
		}
		return failurePluginBuildPath
	}
	cacheDir, err := os.MkdirTemp("", "rpc_plugin_system-kernel-failure-test-")
	if err != nil {
		failurePluginBuildErr = fmt.Errorf("make failure plugin cache dir: %w", err)
		t.Fatal(failurePluginBuildErr)
	}
	failurePluginBuildPath = filepath.Join(cacheDir, "rpcplugin-failure")
	cmd := exec.Command("go", "build", "-buildvcs=false", "-o", failurePluginBuildPath, "./examples/rpcplugin-failure")
	cmd.Dir = testroot.SourceRoot(t)
	out, err := cmd.CombinedOutput()
	if err != nil {
		failurePluginBuildErr = fmt.Errorf("build failure plugin: %w\n%s", err, string(out))
		t.Fatal(failurePluginBuildErr)
	}
	return failurePluginBuildPath
}

func TestFailurePluginSuite(t *testing.T) {
	tests := []struct {
		name     string
		plugin   string
		pluginID string
		behavior testpluginapi.Env
		run      func(t *testing.T, m *Manager)
	}{
		{name: "happy-path", plugin: buildPlugin(t), pluginID: "echo", run: expectHealthyEcho},
		{name: "auth-failure", plugin: buildFailurePlugin(t), pluginID: "failure", behavior: testpluginapi.Env{FailAuth: true}, run: expectStartFailure},
		{name: "identity-mismatch", plugin: buildFailurePlugin(t), pluginID: "failure", behavior: testpluginapi.Env{PluginID: "not-failure"}, run: expectStartFailure},
		{name: "generation-mismatch", plugin: buildFailurePlugin(t), pluginID: "failure", behavior: testpluginapi.Env{GenerationOffset: 1}, run: expectStartFailure},
		{name: "capabilities-error", plugin: buildFailurePlugin(t), pluginID: "failure", behavior: testpluginapi.Env{CapabilitiesMode: "error"}, run: expectStartFailure},
		{name: "capabilities-empty", plugin: buildFailurePlugin(t), pluginID: "failure", behavior: testpluginapi.Env{CapabilitiesMode: "empty"}, run: expectHealthyStart},
		{name: "heartbeat-unhealthy", plugin: buildFailurePlugin(t), pluginID: "failure", behavior: testpluginapi.Env{HeartbeatStatus: "unhealthy"}, run: expectUnhealthyHeartbeat},
		{name: "heartbeat-error", plugin: buildFailurePlugin(t), pluginID: "failure", behavior: testpluginapi.Env{HeartbeatErrors: 1}, run: expectHeartbeatError},
		{name: "timeout", plugin: buildFailurePlugin(t), pluginID: "failure", behavior: testpluginapi.Env{SleepScale: 2}, run: expectSleepTimeout},
		{name: "mid-call-crash", plugin: buildFailurePlugin(t), pluginID: "failure", behavior: testpluginapi.Env{CrashOnEcho: true}, run: expectEchoFailure},
		{name: "shutdown-delay", plugin: buildFailurePlugin(t), pluginID: "failure", behavior: testpluginapi.Env{ShutdownDelayMS: 500}, run: expectKillWorks},
		{name: "startup-failure-close-on-accept", plugin: buildFailurePlugin(t), pluginID: "failure", behavior: testpluginapi.Env{CloseOnAccept: true}, run: expectStartFailure},
		{name: "transport-close-on-echo", plugin: buildFailurePlugin(t), pluginID: "failure", behavior: testpluginapi.Env{CloseOnEcho: true}, run: expectEchoSuccess},
		{name: "restart-churn", plugin: buildFailurePlugin(t), pluginID: "failure", run: expectRestartChurn},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			token, err := auth.NewToken()
			if err != nil {
				t.Fatalf("generate auth token: %v", err)
			}
			runtimeDir := t.TempDir()
			logPath := filepath.Join(runtimeDir, "events.jsonl")
			manager, err := New(Config{
				RuntimeDir:     runtimeDir,
				PluginPath:     tc.plugin,
				PluginID:       tc.pluginID,
				DialTimeout:    2 * time.Second,
				CallTimeout:    150 * time.Millisecond,
				HeartbeatEvery: 100 * time.Millisecond,
				EventLogPath:   logPath,
			})
			if err != nil {
				t.Fatalf("new manager: %v", err)
			}
			defer manager.Close()

			behaviorPath := writeBehaviorConfig(t, runtimeDir, tc.behavior)
			oldEnv := setEnvMap(t, map[string]string{testpluginapi.BehaviorConfigEnv: behaviorPath})
			os.Setenv("RPC_PLUGIN_SYSTEM_AUTH_TOKEN_FILE", auth.Encode(token))
			defer restoreEnvMap(oldEnv)
			defer os.Unsetenv("RPC_PLUGIN_SYSTEM_AUTH_TOKEN_FILE")

			tc.run(t, manager)
		})
	}
}

func setEnvMap(t *testing.T, env map[string]string) map[string]*string {
	t.Helper()
	old := map[string]*string{}
	for key, value := range env {
		if prior, ok := os.LookupEnv(key); ok {
			copy := prior
			old[key] = &copy
		} else {
			old[key] = nil
		}
		if err := os.Setenv(key, value); err != nil {
			t.Fatalf("setenv %s: %v", key, err)
		}
	}
	return old
}

func restoreEnvMap(old map[string]*string) {
	for key, value := range old {
		if value == nil {
			_ = os.Unsetenv(key)
		} else {
			_ = os.Setenv(key, *value)
		}
	}
}

func expectHealthyEcho(t *testing.T, m *Manager) {
	if err := m.Start(); err != nil {
		t.Fatalf("start: %v", err)
	}
	if got, err := m.Echo("ok"); err != nil || got != "ok" {
		t.Fatalf("echo failed: got=%q err=%v", got, err)
	}
}

func expectStartFailure(t *testing.T, m *Manager) {
	if err := m.Start(); err == nil {
		t.Fatal("expected start failure")
	}
}

func expectHealthyStart(t *testing.T, m *Manager) {
	if err := m.Start(); err != nil {
		t.Fatalf("start: %v", err)
	}
	if !m.State().Healthy {
		t.Fatal("expected healthy state after start")
	}
}

func expectUnhealthyHeartbeat(t *testing.T, m *Manager) {
	if err := m.Start(); err != nil {
		t.Fatalf("start: %v", err)
	}
	hb, err := m.Heartbeat()
	if err != nil {
		t.Fatalf("heartbeat: %v", err)
	}
	if hb.Status != "unhealthy" {
		t.Fatalf("heartbeat status=%s want unhealthy", hb.Status)
	}
	if m.State().Healthy {
		t.Fatal("manager should mark plugin unhealthy")
	}
}

func expectHeartbeatError(t *testing.T, m *Manager) {
	if err := m.Start(); err != nil {
		t.Fatalf("start: %v", err)
	}
	if _, err := m.Heartbeat(); err == nil {
		t.Fatal("expected heartbeat error")
	}
}

func expectSleepTimeout(t *testing.T, m *Manager) {
	if err := m.Start(); err != nil {
		t.Fatalf("start: %v", err)
	}
	if err := m.Sleep(100 * time.Millisecond); err == nil {
		t.Fatal("expected sleep timeout")
	}
}

func expectEchoFailure(t *testing.T, m *Manager) {
	if err := m.Start(); err != nil {
		t.Fatalf("start: %v", err)
	}
	if _, err := m.Echo("boom"); err == nil {
		t.Fatal("expected echo failure")
	}
}

func expectKillWorks(t *testing.T, m *Manager) {
	if err := m.Start(); err != nil {
		t.Fatalf("start: %v", err)
	}
	if err := m.Kill(); err != nil {
		t.Fatalf("kill: %v", err)
	}
}

func expectEchoSuccess(t *testing.T, m *Manager) {
	if err := m.Start(); err != nil {
		t.Fatalf("start: %v", err)
	}
	if _, err := m.Echo("close"); err == nil {
		t.Fatal("expected transport break on echo")
	}
	if m.State().Healthy {
		t.Fatal("manager should mark plugin unhealthy after transport break")
	}
}

func expectRestartChurn(t *testing.T, m *Manager) {
	if err := m.Start(); err != nil {
		t.Fatalf("start: %v", err)
	}
	last := m.State()
	for i := 0; i < 3; i++ {
		if err := m.Restart(); err != nil {
			t.Fatalf("restart %d: %v", i+1, err)
		}
		state := m.State()
		if !state.Healthy || state.GenerationID <= last.GenerationID {
			t.Fatalf("restart churn state invalid: prev=%+v next=%+v", last, state)
		}
		last = state
	}
}
