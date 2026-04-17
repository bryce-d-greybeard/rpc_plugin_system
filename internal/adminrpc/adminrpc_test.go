package adminrpc

import (
	"context"
	"fmt"
	"net/rpc"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"rpc_plugin_system/internal/auth"
	"rpc_plugin_system/internal/kernel"
)

func TestStatusAndRestart(t *testing.T) {
	token, err := auth.NewToken()
	if err != nil {
		t.Fatalf("generate auth token: %v", err)
	}
	pluginBin := buildPlugin(t)
	runtimeDir := t.TempDir()
	logPath := filepath.Join(runtimeDir, "events.jsonl")
	adminSocket := filepath.Join(runtimeDir, "admin.sock")

	manager, err := kernel.New(kernel.Config{
		RuntimeDir:       runtimeDir,
		PluginPath:       pluginBin,
		PluginID:         "echo",
		DialTimeout:      2 * time.Second,
		CallTimeout:      200 * time.Millisecond,
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
	firstGeneration := manager.State().GenerationID

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() {
		if err := Serve(ctx, adminSocket, manager); err != nil {
			t.Errorf("serve admin rpc: %v", err)
		}
	}()

	var client *rpc.Client
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		client, err = Dial(adminSocket, 100*time.Millisecond)
		if err == nil {
			break
		}
		time.Sleep(50 * time.Millisecond)
	}
	if err != nil {
		t.Fatalf("dial admin rpc: %v", err)
	}
	defer client.Close()

	info, err := os.Stat(adminSocket)
	if err != nil {
		t.Fatalf("stat admin socket: %v", err)
	}
	if got := info.Mode().Perm(); got != 0o600 {
		t.Fatalf("admin socket permissions = %o, want 600", got)
	}

	var state kernel.State
	if err := client.Call(MethodStatus, Empty{}, &state); err != nil {
		t.Fatalf("status rpc: %v", err)
	}
	if state.PluginID != "echo" || state.GenerationID != firstGeneration || state.PID == 0 {
		t.Fatalf("unexpected status state: %+v", state)
	}

	var restarted kernel.State
	if err := client.Call(MethodRestart, Empty{}, &restarted); err != nil {
		t.Fatalf("restart rpc: %v", err)
	}
	if restarted.GenerationID != firstGeneration+1 {
		t.Fatalf("generation did not increment: got %d want %d", restarted.GenerationID, firstGeneration+1)
	}
	if restarted.PID == 0 {
		t.Fatalf("restart did not leave plugin running: %+v", restarted)
	}
}

var (
	pluginBuildOnce sync.Once
	pluginBuildPath string
	pluginBuildErr  error
)

func buildPlugin(t *testing.T) string {
	t.Helper()
	pluginBuildOnce.Do(func() {
		cacheDir, err := os.MkdirTemp("", "rpc_plugin_system-adminrpc-test-")
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
