package main

import (
	"errors"
	"net"
	"os"
	"os/exec"
	"reflect"
	"strconv"
	"strings"
	"testing"
	"time"

	plugin "rpc_plugin_system/pkg/plugin"
	"rpc_plugin_system/test/testpluginapi"
)

func newTestFailurePlugin(behavior testpluginapi.Env) *failurePlugin {
	return &failurePlugin{
		pluginID:     "runtime-id",
		version:      behavior.Version,
		generationID: 41,
		startedAt:    time.Now().Add(-2 * time.Second),
		behavior:     behavior,
	}
}

func TestIdentityUsesConfiguredOverrides(t *testing.T) {
	p := newTestFailurePlugin(testpluginapi.Env{PluginID: "behavior-id", Version: "1.2.3", GenerationOffset: 7})

	pluginID, generationID := p.Identity()
	if pluginID != "behavior-id" {
		t.Fatalf("plugin id = %q, want behavior-id", pluginID)
	}
	if generationID != 48 {
		t.Fatalf("generation id = %d, want 48", generationID)
	}
	if got := p.Version(); got != "1.2.3" {
		t.Fatalf("version = %q, want 1.2.3", got)
	}
}

func TestIdentityFallsBackToRuntimeID(t *testing.T) {
	p := newTestFailurePlugin(testpluginapi.Env{Version: "0.1.0"})

	pluginID, generationID := p.Identity()
	if pluginID != "runtime-id" {
		t.Fatalf("plugin id = %q, want runtime-id", pluginID)
	}
	if generationID != 41 {
		t.Fatalf("generation id = %d, want 41", generationID)
	}
}

func TestAuthAttemptHonorsFailureConfig(t *testing.T) {
	p := newTestFailurePlugin(testpluginapi.Env{})
	if err := p.OnAuthAttempt(plugin.AuthRequest{Token: "token"}); err != nil {
		t.Fatalf("auth without failure config returned error: %v", err)
	}

	p.behavior.FailAuth = true
	if err := p.OnAuthAttempt(plugin.AuthRequest{Token: "token"}); err == nil || !strings.Contains(err.Error(), "auth failure requested") {
		t.Fatalf("auth failure error = %v, want requested failure", err)
	}
}

func TestServeListenerStoresAndOptionallyClosesListener(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	defer listener.Close()

	p := newTestFailurePlugin(testpluginapi.Env{})
	p.OnServeListener(listener)
	if p.listener != listener {
		t.Fatalf("listener was not stored")
	}

	closingListener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen closing listener: %v", err)
	}
	p = newTestFailurePlugin(testpluginapi.Env{CloseOnAccept: true})
	p.OnServeListener(closingListener)
	_, err = closingListener.Accept()
	if !errors.Is(err, net.ErrClosed) {
		t.Fatalf("Accept after CloseOnAccept error = %v, want net.ErrClosed", err)
	}
}

func TestHeartbeatReportsConfiguredStatusIdentityAndErrors(t *testing.T) {
	p := newTestFailurePlugin(testpluginapi.Env{
		PluginID:         "heartbeat-id",
		Version:          "2.0.0",
		GenerationOffset: 3,
		HeartbeatStatus:  testpluginapi.StatusDegraded,
		HeartbeatErrors:  1,
		HeartbeatDelayMS: 1,
	})

	var out plugin.HeartbeatResponse
	if err := p.Heartbeat(plugin.Empty{}, &out); err == nil || !strings.Contains(err.Error(), "heartbeat failure requested") {
		t.Fatalf("first heartbeat error = %v, want requested failure", err)
	}

	if err := p.Heartbeat(plugin.Empty{}, &out); err != nil {
		t.Fatalf("second heartbeat returned error: %v", err)
	}
	if out.PluginID != "heartbeat-id" || out.Version != "2.0.0" || out.GenerationID != 44 || out.Status != plugin.StatusDegraded {
		t.Fatalf("heartbeat response = %+v, want configured identity/version/generation/status", out)
	}
	if out.UptimeSeconds < 0 {
		t.Fatalf("uptime = %d, want non-negative", out.UptimeSeconds)
	}
}

func TestEchoMirrorsMessageAndCanCloseListener(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}

	p := newTestFailurePlugin(testpluginapi.Env{CloseOnEcho: true})
	p.listener = listener
	var out plugin.EchoResponse
	if err := p.Echo(plugin.EchoRequest{Message: "hello"}, &out); err != nil {
		t.Fatalf("Echo returned error: %v", err)
	}
	if out.Message != "hello" {
		t.Fatalf("echo message = %q, want hello", out.Message)
	}
	_, err = listener.Accept()
	if !errors.Is(err, net.ErrClosed) {
		t.Fatalf("Accept after CloseOnEcho error = %v, want net.ErrClosed", err)
	}
}

