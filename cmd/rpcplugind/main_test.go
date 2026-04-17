package main

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"syscall"
	"testing"
	"time"

	"rpc_plugin_system/internal/auth"
	"rpc_plugin_system/internal/kernel"
)

func TestDaemonAndCLIEndToEnd(t *testing.T) {
	daemonBin := buildBinary(t, "rpcplugind", "./cmd/rpcplugind")
	ctlBin := buildBinary(t, "rpcpluginctl", "./cmd/rpcpluginctl")
	pluginBin := buildBinary(t, "rpcplugin-echo", "./cmd/rpcplugin-echo")

	token, err := auth.NewToken()
	if err != nil {
		t.Fatalf("generate auth token: %v", err)
	}

	runtimeDir := t.TempDir()
	cmd := exec.Command(daemonBin, "-runtime-dir", runtimeDir, "-plugin", pluginBin)
	cmd.Env = append(os.Environ(), "RPC_PLUGIN_SYSTEM_AUTH_TOKEN_FILE="+auth.Encode(token))
	if err := cmd.Start(); err != nil {
		t.Fatalf("start daemon: %v", err)
	}
	defer stopDaemon(t, cmd)

	status := waitForStatus(t, ctlBin, runtimeDir)
	if status.PluginID != "echo" || status.GenerationID == 0 || status.PID == 0 || !status.Healthy {
		t.Fatalf("unexpected initial status: %+v", status)
	}

	restarted := runStatusCommand(t, ctlBin, runtimeDir, "restart")
	if restarted.GenerationID != status.GenerationID+1 {
		t.Fatalf("generation after restart = %d, want %d", restarted.GenerationID, status.GenerationID+1)
	}
	if restarted.PID == 0 || !restarted.Healthy {
		t.Fatalf("unexpected restarted status: %+v", restarted)
	}
}

func TestDaemonShutdownRemovesRuntimeArtifacts(t *testing.T) {
	daemonBin := buildBinary(t, "rpcplugind", "./cmd/rpcplugind")
	ctlBin := buildBinary(t, "rpcpluginctl", "./cmd/rpcpluginctl")
	pluginBin := buildBinary(t, "rpcplugin-echo", "./cmd/rpcplugin-echo")

	token, err := auth.NewToken()
	if err != nil {
		t.Fatalf("generate auth token: %v", err)
	}

	runtimeDir := t.TempDir()
	cmd := exec.Command(daemonBin, "-runtime-dir", runtimeDir, "-plugin", pluginBin)
	cmd.Env = append(os.Environ(), "RPC_PLUGIN_SYSTEM_AUTH_TOKEN_FILE="+auth.Encode(token))
	if err := cmd.Start(); err != nil {
		t.Fatalf("start daemon: %v", err)
	}

	_ = waitForStatus(t, ctlBin, runtimeDir)
	stopDaemon(t, cmd)

	for _, path := range []string{
		filepath.Join(runtimeDir, "admin.sock"),
		filepath.Join(runtimeDir, "echo.sock"),
		filepath.Join(runtimeDir, "echo.auth"),
			} {
		if _, err := os.Stat(path); !os.IsNotExist(err) {
			t.Fatalf("daemon shutdown left runtime artifact behind: %s err=%v", path, err)
		}
	}
}

func waitForStatus(t *testing.T, ctlBin, runtimeDir string) kernel.State {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		state, err := tryStatusCommand(ctlBin, runtimeDir, "status")
		if err == nil {
			return state
		}
		time.Sleep(100 * time.Millisecond)
	}
	t.Fatal("timed out waiting for daemon status")
	return kernel.State{}
}

func runStatusCommand(t *testing.T, ctlBin, runtimeDir, subcommand string) kernel.State {
	t.Helper()
	state, err := tryStatusCommand(ctlBin, runtimeDir, subcommand)
	if err != nil {
		t.Fatalf("run %s: %v", subcommand, err)
	}
	return state
}

func tryStatusCommand(ctlBin, runtimeDir, subcommand string) (kernel.State, error) {
	cmd := exec.Command(ctlBin, "-runtime-dir", runtimeDir, subcommand)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return kernel.State{}, err
	}
	var state kernel.State
	if err := json.Unmarshal(out, &state); err != nil {
		return kernel.State{}, err
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

	cmd := exec.Command("go", "build", "-o", bin, pkg)
	cmd.Dir = filepath.Clean(filepath.Join("..", ".."))
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
