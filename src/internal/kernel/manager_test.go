package kernel

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"rpc_plugin_system/internal/eventlog"
	"rpc_plugin_system/internal/runtime"
	"rpc_plugin_system/test/testpluginapi"
	"rpc_plugin_system/test/testroot"
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
		cmd := exec.Command("go", "build", "-buildvcs=false", "-o", pluginBuildPath, "./examples/rpcplugin-echo")
		cmd.Dir = testroot.SourceRoot(t)
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

func TestManagerSerializesConcurrentRestarts(t *testing.T) {
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

	if err := manager.Start(); err != nil {
		t.Fatalf("start manager: %v", err)
	}
	firstGen := manager.State().GenerationID

	const restarts = 4
	errCh := make(chan error, restarts)
	var ready sync.WaitGroup
	ready.Add(restarts)
	start := make(chan struct{})
	for i := 0; i < restarts; i++ {
		go func() {
			ready.Done()
			<-start
			errCh <- manager.Restart()
		}()
	}
	ready.Wait()
	close(start)
	for i := 0; i < restarts; i++ {
		if err := <-errCh; err != nil {
			t.Fatalf("restart %d: %v", i, err)
		}
	}

	state := manager.State()
	if got, want := state.GenerationID, firstGen+restarts; got != want {
		t.Fatalf("generation after concurrent restarts = %d, want %d", got, want)
	}
	if !state.Healthy || state.PID == 0 {
		t.Fatalf("manager not healthy after concurrent restarts: %+v", state)
	}
	if got, err := manager.Echo("after-restarts"); err != nil || got != "after-restarts" {
		t.Fatalf("echo after concurrent restarts = %q, %v", got, err)
	}
}

func TestManagerSkipsStaleRuntimeArtifactCleanup(t *testing.T) {
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

	if err := manager.Start(); err != nil {
		t.Fatalf("start manager: %v", err)
	}
	staleGen := manager.State().GenerationID
	if err := manager.Restart(); err != nil {
		t.Fatalf("restart manager: %v", err)
	}

	socketPath := runtime.SocketPath(runtimeDir, "echo")
	authPath := runtime.AuthPath(runtimeDir, "echo")
	if err := os.WriteFile(authPath, []byte("current-generation-auth"), 0o600); err != nil {
		t.Fatalf("write current auth artifact: %v", err)
	}

	manager.cleanupRuntimeArtifacts(staleGen)

	if _, err := os.Stat(socketPath); err != nil {
		t.Fatalf("stale cleanup removed current socket: %v", err)
	}
	if got, err := os.ReadFile(authPath); err != nil || string(got) != "current-generation-auth" {
		t.Fatalf("stale cleanup changed current auth artifact: got %q err=%v", string(got), err)
	}
	if got, err := manager.Echo("still-current"); err != nil || got != "still-current" {
		t.Fatalf("echo after stale cleanup = %q, %v", got, err)
	}
}

func TestManagerStartRejectsAlreadyRunningPlugin(t *testing.T) {
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

	if err := manager.Start(); err != nil {
		t.Fatalf("start manager: %v", err)
	}
	state := manager.State()
	if err := manager.Start(); err == nil {
		t.Fatal("expected second start to fail while plugin is already running")
	}
	if got := manager.State(); got.GenerationID != state.GenerationID || got.PID != state.PID || !got.Healthy {
		t.Fatalf("second start changed running generation: before=%+v after=%+v", state, got)
	}
	if got, err := manager.Echo("still-running"); err != nil || got != "still-running" {
		t.Fatalf("echo after rejected second start = %q, %v", got, err)
	}
}

func TestManagerSleepTimeout(t *testing.T) {
	pluginBin := buildPlugin(t)
	runtimeDir := t.TempDir()
	logPath := filepath.Join(runtimeDir, "events.jsonl")

	manager, err := New(Config{
		RuntimeDir:   runtimeDir,
		PluginPath:   pluginBin,
		PluginID:     "echo",
		DialTimeout:  2 * time.Second,
		CallTimeout:  100 * time.Millisecond,
		EventLogPath: logPath,
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

	events := readEvents(t, logPath)
	assertEvent(t, events, eventlog.EventRPCTimeout, func(event eventlog.Event) bool {
		return event.Method == testpluginapi.MethodSleep && event.Level == eventlog.LevelError
	})
	assertEvent(t, events, eventlog.EventRPCPoisoned, func(event eventlog.Event) bool {
		return event.Method == testpluginapi.MethodSleep && event.Reason == "rpc timeout"
	})
}

func TestManagerRejectsUntrustedPlugin(t *testing.T) {
	pluginBin := buildFailurePlugin(t)
	runtimeDir := t.TempDir()
	logPath := filepath.Join(runtimeDir, "events.jsonl")

	manager, err := New(Config{
		RuntimeDir:   runtimeDir,
		PluginPath:   pluginBin,
		PluginID:     "failure",
		DialTimeout:  2 * time.Second,
		CallTimeout:  200 * time.Millisecond,
		EventLogPath: logPath,
	})
	if err != nil {
		t.Fatalf("new manager: %v", err)
	}
	defer manager.Close()

	behaviorPath := writeBehaviorConfig(t, runtimeDir, testpluginapi.Env{FailAuth: true})
	oldEnv := setEnvMap(t, map[string]string{testpluginapi.BehaviorConfigEnv: behaviorPath})
	defer restoreEnvMap(oldEnv)

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

	events := readEvents(t, logPath)
	assertEvent(t, events, eventlog.EventAuthStarted, func(event eventlog.Event) bool {
		return event.PluginID == "failure"
	})
	assertEvent(t, events, eventlog.EventAuthFailed, func(event eventlog.Event) bool {
		return event.PluginID == "failure"
	})
	assertEvent(t, events, eventlog.EventPluginStartFailed, func(event eventlog.Event) bool {
		return event.PluginID == "failure"
	})
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

	oldEnv := setEnvMap(t, map[string]string{testpluginapi.ShutdownMarkerEnv: shutdownMarker})
	defer restoreEnvMap(oldEnv)

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

	events := readEvents(t, logPath)
	assertEvent(t, events, eventlog.EventShutdownRequested, func(event eventlog.Event) bool {
		return event.PluginID == "echo"
	})
	assertEvent(t, events, eventlog.EventShutdownSucceeded, func(event eventlog.Event) bool {
		return event.PluginID == "echo"
	})
	assertEvent(t, events, eventlog.EventPluginStopped, func(event eventlog.Event) bool {
		return event.PluginID == "echo"
	})
}
