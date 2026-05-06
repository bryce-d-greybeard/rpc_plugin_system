package main

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"

	"rpc_plugin_system/kernel-runtime-supervision/manager-lifecycle/kernel"
	"rpc_plugin_system/release-packaging-governance/go-build-deps/testroot"
)

func TestDaemonAndCLIEndToEnd(t *testing.T) {
	daemonBin := buildBinary(t, "rpcplugind", "./kernel-runtime-supervision/daemon-entrypoint/rpcplugind")
	ctlBin := buildBinary(t, "rpcpluginctl", "./control-plane-ops/cli-status-control/rpcpluginctl")
	pluginBin := buildBinary(t, "rpcplugin-echo", "./plugin-authoring-sdk/echo-example/rpcplugin-echo")

	runtimeDir := t.TempDir()
	cmd := exec.Command(daemonBin, "-runtime-dir", runtimeDir, "-plugin", pluginBin)
	if err := cmd.Start(); err != nil {
		t.Fatalf("start daemon: %v", err)
	}
	defer stopDaemon(t, cmd)

	status := waitForHostStatus(t, ctlBin, runtimeDir)
	if len(status.Plugins) != 1 {
		t.Fatalf("plugin count = %d, want 1", len(status.Plugins))
	}
	plugin := status.Plugins[0]
	if plugin.PluginID != "echo" || plugin.GenerationID == 0 || plugin.PID == 0 || !plugin.Healthy {
		t.Fatalf("unexpected initial status: %+v", plugin)
	}

	restarted := runRestartCommand(t, ctlBin, runtimeDir, "echo")
	if restarted.GenerationID != plugin.GenerationID+1 {
		t.Fatalf("generation after restart = %d, want %d", restarted.GenerationID, plugin.GenerationID+1)
	}
	if restarted.PID == 0 || !restarted.Healthy {
		t.Fatalf("unexpected restarted status: %+v", restarted)
	}
}

func TestParsePluginsRejectsUnsafePluginIDs(t *testing.T) {
	for _, tc := range []struct {
		name     string
		plugins  string
		pluginID string
		wantErr  string
	}{
		{name: "single path traversal", pluginID: "../echo", wantErr: "invalid plugin id"},
		{name: "single slash", pluginID: "echo/test", wantErr: "invalid plugin id"},
		{name: "single empty", pluginID: "", wantErr: "plugin id is required"},
		{name: "multi path traversal", plugins: "../echo=/bin/true", wantErr: "invalid plugin id"},
		{name: "multi slash", plugins: "echo/test=/bin/true", wantErr: "invalid plugin id"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, err := parsePlugins(tc.plugins, tc.pluginID, "/bin/true")
			if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
				t.Fatalf("parsePlugins err = %v, want substring %q", err, tc.wantErr)
			}
		})
	}
}

func TestDaemonShutdownRemovesRuntimeArtifacts(t *testing.T) {
	daemonBin := buildBinary(t, "rpcplugind", "./kernel-runtime-supervision/daemon-entrypoint/rpcplugind")
	ctlBin := buildBinary(t, "rpcpluginctl", "./control-plane-ops/cli-status-control/rpcpluginctl")
	pluginBin := buildBinary(t, "rpcplugin-echo", "./plugin-authoring-sdk/echo-example/rpcplugin-echo")

	runtimeDir := t.TempDir()
	cmd := exec.Command(daemonBin, "-runtime-dir", runtimeDir, "-plugin", pluginBin)
	if err := cmd.Start(); err != nil {
		t.Fatalf("start daemon: %v", err)
	}

	_ = waitForHostStatus(t, ctlBin, runtimeDir)
	stopDaemon(t, cmd)

	for _, path := range []string{
		filepath.Join(runtimeDir, "admin.sock"),
		filepath.Join(runtimeDir, "echo", "echo.sock"),
		filepath.Join(runtimeDir, "echo", "echo.auth"),
	} {
		if _, err := os.Stat(path); !os.IsNotExist(err) {
			t.Fatalf("daemon shutdown left runtime artifact behind: %s err=%v", path, err)
		}
	}
}

func waitForHostStatus(t *testing.T, ctlBin, runtimeDir string) kernel.HostState {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		state, err := tryHostStatusCommand(ctlBin, runtimeDir, "status")
		if err == nil && len(state.Plugins) > 0 && state.Plugins[0].PID != 0 {
			return state
		}
		time.Sleep(100 * time.Millisecond)
	}
	t.Fatal("timed out waiting for daemon status")
	return kernel.HostState{}
}

func runRestartCommand(t *testing.T, ctlBin, runtimeDir, pluginID string) kernel.State {
	t.Helper()
	cmd := exec.Command(ctlBin, "-runtime-dir", runtimeDir, "-plugin-id", pluginID, "restart")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("run restart: %v\n%s", err, string(out))
	}
	var state kernel.State
	if err := json.Unmarshal(out, &state); err != nil {
		t.Fatalf("decode restart state: %v", err)
	}
	return state
}

func tryHostStatusCommand(ctlBin, runtimeDir, subcommand string) (kernel.HostState, error) {
	cmd := exec.Command(ctlBin, "-runtime-dir", runtimeDir, subcommand)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return kernel.HostState{}, err
	}
	var state kernel.HostState
	if err := json.Unmarshal(out, &state); err != nil {
		return kernel.HostState{}, err
	}
	return state, nil
}

func stopDaemon(t *testing.T, cmd *exec.Cmd) {
	t.Helper()
	if cmd.Process == nil {
		return
	}
	_ = cmd.Process.Signal(syscall.SIGTERM)
	waitCh := make(chan error, 1)
	go func() { waitCh <- cmd.Wait() }()
	select {
	case <-time.After(3 * time.Second):
		_ = cmd.Process.Kill()
		<-waitCh
	case <-waitCh:
	}
}

var (
	binaryBuildMu    sync.Mutex
	binaryBuildCache = map[string]string{}
	binaryBuildErrs  = map[string]error{}
)

func buildBinary(t *testing.T, name, pkg string) string {
	t.Helper()

	binaryBuildMu.Lock()
	if err := binaryBuildErrs[name]; err != nil {
		binaryBuildMu.Unlock()
		t.Fatal(err)
	}
	if bin := binaryBuildCache[name]; bin != "" {
		binaryBuildMu.Unlock()
		return bin
	}
	cacheDir, err := os.MkdirTemp("", "rpc_plugin_system-daemon-test-")
	if err != nil {
		binaryBuildMu.Unlock()
		t.Fatalf("make binary cache dir for %s: %v", name, err)
	}
	bin := filepath.Join(cacheDir, name)
	binaryBuildMu.Unlock()

	cmd := exec.Command("go", "build", "-buildvcs=false", "-o", bin, pkg)
	cmd.Dir = testroot.SourceRoot(t)
	out, err := cmd.CombinedOutput()
	if err != nil {
		err = fmt.Errorf("build %s: %w\n%s", name, err, string(out))
	}

	binaryBuildMu.Lock()
	defer binaryBuildMu.Unlock()
	if err != nil {
		binaryBuildErrs[name] = err
		t.Fatal(err)
	}
	binaryBuildCache[name] = bin
	return bin
}
