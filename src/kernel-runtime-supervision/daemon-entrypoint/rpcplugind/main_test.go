package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
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

func TestMainUsesRunAndExitSeam(t *testing.T) {
	oldArgs := os.Args
	oldExit := exitProcess
	oldDeps := daemonDepsForMain
	defer func() {
		os.Args = oldArgs
		exitProcess = oldExit
		daemonDepsForMain = oldDeps
	}()

	var exited int
	exitProcess = func(code int) {
		exited = code
		panic("exit")
	}
	fakeHost := &fakeDaemonHost{}
	daemonDepsForMain = func() daemonDeps {
		return daemonDeps{
			newHost: func(cfg kernel.HostConfig) (daemonHost, error) {
				if cfg.RuntimeDir != "/tmp/main-runtime" {
					t.Fatalf("runtime dir = %q", cfg.RuntimeDir)
				}
				return fakeHost, nil
			},
			serve: func(ctx context.Context, socketPath string, host daemonHost) error {
				if socketPath != "/tmp/main-runtime/admin.sock" {
					t.Fatalf("socket path = %q", socketPath)
				}
				return nil
			},
			notifySignal: func(parent context.Context, signals ...os.Signal) (context.Context, context.CancelFunc) {
				return context.WithCancel(parent)
			},
			logOutput: &bytes.Buffer{},
		}
	}
	os.Args = []string{"rpcplugind", "-runtime-dir", "/tmp/main-runtime", "-plugin", "/bin/true"}

	defer func() {
		if got := recover(); got != "exit" {
			t.Fatalf("main recover = %v, want exit panic", got)
		}
		if exited != 0 {
			t.Fatalf("exit code = %d, want 0", exited)
		}
		if !fakeHost.started || !fakeHost.monitorStarted || !fakeHost.closed {
			t.Fatalf("host lifecycle = started:%v monitor:%v closed:%v", fakeHost.started, fakeHost.monitorStarted, fakeHost.closed)
		}
	}()
	main()
}

func TestProductionDaemonDepsRejectsNonKernelHostForServe(t *testing.T) {
	deps := productionDaemonDeps()
	if deps.logOutput != os.Stderr {
		t.Fatalf("production log output is not stderr")
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	if err := deps.serve(ctx, "/tmp/admin.sock", &fakeDaemonHost{}); err == nil || !strings.Contains(err.Error(), "cannot serve admin rpc") {
		t.Fatalf("production serve err = %v, want host type error", err)
	}
	if _, err := deps.newHost(kernel.HostConfig{}); err == nil || !strings.Contains(err.Error(), "runtime dir is required") {
		t.Fatalf("production newHost err = %v, want runtime dir error", err)
	}
	ctx, cancel = deps.notifySignal(context.Background(), os.Interrupt)
	cancel()
	<-ctx.Done()

	ctx, cancel = context.WithCancel(context.Background())
	cancel()
	if err := deps.serve(ctx, filepath.Join(t.TempDir(), "admin.sock"), (*kernel.Host)(nil)); err != nil {
		t.Fatalf("production serve with canceled context: %v", err)
	}
}

func TestRunHandlesStartupAndServeFailures(t *testing.T) {
	for _, tc := range []struct {
		name       string
		args       []string
		newHostErr error
		startErr   error
		serveErr   error
		wantCode   int
		wantLog    string
		wantClosed bool
	}{
		{name: "flag parse", args: []string{"-badflag"}, wantCode: 2, wantLog: "flag provided but not defined"},
		{name: "help", args: []string{"-h"}, wantCode: 0, wantLog: "Usage of rpcplugind"},
		{name: "config parse", args: nil, wantCode: 1, wantLog: "-plugin is required"},
		{name: "new host", args: []string{"-plugin", "/bin/true"}, newHostErr: errors.New("new host failed"), wantCode: 1, wantLog: "new host failed"},
		{name: "start", args: []string{"-plugin", "/bin/true"}, startErr: errors.New("start failed"), wantCode: 1, wantLog: "start failed", wantClosed: true},
		{name: "serve", args: []string{"-plugin", "/bin/true"}, serveErr: errors.New("serve failed"), wantCode: 1, wantLog: "serve failed", wantClosed: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var log bytes.Buffer
			fakeHost := &fakeDaemonHost{startErr: tc.startErr}
			code := run("rpcplugind", tc.args, daemonDeps{
				newHost: func(cfg kernel.HostConfig) (daemonHost, error) {
					if tc.newHostErr != nil {
						return nil, tc.newHostErr
					}
					return fakeHost, nil
				},
				serve: func(ctx context.Context, socketPath string, host daemonHost) error {
					if socketPath == "" || !strings.HasSuffix(socketPath, "admin.sock") {
						t.Fatalf("socket path = %q", socketPath)
					}
					return tc.serveErr
				},
				notifySignal: func(parent context.Context, signals ...os.Signal) (context.Context, context.CancelFunc) {
					if len(signals) != 2 || signals[0] != os.Interrupt || signals[1] != syscall.SIGTERM {
						t.Fatalf("signals = %+v", signals)
					}
					return context.WithCancel(parent)
				},
				logOutput: &log,
			})
			if code != tc.wantCode {
				t.Fatalf("run code = %d, want %d; log=%s", code, tc.wantCode, log.String())
			}
			if !strings.Contains(log.String(), tc.wantLog) {
				t.Fatalf("run log = %q, want substring %q", log.String(), tc.wantLog)
			}
			if fakeHost.closed != tc.wantClosed {
				t.Fatalf("host closed = %v, want %v", fakeHost.closed, tc.wantClosed)
			}
		})
	}
}

