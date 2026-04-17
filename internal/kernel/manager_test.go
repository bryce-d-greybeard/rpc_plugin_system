package kernel

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"rpc_plugin_system/internal/eventlog"
	"rpc_plugin_system/internal/runtime"
	"rpc_plugin_system/internal/testpluginapi"
)

var (
	pluginBuildOnce sync.Once
	pluginBuildPath string
	pluginBuildErr  error
)

func buildPlugin(t *testing.T) string {
	t.Helper()
	pluginBuildOnce.Do(func() {
		cacheDir, err := os.MkdirTemp("", "rpc_plugin_system-kernel-test-")
		if err != nil {
			pluginBuildErr = fmt.Errorf("make plugin cache dir: %w", err)
			return
		}
		pluginBuildPath = filepath.Join(cacheDir, "rpcplugin-echo")
		cmd := exec.Command("go", "build", "-o", pluginBuildPath, "./cmd/rpcplugin-echo")
		cmd.Dir = filepath.Clean(filepath.Join("..", ".."))
		out, err := cmd.CombinedOutput()
		if err != nil {
			pluginBuildErr = fmt.Errorf("build plugin: %w\n%s", err, string(out))
		}
	})
	if pluginBuildErr != nil {
		t.Fatal(pluginBuildErr)
	}
	return pluginBuildPath
}

func TestManagerStartHeartbeatEchoAndRestart(t *testing.T) {
	pluginBin := buildPlugin(t)
	runtimeDir := t.TempDir()
	logPath := filepath.Join(runtimeDir, "events.jsonl")

	manager, err := New(Config{
		RuntimeDir:       runtimeDir,
		PluginPath:       pluginBin,
		PluginID:         "echo",
		DialTimeout:      2 * time.Second,
		CallTimeout:      200 * time.Millisecond,
		EventLogPath:     logPath,
	})
	if err != nil {
		t.Fatalf("new manager: %v", err)
	}
	defer manager.Close()

	if err := manager.Start(); err != nil {
		t.Fatalf("start manager: %v", err)
	}

	info, err := os.Stat(manager.State().SocketPath)
	if err != nil {
		t.Fatalf("stat plugin socket: %v", err)
	}
	if got := info.Mode().Perm(); got != 0o600 {
		t.Fatalf("plugin socket permissions = %o, want 600", got)
	}

	hb, err := manager.Heartbeat()
	if err != nil {
		t.Fatalf("heartbeat: %v", err)
	}
	if hb.PluginID != "echo" {
		t.Fatalf("heartbeat plugin id mismatch: %q", hb.PluginID)
	}

	msg, err := manager.Echo("hi")
	if err != nil {
		t.Fatalf("echo: %v", err)
	}
	if msg != "hi" {
		t.Fatalf("echo mismatch: %q", msg)
	}

	firstGen := manager.State().GenerationID
	if err := manager.Restart(); err != nil {
		t.Fatalf("restart: %v", err)
	}
	if got := manager.State().GenerationID; got != firstGen+1 {
		t.Fatalf("generation did not increment: got %d want %d", got, firstGen+1)
	}
}

func TestManagerSleepTimeout(t *testing.T) {
	pluginBin := buildPlugin(t)
	runtimeDir := t.TempDir()
	logPath := filepath.Join(runtimeDir, "events.jsonl")

	manager, err := New(Config{
		RuntimeDir:       runtimeDir,
		PluginPath:       pluginBin,
		PluginID:         "echo",
		DialTimeout:      2 * time.Second,
		CallTimeout:      100 * time.Millisecond,
		EventLogPath:     logPath,
	})
	if err != nil {
		t.Fatalf("new manager: %v", err)
	}
	defer manager.Close()

	if err := manager.Start(); err != nil {
		t.Fatalf("start manager: %v", err)
	}
	if err := manager.Sleep(300 * time.Millisecond); err == nil {
		t.Fatal("expected sleep timeout")
	}
	if manager.State().Healthy {
		t.Fatal("manager should mark plugin unhealthy after timeout poisons connection")
	}
	if _, err := manager.Heartbeat(); err == nil {
		t.Fatal("expected heartbeat to fail after timeout poisoned rpc client")
	}
}