func TestSleepUsesConfiguredScale(t *testing.T) {
	p := newTestFailurePlugin(testpluginapi.Env{SleepScale: 2})

	start := time.Now()
	if err := p.Sleep(plugin.SleepRequest{Duration: 2 * time.Millisecond}, &plugin.Empty{}); err != nil {
		t.Fatalf("Sleep returned error: %v", err)
	}
	if elapsed := time.Since(start); elapsed < 3*time.Millisecond {
		t.Fatalf("sleep elapsed = %s, want scaled delay", elapsed)
	}

	p = newTestFailurePlugin(testpluginapi.Env{SleepScale: 0})
	start = time.Now()
	if err := p.Sleep(plugin.SleepRequest{Duration: time.Millisecond}, &plugin.Empty{}); err != nil {
		t.Fatalf("Sleep with default scale returned error: %v", err)
	}
	if elapsed := time.Since(start); elapsed < time.Millisecond {
		t.Fatalf("default-scale sleep elapsed = %s, want unscaled delay", elapsed)
	}
}

func TestCapabilitiesModes(t *testing.T) {
	tests := []struct {
		name string
		mode string
		want []string
	}{
		{name: "normal", mode: "", want: []string{"heartbeat", "shutdown", "echo", "sleep", "crash"}},
		{name: "error", mode: "error", want: []string{"__capabilities_error__"}},
		{name: "empty", mode: "empty", want: nil},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := newTestFailurePlugin(testpluginapi.Env{CapabilitiesMode: tt.mode})
			if got := p.Capabilities(); !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("Capabilities() = %#v, want %#v", got, tt.want)
			}
		})
	}
}

func TestRunReportsMissingStartupConfig(t *testing.T) {
	t.Setenv("RPC_PLUGIN_SYSTEM_PLUGIN_SOCKET", "")
	t.Setenv("RPC_PLUGIN_SYSTEM_PLUGIN_ID", "")
	t.Setenv("RPC_PLUGIN_SYSTEM_PLUGIN_GENERATION", "")
	t.Setenv("RPC_PLUGIN_SYSTEM_AUTH_TOKEN_FILE", "")
	t.Setenv(testpluginapi.BehaviorConfigEnv, "")

	if err := run(); err == nil || !strings.Contains(err.Error(), "RPC_PLUGIN_SYSTEM_PLUGIN_SOCKET") {
		t.Fatalf("run missing config error = %v, want missing socket env", err)
	}
}

func setRunEnv(t *testing.T, socketPath string, authFile string, behaviorFile string) {
	t.Helper()
	t.Setenv("RPC_PLUGIN_SYSTEM_PLUGIN_SOCKET", socketPath)
	t.Setenv("RPC_PLUGIN_SYSTEM_PLUGIN_ID", "runtime-id")
	t.Setenv("RPC_PLUGIN_SYSTEM_PLUGIN_GENERATION", "12")
	t.Setenv("RPC_PLUGIN_SYSTEM_AUTH_TOKEN_FILE", authFile)
	t.Setenv(testpluginapi.BehaviorConfigEnv, behaviorFile)
}

func TestRunReportsLoggerFailure(t *testing.T) {
	dir := t.TempDir()
	authFile := dir + "/auth-token"
	if err := os.WriteFile(authFile, []byte("secret-token"), 0o600); err != nil {
		t.Fatalf("write auth token: %v", err)
	}
	setRunEnv(t, dir+"/missing/plugin.sock", authFile, "")

	if err := run(); err == nil || !strings.Contains(err.Error(), "open plugin event log") {
		t.Fatalf("run logger error = %v, want plugin event log failure", err)
	}
}

func TestRunReportsServeFailure(t *testing.T) {
	dir := t.TempDir()
	authFile := dir + "/auth-token"
	if err := os.WriteFile(authFile, []byte("secret-token"), 0o600); err != nil {
		t.Fatalf("write auth token: %v", err)
	}
	setRunEnv(t, dir+"/"+strings.Repeat("x", 200)+".sock", authFile, "")

	if err := run(); err == nil || !strings.Contains(err.Error(), "listen unix socket") {
		t.Fatalf("run serve error = %v, want listen failure", err)
	}
}