func TestRunReturnsZeroAfterServeStops(t *testing.T) {
	var log bytes.Buffer
	fakeHost := &fakeDaemonHost{}
	code := run("rpcplugind", []string{"-runtime-dir", "/tmp/run-runtime", "-plugin", "/bin/true"}, daemonDeps{
		newHost: func(cfg kernel.HostConfig) (daemonHost, error) {
			if cfg.RuntimeDir != "/tmp/run-runtime" || len(cfg.Plugins) != 1 || cfg.Plugins[0].PluginID != "echo" {
				t.Fatalf("config = %+v", cfg)
			}
			return fakeHost, nil
		},
		serve: func(ctx context.Context, socketPath string, host daemonHost) error {
			if socketPath != "/tmp/run-runtime/admin.sock" {
				t.Fatalf("socket path = %q", socketPath)
			}
			return nil
		},
		notifySignal: func(parent context.Context, signals ...os.Signal) (context.Context, context.CancelFunc) {
			return context.WithCancel(parent)
		},
		logOutput: &log,
	})
	if code != 0 {
		t.Fatalf("run code = %d, want 0; log=%s", code, log.String())
	}
	if !fakeHost.started || !fakeHost.monitorStarted || !fakeHost.closed {
		t.Fatalf("host lifecycle = started:%v monitor:%v closed:%v", fakeHost.started, fakeHost.monitorStarted, fakeHost.closed)
	}
	if log.Len() != 0 {
		t.Fatalf("run log = %q, want empty", log.String())
	}
}

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