func TestManagerRejectsUntrustedPlugin(t *testing.T) {
	pluginBin := buildFailurePlugin(t)
	runtimeDir := t.TempDir()
	logPath := filepath.Join(runtimeDir, "events.jsonl")

	manager, err := New(Config{
		RuntimeDir:       runtimeDir,
		PluginPath:       pluginBin,
		PluginID:         "failure",
		DialTimeout:      2 * time.Second,
		CallTimeout:      200 * time.Millisecond,
		EventLogPath:     logPath,
	})
	if err != nil {
		t.Fatalf("new manager: %v", err)
	}
	defer manager.Close()

	os.Setenv("RPC_PLUGIN_SYSTEM_PLUGIN_BEHAVIOR_FAIL_AUTH", "true")
	defer os.Unsetenv("RPC_PLUGIN_SYSTEM_PLUGIN_BEHAVIOR_FAIL_AUTH")

	if err := manager.Start(); err == nil {
		t.Fatal("expected auth rejection")
	}

	for _, path := range []string{
		runtime.SocketPath(runtimeDir, "failure"),
		runtime.AuthPath(runtimeDir, "failure"),
	} {
		if _, err := os.Stat(path); !os.IsNotExist(err) {
			t.Fatalf("failed start left runtime artifact behind: %s err=%v", path, err)
		}
	}
	if state := manager.State(); state.GenerationID != 0 || state.PID != 0 || state.Healthy {
		t.Fatalf("unexpected state after failed start: %+v", state)
	}
}

func TestManagerRejectsNonExecutablePluginPath(t *testing.T) {
	runtimeDir := t.TempDir()
	logPath := filepath.Join(runtimeDir, "events.jsonl")
	notExec := filepath.Join(runtimeDir, "not-executable")
	if err := os.WriteFile(notExec, []byte("#!/bin/sh\nexit 0\n"), 0o600); err != nil {
		t.Fatalf("write non-executable file: %v", err)
	}

	if _, err := New(Config{
		RuntimeDir:   runtimeDir,
		PluginPath:   notExec,
		PluginID:     "echo",
		EventLogPath: logPath,
	}); err == nil {
		t.Fatal("expected non-executable plugin path rejection")
	}
}

func TestManagerKillUsesPluginShutdownWhenAvailable(t *testing.T) {
	pluginBin := buildPlugin(t)
	runtimeDir := t.TempDir()
	logPath := filepath.Join(runtimeDir, "events.jsonl")
	shutdownMarker := filepath.Join(runtimeDir, "shutdown.marker")

	manager, err := New(Config{
		RuntimeDir:       runtimeDir,
		PluginPath:       pluginBin,
		PluginID:         "echo",
		DialTimeout:      2 * time.Second,
		CallTimeout:      200 * time.Millisecond,
		EventLogPath:     logPath,
	})
	if err != nil {
		t.Fatalf("new manager: %v", err)
	}
	defer manager.Close()

	os.Setenv("RPC_PLUGIN_SYSTEM_PLUGIN_BEHAVIOR_FAIL_AUTH", "true")
	os.Setenv("RPC_PLUGIN_SYSTEM_PLUGIN_SHUTDOWN_MARKER", shutdownMarker)
	defer os.Unsetenv("RPC_PLUGIN_SYSTEM_PLUGIN_BEHAVIOR_FAIL_AUTH")
	defer os.Unsetenv("RPC_PLUGIN_SYSTEM_PLUGIN_SHUTDOWN_MARKER")

	if err := manager.Start(); err != nil {
		t.Fatalf("start manager: %v", err)
	}
	if err := manager.Kill(); err != nil {
		t.Fatalf("kill: %v", err)
	}
	if _, err := os.Stat(shutdownMarker); err != nil {
		t.Fatalf("shutdown marker missing, graceful shutdown was not observed: %v", err)
	}
	if state := manager.State(); state.PID != 0 || state.Healthy {
		t.Fatalf("unexpected state after kill: %+v", state)
	}
}