func configureClosingRun(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	authFile := dir + "/auth-token"
	if err := os.WriteFile(authFile, []byte("secret-token"), 0o600); err != nil {
		t.Fatalf("write auth token: %v", err)
	}
	behaviorFile := dir + "/behavior.json"
	if err := testpluginapi.WriteConfig(behaviorFile, testpluginapi.Env{Version: "9.9.9", CloseOnAccept: true}); err != nil {
		t.Fatalf("write behavior config: %v", err)
	}
	setRunEnv(t, dir+"/plugin.sock", authFile, behaviorFile)
	return dir
}

func TestRunLoadsBehaviorConfigAndStopsWhenConfiguredListenerCloses(t *testing.T) {
	dir := configureClosingRun(t)

	if err := run(); err != nil {
		t.Fatalf("run returned error: %v", err)
	}
	if _, err := os.Stat(dir + "/plugin-events.jsonl"); err != nil {
		t.Fatalf("plugin log was not written: %v", err)
	}
}

func TestMainReturnsAfterConfiguredListenerCloses(t *testing.T) {
	configureClosingRun(t)
	main()
}

func TestMainPanicsWhenRunFails(t *testing.T) {
	t.Setenv("RPC_PLUGIN_SYSTEM_PLUGIN_SOCKET", "")
	t.Setenv("RPC_PLUGIN_SYSTEM_PLUGIN_ID", "")
	t.Setenv("RPC_PLUGIN_SYSTEM_PLUGIN_GENERATION", "")
	t.Setenv("RPC_PLUGIN_SYSTEM_AUTH_TOKEN_FILE", "")
	t.Setenv(testpluginapi.BehaviorConfigEnv, "")
	defer func() {
		if recovered := recover(); recovered == nil {
			t.Fatalf("main did not panic on missing config")
		}
	}()
	main()
}

func TestCrashExitsWithRequestedCode(t *testing.T) {
	if os.Getenv("RPCPLUGIN_FAILURE_TEST_HELPER") == "crash" {
		code, _ := strconv.Atoi(os.Getenv("RPCPLUGIN_FAILURE_TEST_EXIT_CODE"))
		_ = newTestFailurePlugin(testpluginapi.Env{}).Crash(plugin.CrashRequest{Code: code}, &plugin.Empty{})
		return
	}

	cmd := exec.Command(os.Args[0], "-test.run=^TestCrashExitsWithRequestedCode$")
	cmd.Env = append(os.Environ(), "RPCPLUGIN_FAILURE_TEST_HELPER=crash", "RPCPLUGIN_FAILURE_TEST_EXIT_CODE=17")
	err := cmd.Run()
	if exitErr, ok := err.(*exec.ExitError); !ok || exitErr.ExitCode() != 17 {
		t.Fatalf("Crash subprocess error = %v, want exit code 17", err)
	}
}

func TestEchoCrashConfigExitsWithCodeNine(t *testing.T) {
	if os.Getenv("RPCPLUGIN_FAILURE_TEST_HELPER") == "echo-crash" {
		var out plugin.EchoResponse
		_ = newTestFailurePlugin(testpluginapi.Env{CrashOnEcho: true}).Echo(plugin.EchoRequest{Message: "boom"}, &out)
		return
	}

	cmd := exec.Command(os.Args[0], "-test.run=^TestEchoCrashConfigExitsWithCodeNine$")
	cmd.Env = append(os.Environ(), "RPCPLUGIN_FAILURE_TEST_HELPER=echo-crash")
	err := cmd.Run()
	if exitErr, ok := err.(*exec.ExitError); !ok || exitErr.ExitCode() != 9 {
		t.Fatalf("Echo crash subprocess error = %v, want exit code 9", err)
	}
}

func TestShutdownWritesMarkerThenExits(t *testing.T) {
	if os.Getenv("RPCPLUGIN_FAILURE_TEST_HELPER") == "shutdown" {
		_ = newTestFailurePlugin(testpluginapi.Env{ShutdownDelayMS: 1}).Shutdown(plugin.Empty{}, &plugin.Empty{})
		select {}
	}

	marker := t.TempDir() + "/shutdown.marker"
	cmd := exec.Command(os.Args[0], "-test.run=^TestShutdownWritesMarkerThenExits$")
	cmd.Env = append(os.Environ(), "RPCPLUGIN_FAILURE_TEST_HELPER=shutdown", testpluginapi.ShutdownMarkerEnv+"="+marker)
	if err := cmd.Run(); err != nil {
		t.Fatalf("Shutdown subprocess error = %v, want clean exit", err)
	}
	data, err := os.ReadFile(marker)
	if err != nil {
		t.Fatalf("read shutdown marker: %v", err)
	}
	if string(data) != "shutdown\n" {
		t.Fatalf("shutdown marker = %q, want shutdown\\n", string(data))
	}
}
