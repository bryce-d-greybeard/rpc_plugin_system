package main

import (
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	plugin "rpc_plugin_system/plugin-authoring-sdk/go-runtime/plugin"
	"rpc_plugin_system/plugin-authoring-sdk/test-plugin-api/testpluginapi"
)

func newTestEchoPlugin() *echoPlugin {
	return &echoPlugin{TemplatePlugin: plugin.NewTemplate(plugin.Config{PluginID: "echo-test", GenerationID: 42}, "0.1.0")}
}

func TestRunReportsMissingConfig(t *testing.T) {
	t.Setenv("RPC_PLUGIN_SYSTEM_PLUGIN_SOCKET", "")
	t.Setenv("RPC_PLUGIN_SYSTEM_PLUGIN_ID", "")
	t.Setenv("RPC_PLUGIN_SYSTEM_PLUGIN_GENERATION", "")
	t.Setenv("RPC_PLUGIN_SYSTEM_AUTH_TOKEN_FILE", "")

	if err := run(); err == nil {
		t.Fatalf("run succeeded without required plugin startup environment")
	}
}

func setValidRunEnv(t *testing.T) plugin.Config {
	t.Helper()
	dir := t.TempDir()
	tokenFile := filepath.Join(dir, "token")
	if err := os.WriteFile(tokenFile, []byte("token"), 0o600); err != nil {
		t.Fatalf("write token file: %v", err)
	}
	t.Setenv("RPC_PLUGIN_SYSTEM_PLUGIN_SOCKET", filepath.Join(dir, "plugin.sock"))
	t.Setenv("RPC_PLUGIN_SYSTEM_PLUGIN_ID", "echo-test")
	t.Setenv("RPC_PLUGIN_SYSTEM_PLUGIN_GENERATION", "42")
	t.Setenv("RPC_PLUGIN_SYSTEM_AUTH_TOKEN_FILE", tokenFile)
	return plugin.Config{SocketPath: filepath.Join(dir, "plugin.sock"), PluginID: "echo-test", GenerationID: 42, AuthToken: "token"}
}

func TestRunReportsLoggerOpenFailure(t *testing.T) {
	tokenFile := filepath.Join(t.TempDir(), "token")
	if err := os.WriteFile(tokenFile, []byte("token"), 0o600); err != nil {
		t.Fatalf("write token file: %v", err)
	}
	t.Setenv("RPC_PLUGIN_SYSTEM_PLUGIN_SOCKET", filepath.Join(t.TempDir(), "missing", "plugin.sock"))
	t.Setenv("RPC_PLUGIN_SYSTEM_PLUGIN_ID", "echo-test")
	t.Setenv("RPC_PLUGIN_SYSTEM_PLUGIN_GENERATION", "42")
	t.Setenv("RPC_PLUGIN_SYSTEM_AUTH_TOKEN_FILE", tokenFile)

	if err := run(); err == nil {
		t.Fatalf("run succeeded with socket directory that cannot host plugin log")
	}
}

func TestRunServesEchoPlugin(t *testing.T) {
	wantCfg := setValidRunEnv(t)
	served := false
	oldServeWithConfig := serveWithConfig
	serveWithConfig = func(cfg plugin.Config, core plugin.Core) error {
		served = true
		if !reflect.DeepEqual(cfg, wantCfg) {
			t.Fatalf("serve cfg = %#v, want %#v", cfg, wantCfg)
		}
		p, ok := core.(*echoPlugin)
		if !ok {
			t.Fatalf("serve core type = %T, want *echoPlugin", core)
		}
		if p.Version() != "0.1.0" {
			t.Fatalf("serve plugin version = %q, want 0.1.0", p.Version())
		}
		return nil
	}
	t.Cleanup(func() { serveWithConfig = oldServeWithConfig })

	if err := run(); err != nil {
		t.Fatalf("run returned error: %v", err)
	}
	if !served {
		t.Fatalf("run did not call serveWithConfig")
	}
}

func TestRunReportsServeFailure(t *testing.T) {
	setValidRunEnv(t)
	serveErr := errors.New("serve failed")
	oldServeWithConfig := serveWithConfig
	serveWithConfig = func(plugin.Config, plugin.Core) error { return serveErr }
	t.Cleanup(func() { serveWithConfig = oldServeWithConfig })

	if err := run(); !errors.Is(err, serveErr) {
		t.Fatalf("run error = %v, want %v", err, serveErr)
	}
}

func TestMainReturnsAfterSuccessfulRun(t *testing.T) {
	called := false
	oldRunPlugin := runPlugin
	runPlugin = func() error {
		called = true
		return nil
	}
	t.Cleanup(func() { runPlugin = oldRunPlugin })

	main()
	if !called {
		t.Fatalf("main did not call runPlugin")
	}
}