func TestManagerRejectsStaleGenerationResponse(t *testing.T) {
	pluginBin := buildPlugin(t)
	runtimeDir := t.TempDir()
	logPath := filepath.Join(runtimeDir, "events.jsonl")

	manager, err := New(Config{
		RuntimeDir:       runtimeDir,
		PluginPath:       pluginBin,
		PluginID:         "echo",
		DialTimeout:      2 * time.Second,
		CallTimeout:      200 * time.Millisecond,
		EventLogPath:     logPath,
	})
	if err != nil {
		t.Fatalf("new manager: %v", err)
	}
	defer manager.Close()

	if err := manager.Start(); err != nil {
		t.Fatalf("start manager: %v", err)
	}

	manager.mu.Lock()
	manager.state.GenerationID++
	client := manager.client
	manager.mu.Unlock()

	var out testpluginapi.EchoResponse
	if err := manager.call(client, manager.State().GenerationID-1, testpluginapi.MethodEcho, testpluginapi.EchoRequest{Message: "stale"}, &out); err == nil {
		t.Fatal("expected stale generation rejection")
	}
}

func TestManagerEventLogRecordsLifecycle(t *testing.T) {
	pluginBin := buildPlugin(t)
	runtimeDir := t.TempDir()
	logPath := filepath.Join(runtimeDir, "events.jsonl")

	manager, err := New(Config{
		RuntimeDir:       runtimeDir,
		PluginPath:       pluginBin,
		PluginID:         "echo",
		DialTimeout:      2 * time.Second,
		CallTimeout:      200 * time.Millisecond,
		EventLogPath:     logPath,
	})
	if err != nil {
		t.Fatalf("new manager: %v", err)
	}
	defer manager.Close()

	if err := manager.Start(); err != nil {
		t.Fatalf("start manager: %v", err)
	}
	if err := manager.Kill(); err != nil {
		t.Fatalf("kill manager: %v", err)
	}
	if err := manager.Start(); err != nil {
		t.Fatalf("restart manager via start: %v", err)
	}

	events := readEvents(t, logPath)
	assertEventType(t, events, "plugin_started")
	assertEventType(t, events, "plugin_stopped")

	startedCount := countEventType(events, "plugin_started")
	if startedCount < 2 {
		t.Fatalf("plugin_started count = %d, want at least 2", startedCount)
	}
}

func TestManagerCloseRemovesRuntimeArtifacts(t *testing.T) {
	pluginBin := buildPlugin(t)
	runtimeDir := t.TempDir()
	logPath := filepath.Join(runtimeDir, "events.jsonl")

	manager, err := New(Config{
		RuntimeDir:       runtimeDir,
		PluginPath:       pluginBin,
		PluginID:         "echo",
		DialTimeout:      2 * time.Second,
		CallTimeout:      200 * time.Millisecond,
		EventLogPath:     logPath,
	})
	if err != nil {
		t.Fatalf("new manager: %v", err)
	}

	if err := manager.Start(); err != nil {
		t.Fatalf("start manager: %v", err)
	}
	if err := manager.Close(); err != nil {
		t.Fatalf("close manager: %v", err)
	}

	for _, path := range []string{
		runtime.SocketPath(runtimeDir, "echo"),
		runtime.AuthPath(runtimeDir, "echo"),
			} {
		if _, err := os.Stat(path); !os.IsNotExist(err) {
			t.Fatalf("runtime artifact still present: %s err=%v", path, err)
		}
	}
}

func readEvents(t *testing.T, path string) []eventlog.Event {
	t.Helper()
	file, err := os.Open(path)
	if err != nil {
		t.Fatalf("open event log: %v", err)
	}
	defer file.Close()

	var events []eventlog.Event
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		var event eventlog.Event
		if err := json.Unmarshal(scanner.Bytes(), &event); err != nil {
			t.Fatalf("decode event log line: %v", err)
		}
		events = append(events, event)
	}
	if err := scanner.Err(); err != nil {
		t.Fatalf("scan event log: %v", err)
	}
	return events
}

func assertEventType(t *testing.T, events []eventlog.Event, want string) {
	t.Helper()
	if countEventType(events, want) == 0 {
		t.Fatalf("event log missing %q in %+v", want, events)
	}
}

func countEventType(events []eventlog.Event, want string) int {
	count := 0
	for _, event := range events {
		if event.Type == want {
			count++
		}
	}
	return count
}
