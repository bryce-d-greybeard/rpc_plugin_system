package kernel

import (
	"path/filepath"
	"testing"
	"time"
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