func TestMainPanicsOnRunFailure(t *testing.T) {
	runErr := errors.New("run failed")
	oldRunPlugin := runPlugin
	runPlugin = func() error { return runErr }
	t.Cleanup(func() { runPlugin = oldRunPlugin })

	defer func() {
		got := recover()
		if got == nil {
			t.Fatalf("main did not panic")
		}
		err, ok := got.(error)
		if !ok || !errors.Is(err, runErr) {
			t.Fatalf("panic = %v, want %v", got, runErr)
		}
	}()
	main()
}

func TestEchoPluginImplementsExpectedAPI(t *testing.T) {
	p := newTestEchoPlugin()

	if _, ok := any(p).(plugin.Core); !ok {
		t.Fatalf("echoPlugin does not implement plugin.Core")
	}
	if _, ok := any(p).(plugin.Echo); !ok {
		t.Fatalf("echoPlugin does not implement plugin.Echo")
	}
	if _, ok := any(p).(plugin.Sleep); !ok {
		t.Fatalf("echoPlugin does not implement plugin.Sleep")
	}
	if _, ok := any(p).(plugin.Crash); !ok {
		t.Fatalf("echoPlugin does not implement plugin.Crash")
	}
}

func TestEchoReturnsInputMessage(t *testing.T) {
	p := newTestEchoPlugin()
	var out plugin.EchoResponse

	if err := p.Echo(plugin.EchoRequest{Message: "hello from test"}, &out); err != nil {
		t.Fatalf("Echo returned error: %v", err)
	}
	if out.Message != "hello from test" {
		t.Fatalf("Echo message = %q, want %q", out.Message, "hello from test")
	}
}

func TestSleepWaitsForRequestedDuration(t *testing.T) {
	p := newTestEchoPlugin()
	requested := 10 * time.Millisecond
	started := time.Now()

	if err := p.Sleep(plugin.SleepRequest{Duration: requested}, &plugin.Empty{}); err != nil {
		t.Fatalf("Sleep returned error: %v", err)
	}
	if elapsed := time.Since(started); elapsed < requested {
		t.Fatalf("Sleep elapsed = %v, want at least %v", elapsed, requested)
	}
}

func TestCrashExitsWithRequestedCode(t *testing.T) {
	p := newTestEchoPlugin()
	exitCalls := make(chan int, 1)
	oldExitProcess := exitProcess
	exitProcess = func(code int) { exitCalls <- code }
	t.Cleanup(func() { exitProcess = oldExitProcess })

	if err := p.Crash(plugin.CrashRequest{Code: 7}, &plugin.Empty{}); err != nil {
		t.Fatalf("Crash returned error: %v", err)
	}
	select {
	case code := <-exitCalls:
		if code != 7 {
			t.Fatalf("Crash exit code = %d, want 7", code)
		}
	default:
		t.Fatalf("Crash did not call exitProcess")
	}
}

func TestShutdownWritesMarkerThenExitsZero(t *testing.T) {
	p := newTestEchoPlugin()
	marker := filepath.Join(t.TempDir(), "shutdown.marker")
	t.Setenv(testpluginapi.ShutdownMarkerEnv, marker)

	exitCalls := make(chan int, 1)
	oldExitProcess := exitProcess
	exitProcess = func(code int) { exitCalls <- code }
	t.Cleanup(func() { exitProcess = oldExitProcess })

	if err := p.Shutdown(plugin.Empty{}, &plugin.Empty{}); err != nil {
		t.Fatalf("Shutdown returned error: %v", err)
	}

	select {
	case code := <-exitCalls:
		if code != 0 {
			t.Fatalf("Shutdown exit code = %d, want 0", code)
		}
	case <-time.After(500 * time.Millisecond):
		t.Fatalf("Shutdown did not call exitProcess")
	}

	got, err := os.ReadFile(marker)
	if err != nil {
		t.Fatalf("read shutdown marker: %v", err)
	}
	if string(got) != "shutdown\n" {
		t.Fatalf("shutdown marker = %q, want %q", string(got), "shutdown\n")
	}
}

func TestTemplateCapabilitiesForEchoPlugin(t *testing.T) {
	p := newTestEchoPlugin()
	caps := make([]string, 0, 5)
	if _, ok := any(p).(plugin.Core); ok {
		caps = append(caps, "heartbeat", "shutdown")
	}
	if _, ok := any(p).(plugin.Echo); ok {
		caps = append(caps, "echo")
	}
	if _, ok := any(p).(plugin.Sleep); ok {
		caps = append(caps, "sleep")
	}
	if _, ok := any(p).(plugin.Crash); ok {
		caps = append(caps, "crash")
	}

	want := []string{"heartbeat", "shutdown", "echo", "sleep", "crash"}
	if !reflect.DeepEqual(caps, want) {
		t.Fatalf("capabilities = %#v, want %#v", caps, want)
	}
}
