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

	"rpc_plugin_system/internal/kernel"
	"rpc_plugin_system/internal/testpluginapi"
)

func TestStatusAndRestart(t *testing.T) {
	pluginBin := buildPlugin(t)
	runtimeDir := t.TempDir()
	adminSocket := filepath.Join(runtimeDir, "admin.sock")

	host, err := kernel.NewHost(kernel.HostConfig{
		RuntimeDir:     runtimeDir,
		DialTimeout:    2 * time.Second,
		CallTimeout:    200 * time.Millisecond,
		HeartbeatEvery: 100 * time.Millisecond,
		Plugins:        []kernel.PluginConfig{{PluginID: "echo", PluginPath: pluginBin}},
	})
	if err != nil {
		t.Fatalf("new host: %v", err)
	}
	defer host.Close()

	if err := host.StartAll(); err != nil {
		t.Fatalf("start all: %v", err)
	}
	firstGeneration, err := host.Manager("echo")
	if err != nil {
		t.Fatalf("manager echo: %v", err)
	}
	firstState := firstGeneration.State()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() {
		if err := Serve(ctx, adminSocket, host); err != nil {
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

	var state kernel.HostState
	if err := client.Call(MethodStatus, Empty{}, &state); err != nil {
		t.Fatalf("status rpc: %v", err)
	}
	if len(state.Plugins) != 1 || state.Plugins[0].PluginID != "echo" || state.Plugins[0].GenerationID != firstState.GenerationID || state.Plugins[0].PID == 0 {
		t.Fatalf("unexpected status state: %+v", state)
	}

	var pluginState kernel.State
	if err := client.Call(MethodPlugin, PluginRequest{PluginID: "echo"}, &pluginState); err != nil {
		t.Fatalf("plugin rpc: %v", err)
	}
	if pluginState.PluginID != "echo" || pluginState.PID == 0 {
		t.Fatalf("unexpected plugin state: %+v", pluginState)
	}

	var caps map[string][]string
	if err := client.Call(MethodCapabilities, Empty{}, &caps); err != nil {
		t.Fatalf("capabilities rpc: %v", err)
	}
	if len(caps["echo"]) != 1 || caps["echo"][0] != "echo" {
		t.Fatalf("unexpected capability map: %+v", caps)
	}

	var routes []kernel.Route
	if err := client.Call(MethodRoutes, Empty{}, &routes); err != nil {
		t.Fatalf("routes rpc: %v", err)
	}
	if len(routes) != 1 || routes[0].PluginID != "echo" {
		t.Fatalf("unexpected routes: %+v", routes)
	}

	var hb testpluginapi.HeartbeatResponse
	if err := client.Call(MethodHeartbeat, HeartbeatRequest{PluginID: "echo"}, &hb); err != nil {
		t.Fatalf("heartbeat rpc: %v", err)
	}
	if hb.PluginID != "echo" {
		t.Fatalf("unexpected heartbeat plugin id: %+v", hb)
	}

	var echoed testpluginapi.EchoResponse
	if err := client.Call(MethodEcho, EchoRequest{PluginID: "echo", Message: "admin-router"}, &echoed); err != nil {
		t.Fatalf("echo rpc: %v", err)
	}
	if echoed.Message != "admin-router" {
		t.Fatalf("unexpected echo response: %+v", echoed)
	}

	var restarted kernel.State
	if err := client.Call(MethodRestart, RestartRequest{PluginID: "echo"}, &restarted); err != nil {
		t.Fatalf("restart rpc: %v", err)
	}
	if restarted.GenerationID != firstState.GenerationID+1 {
		t.Fatalf("generation did not increment: got %d want %d", restarted.GenerationID, firstState.GenerationID+1)
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
		cmd := exec.Command("go", "build", "-buildvcs=false", "-o", pluginBuildPath, "./cmd/rpcplugin-echo")
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