func TestParseDaemonConfigBuildsKernelHostConfig(t *testing.T) {
	cfg, err := parseDaemonConfig(daemonOptions{
		RuntimeDir: "/tmp/rpc-runtime",
		PluginsArg: "alpha=/bin/alpha,beta=/bin/beta",
		PluginID:   "ignored",
		PluginPath: "/bin/ignored",
	})
	if err != nil {
		t.Fatalf("parseDaemonConfig: %v", err)
	}
	if cfg.RuntimeDir != "/tmp/rpc-runtime" {
		t.Fatalf("runtime dir = %q", cfg.RuntimeDir)
	}
	if cfg.DialTimeout != 3*time.Second || cfg.CallTimeout != 500*time.Millisecond || cfg.HeartbeatEvery != 2*time.Second {
		t.Fatalf("unexpected timeout config: dial=%s call=%s heartbeat=%s", cfg.DialTimeout, cfg.CallTimeout, cfg.HeartbeatEvery)
	}
	if len(cfg.Plugins) != 2 {
		t.Fatalf("plugin count = %d, want 2", len(cfg.Plugins))
	}
	if cfg.Plugins[0] != (kernel.PluginConfig{PluginID: "alpha", PluginPath: "/bin/alpha"}) {
		t.Fatalf("plugin[0] = %+v", cfg.Plugins[0])
	}
	if cfg.Plugins[1] != (kernel.PluginConfig{PluginID: "beta", PluginPath: "/bin/beta"}) {
		t.Fatalf("plugin[1] = %+v", cfg.Plugins[1])
	}
}

func TestParseDaemonConfigReturnsParseErrors(t *testing.T) {
	_, err := parseDaemonConfig(daemonOptions{RuntimeDir: "/tmp/rpc-runtime"})
	if err == nil || !strings.Contains(err.Error(), "-plugin is required") {
		t.Fatalf("parseDaemonConfig err = %v, want missing plugin error", err)
	}
}

func TestParsePluginsAcceptsSingleAndMultiPluginConfig(t *testing.T) {
	t.Run("single plugin compatibility", func(t *testing.T) {
		plugins, err := parsePlugins("", "echo", "/bin/echo")
		if err != nil {
			t.Fatalf("parsePlugins: %v", err)
		}
		if len(plugins) != 1 || plugins[0] != (kernel.PluginConfig{PluginID: "echo", PluginPath: "/bin/echo"}) {
			t.Fatalf("plugins = %+v", plugins)
		}
	})

	t.Run("multi plugin trims spec boundaries", func(t *testing.T) {
		plugins, err := parsePlugins(" alpha=/bin/alpha , beta=/bin/beta ", "ignored", "")
		if err != nil {
			t.Fatalf("parsePlugins: %v", err)
		}
		want := []kernel.PluginConfig{
			{PluginID: "alpha", PluginPath: "/bin/alpha"},
			{PluginID: "beta", PluginPath: "/bin/beta"},
		}
		if len(plugins) != len(want) {
			t.Fatalf("plugin count = %d, want %d", len(plugins), len(want))
		}
		for i := range want {
			if plugins[i] != want[i] {
				t.Fatalf("plugins[%d] = %+v, want %+v", i, plugins[i], want[i])
			}
		}
	})
}

func TestParsePluginsRejectsMalformedSpecs(t *testing.T) {
	for _, tc := range []struct {
		name    string
		plugins string
		wantErr string
	}{
		{name: "missing equals", plugins: "echo", wantErr: "invalid plugin spec"},
		{name: "empty id", plugins: "=/bin/true", wantErr: "invalid plugin spec"},
		{name: "empty path", plugins: "echo=", wantErr: "invalid plugin spec"},
		{name: "empty part after comma", plugins: "echo=/bin/true,", wantErr: "invalid plugin spec"},
		{name: "space before equals is plugin id", plugins: "echo =/bin/true", wantErr: "invalid plugin id"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, err := parsePlugins(tc.plugins, "ignored", "")
			if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
				t.Fatalf("parsePlugins err = %v, want substring %q", err, tc.wantErr)
			}
		})
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

type fakeDaemonHost struct {
	startErr       error
	closed         bool
	started        bool
	monitorStarted bool
}

func (h *fakeDaemonHost) StartAll() error {
	h.started = true
	return h.startErr
}

func (h *fakeDaemonHost) MonitorLoop(context.Context) {
	h.monitorStarted = true
}

func (h *fakeDaemonHost) Close() error {
	h.closed = true
	return nil
}
