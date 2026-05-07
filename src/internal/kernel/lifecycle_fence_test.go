package kernel

import (
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"rpc_plugin_system/test/testpluginapi"
)

func TestManagerRejectsInFlightHeartbeatAfterKill(t *testing.T) {
	pluginBin := buildFailurePlugin(t)
	runtimeDir := t.TempDir()
	behaviorPath := writeBehaviorConfig(t, runtimeDir, testpluginapi.Env{HeartbeatDelayMS: 250, ShutdownDelayMS: 500})
	t.Setenv(testpluginapi.BehaviorConfigEnv, behaviorPath)

	manager, err := New(Config{
		RuntimeDir:   runtimeDir,
		PluginPath:   pluginBin,
		PluginID:     "failure",
		DialTimeout:  2 * time.Second,
		CallTimeout:  750 * time.Millisecond,
		EventLogPath: filepath.Join(runtimeDir, "events.jsonl"),
	})
	if err != nil {
		t.Fatalf("new manager: %v", err)
	}
	defer manager.Close()
	if err := manager.Start(); err != nil {
		t.Fatalf("start manager: %v", err)
	}

	errCh := make(chan error, 1)
	go func() {
		_, err := manager.Heartbeat()
		errCh <- err
	}()
	time.Sleep(50 * time.Millisecond)
	if err := manager.Kill(); err != nil {
		t.Fatalf("kill manager: %v", err)
	}

	err = <-errCh
	if err == nil {
		t.Fatal("in-flight heartbeat succeeded after kill")
	}
	if !strings.Contains(err.Error(), "stale") && !strings.Contains(err.Error(), "shutdown") && !strings.Contains(err.Error(), "closed") {
		t.Fatalf("heartbeat error = %v, want stale/closed transport failure", err)
	}
	state := manager.State()
	if state.Healthy || state.PID != 0 {
		t.Fatalf("stale heartbeat mutated stopped state: %+v", state)
	}
}

func TestManagerRejectsInFlightHeartbeatAfterRestart(t *testing.T) {
	pluginBin := buildFailurePlugin(t)
	runtimeDir := t.TempDir()
	behaviorPath := writeBehaviorConfig(t, runtimeDir, testpluginapi.Env{HeartbeatDelayMS: 250})
	t.Setenv(testpluginapi.BehaviorConfigEnv, behaviorPath)

	manager, err := New(Config{
		RuntimeDir:   runtimeDir,
		PluginPath:   pluginBin,
		PluginID:     "failure",
		DialTimeout:  2 * time.Second,
		CallTimeout:  750 * time.Millisecond,
		EventLogPath: filepath.Join(runtimeDir, "events.jsonl"),
	})
	if err != nil {
		t.Fatalf("new manager: %v", err)
	}
	defer manager.Close()
	if err := manager.Start(); err != nil {
		t.Fatalf("start manager: %v", err)
	}
	firstGen := manager.State().GenerationID

	errCh := make(chan error, 1)
	go func() {
		_, err := manager.Heartbeat()
		errCh <- err
	}()
	time.Sleep(50 * time.Millisecond)
	if err := manager.Restart(); err != nil {
		t.Fatalf("restart manager: %v", err)
	}

	if err := <-errCh; err == nil {
		t.Fatal("in-flight heartbeat succeeded after restart")
	}
	state := manager.State()
	if state.GenerationID != firstGen+1 || !state.Healthy || state.PID == 0 {
		t.Fatalf("restart did not leave fresh healthy generation: %+v", state)
	}
}

func TestManagerCloseIsIdempotent(t *testing.T) {
	pluginBin := buildPlugin(t)
	runtimeDir := t.TempDir()
	manager, err := New(Config{
		RuntimeDir:   runtimeDir,
		PluginPath:   pluginBin,
		PluginID:     "echo",
		DialTimeout:  2 * time.Second,
		CallTimeout:  200 * time.Millisecond,
		EventLogPath: filepath.Join(runtimeDir, "events.jsonl"),
	})
	if err != nil {
		t.Fatalf("new manager: %v", err)
	}
	if err := manager.Start(); err != nil {
		t.Fatalf("start manager: %v", err)
	}
	for i := 0; i < 3; i++ {
		if err := manager.Close(); err != nil {
			t.Fatalf("close #%d: %v", i+1, err)
		}
	}
}

func TestManagerConcurrentCloseIsIdempotent(t *testing.T) {
	pluginBin := buildPlugin(t)
	runtimeDir := t.TempDir()
	manager, err := New(Config{
		RuntimeDir:   runtimeDir,
		PluginPath:   pluginBin,
		PluginID:     "echo",
		DialTimeout:  2 * time.Second,
		CallTimeout:  200 * time.Millisecond,
		EventLogPath: filepath.Join(runtimeDir, "events.jsonl"),
	})
	if err != nil {
		t.Fatalf("new manager: %v", err)
	}
	if err := manager.Start(); err != nil {
		t.Fatalf("start manager: %v", err)
	}

	var wg sync.WaitGroup
	errCh := make(chan error, 8)
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			errCh <- manager.Close()
		}()
	}
	wg.Wait()
	close(errCh)
	for err := range errCh {
		if err != nil {
			t.Fatalf("concurrent close: %v", err)
		}
	}
}
