package plugin

import (
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/rpc"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"
)

type sdkTestCore struct {
	version      string
	pluginID     string
	generationID uint64
	capabilities []string
	authErr      error
	heartbeatErr error
	shutdownErr  error
	echoErr      error
	sleepErr     error
	crashErr     error
	heartbeats   int
	shutdowns    int
	echoes       []string
	sleeps       []time.Duration
	crashes      []int
}

func (c *sdkTestCore) Version() string {
	if c.version == "" {
		return "1.2.3"
	}
	return c.version
}

func (c *sdkTestCore) Heartbeat(_ Empty, out *HeartbeatResponse) error {
	c.heartbeats++
	if c.heartbeatErr != nil {
		return c.heartbeatErr
	}
	*out = HeartbeatResponse{PluginID: c.pluginID, Version: c.Version(), GenerationID: c.generationID, Status: StatusHealthy}
	return nil
}

func (c *sdkTestCore) Shutdown(_ Empty, _ *Empty) error {
	c.shutdowns++
	return c.shutdownErr
}

func (c *sdkTestCore) Echo(in EchoRequest, out *EchoResponse) error {
	c.echoes = append(c.echoes, in.Message)
	if c.echoErr != nil {
		return c.echoErr
	}
	out.Message = in.Message
	return nil
}

func (c *sdkTestCore) Sleep(in SleepRequest, _ *Empty) error {
	c.sleeps = append(c.sleeps, in.Duration)
	return c.sleepErr
}

func (c *sdkTestCore) Crash(in CrashRequest, _ *Empty) error {
	c.crashes = append(c.crashes, in.Code)
	return c.crashErr
}

func (c *sdkTestCore) Capabilities() []string { return c.capabilities }

func (c *sdkTestCore) OnAuthAttempt(_ AuthRequest) error { return c.authErr }

func (c *sdkTestCore) Identity() (string, uint64) { return c.pluginID, c.generationID }

type sdkBaseCore struct{}

func (sdkBaseCore) Version() string { return "base" }
func (sdkBaseCore) Heartbeat(_ Empty, out *HeartbeatResponse) error {
	out.Status = StatusHealthy
	return nil
}
func (sdkBaseCore) Shutdown(_ Empty, _ *Empty) error { return nil }

type sdkEchoCore struct{ sdkBaseCore }

func (sdkEchoCore) Echo(in EchoRequest, out *EchoResponse) error {
	out.Message = in.Message
	return nil
}

type sdkSleepCore struct{ sdkBaseCore }

func (sdkSleepCore) Sleep(_ SleepRequest, _ *Empty) error { return nil }

type sdkCrashCore struct{ sdkBaseCore }

func (sdkCrashCore) Crash(_ CrashRequest, _ *Empty) error { return nil }

type closeOnServeCore struct{ sdkBaseCore }

func (closeOnServeCore) OnServeListener(l net.Listener) { _ = l.Close() }

type observedServeCore struct {
	sdkBaseCore
	served chan struct{}
}

func (c observedServeCore) OnServeListener(l net.Listener) {
	go func() {
		conn, err := net.Dial("unix", l.Addr().String())
		if err == nil {
			_ = conn.Close()
		}
		<-c.served
		_ = l.Close()
	}()
}

func TestLoadConfigFromEnvMissingVarsAreExplicit(t *testing.T) {
	for _, name := range []string{
		"RPC_PLUGIN_SYSTEM_PLUGIN_SOCKET",
		"RPC_PLUGIN_SYSTEM_PLUGIN_ID",
		"RPC_PLUGIN_SYSTEM_PLUGIN_GENERATION",
		"RPC_PLUGIN_SYSTEM_AUTH_TOKEN_FILE",
	} {
		t.Run(name, func(t *testing.T) {
			clearPluginEnv(t)
			setValidPluginEnv(t)
			os.Unsetenv(name)

			_, err := LoadConfigFromEnv()
			if err == nil {
				t.Fatal("expected missing env error")
			}
			var missing ErrMissingEnv
			if !errors.As(err, &missing) {
				t.Fatalf("expected ErrMissingEnv, got %T: %v", err, err)
			}
			if missing.Name != name {
				t.Fatalf("missing env name = %q want %q", missing.Name, name)
			}
		})
	}
}

func TestLoadConfigFromEnvInvalidGenerationNamesVariable(t *testing.T) {
	clearPluginEnv(t)
	setValidPluginEnv(t)
	if err := os.Setenv("RPC_PLUGIN_SYSTEM_PLUGIN_GENERATION", "abc"); err != nil {
		t.Fatalf("set generation: %v", err)
	}

	_, err := LoadConfigFromEnv()
	if err == nil {
		t.Fatal("expected parse error")
	}
	if !strings.Contains(err.Error(), "RPC_PLUGIN_SYSTEM_PLUGIN_GENERATION") {
		t.Fatalf("parse error should name variable, got: %v", err)
	}
}

func TestNewTemplateBuildsMinimalHealthyPlugin(t *testing.T) {
	cfg := Config{PluginID: "echo", GenerationID: 7}
	p := NewTemplate(cfg, "1.2.3")

	if p.PluginID != "echo" || p.GenerationID != 7 || p.Version() != "1.2.3" || p.StartedAt.IsZero() {
		t.Fatalf("unexpected template plugin: %+v", p)
	}

	var hb HeartbeatResponse
	if err := p.Heartbeat(Empty{}, &hb); err != nil {
		t.Fatalf("heartbeat: %v", err)
	}
	if hb.PluginID != "echo" || hb.GenerationID != 7 || hb.Version != "1.2.3" || hb.Status != StatusHealthy {
		t.Fatalf("unexpected heartbeat: %+v", hb)
	}
}

func TestServerRejectsNonAuthMethodsBeforeAuth(t *testing.T) {
	srv := &server{core: NewTemplate(Config{PluginID: "echo", GenerationID: 7}, "1.2.3"), cfg: Config{PluginID: "echo", GenerationID: 7, AuthToken: "secret"}}

	var caps CapabilitiesResponse
	if err := srv.Capabilities(Empty{}, &caps); !errors.Is(err, os.ErrPermission) {
		t.Fatalf("capabilities before auth err = %v, want permission", err)
	}

	var hb HeartbeatResponse
	if err := srv.Heartbeat(Empty{}, &hb); !errors.Is(err, os.ErrPermission) {
		t.Fatalf("heartbeat before auth err = %v, want permission", err)
	}

	var authOut AuthResponse
	if err := srv.Auth(AuthRequest{Token: "secret"}, &authOut); err != nil {
		t.Fatalf("auth: %v", err)
	}

	if err := srv.Capabilities(Empty{}, &caps); err != nil {
		t.Fatalf("capabilities after auth: %v", err)
	}
	if caps.PluginID != "echo" || caps.GenerationID != 7 || caps.Version != "1.2.3" {
		t.Fatalf("unexpected capabilities response: %+v", caps)
	}

	if err := srv.Heartbeat(Empty{}, &hb); err != nil {
		t.Fatalf("heartbeat after auth: %v", err)
	}
}

func TestServerRejectsBadTokenBeforeSuccessfulAuth(t *testing.T) {
	srv := &server{core: NewTemplate(Config{PluginID: "echo", GenerationID: 7}, "1.2.3"), cfg: Config{PluginID: "echo", GenerationID: 7, AuthToken: "secret"}}

	var authOut AuthResponse
	if err := srv.Auth(AuthRequest{Token: "wrong"}, &authOut); !errors.Is(err, os.ErrPermission) {
		t.Fatalf("bad auth token err = %v, want permission", err)
	}

	var hb HeartbeatResponse
	if err := srv.Heartbeat(Empty{}, &hb); !errors.Is(err, os.ErrPermission) {
		t.Fatalf("heartbeat after bad auth err = %v, want permission", err)
	}

	if err := srv.Auth(AuthRequest{Token: "secret"}, &authOut); err != nil {
		t.Fatalf("auth after bad token: %v", err)
	}
	if authOut.PluginID != "echo" || authOut.GenerationID != 7 || authOut.Version != "1.2.3" {
		t.Fatalf("unexpected auth response: %+v", authOut)
	}
}

func TestServerRejectsAuthReplayAndKeepsEstablishedSession(t *testing.T) {
	srv := &server{core: NewTemplate(Config{PluginID: "echo", GenerationID: 7}, "1.2.3"), cfg: Config{PluginID: "echo", GenerationID: 7, AuthToken: "secret"}}

	var authOut AuthResponse
	if err := srv.Auth(AuthRequest{Token: "secret"}, &authOut); err != nil {
		t.Fatalf("first auth: %v", err)
	}
	if authOut.PluginID != "echo" || authOut.GenerationID != 7 || authOut.Version != "1.2.3" {
		t.Fatalf("unexpected first auth response: %+v", authOut)
	}

	var replayOut AuthResponse
	if err := srv.Auth(AuthRequest{Token: "secret"}, &replayOut); !errors.Is(err, os.ErrPermission) {
		t.Fatalf("replayed auth err = %v, want permission", err)
	}

	var caps CapabilitiesResponse
	if err := srv.Capabilities(Empty{}, &caps); err != nil {
		t.Fatalf("capabilities after replay rejection: %v", err)
	}
	if caps.PluginID != "echo" || caps.GenerationID != 7 || caps.Version != "1.2.3" {
		t.Fatalf("unexpected capabilities response: %+v", caps)
	}

	var hb HeartbeatResponse
	if err := srv.Heartbeat(Empty{}, &hb); err != nil {
		t.Fatalf("heartbeat after replay rejection: %v", err)
	}
}

func clearPluginEnv(t *testing.T) {
	t.Helper()
	for _, name := range []string{
		"RPC_PLUGIN_SYSTEM_PLUGIN_SOCKET",
		"RPC_PLUGIN_SYSTEM_PLUGIN_ID",
		"RPC_PLUGIN_SYSTEM_PLUGIN_GENERATION",
		"RPC_PLUGIN_SYSTEM_AUTH_TOKEN_FILE",
	} {
		if err := os.Unsetenv(name); err != nil {
			t.Fatalf("unset %s: %v", name, err)
		}
	}
}

func setValidPluginEnv(t *testing.T) {
	t.Helper()
	dir := t.TempDir()
	authFile := filepath.Join(dir, "token")
	if err := os.WriteFile(authFile, []byte("secret-token"), 0o600); err != nil {
		t.Fatalf("write auth file: %v", err)
	}
	values := map[string]string{
		"RPC_PLUGIN_SYSTEM_PLUGIN_SOCKET":     filepath.Join(dir, "plugin.sock"),
		"RPC_PLUGIN_SYSTEM_PLUGIN_ID":         "echo",
		"RPC_PLUGIN_SYSTEM_PLUGIN_GENERATION": "7",
		"RPC_PLUGIN_SYSTEM_AUTH_TOKEN_FILE":   authFile,
	}
	for name, value := range values {
		if err := os.Setenv(name, value); err != nil {
			t.Fatalf("set %s: %v", name, err)
		}
	}
}

func TestErrMissingEnvErrorNamesVariable(t *testing.T) {
	err := ErrMissingEnv{Name: "RPC_PLUGIN_SYSTEM_PLUGIN_ID"}
	if got, want := err.Error(), "RPC_PLUGIN_SYSTEM_PLUGIN_ID is required for plugin startup"; got != want {
		t.Fatalf("Error() = %q want %q", got, want)
	}
}

func TestLoadConfigFromEnvReadsValidConfig(t *testing.T) {
	clearPluginEnv(t)
	setValidPluginEnv(t)

	cfg, err := LoadConfigFromEnv()
	if err != nil {
		t.Fatalf("LoadConfigFromEnv: %v", err)
	}
	if cfg.SocketPath == "" || cfg.PluginID != "echo" || cfg.GenerationID != 7 || cfg.AuthToken != "secret-token" {
		t.Fatalf("unexpected config: %+v", cfg)
	}
}

func TestLoadConfigFromEnvGenerationOverflowNamesVariable(t *testing.T) {
	clearPluginEnv(t)
	setValidPluginEnv(t)
	if err := os.Setenv("RPC_PLUGIN_SYSTEM_PLUGIN_GENERATION", "18446744073709551616"); err != nil {
		t.Fatalf("set generation: %v", err)
	}

	_, err := LoadConfigFromEnv()
	if err == nil || !strings.Contains(err.Error(), "RPC_PLUGIN_SYSTEM_PLUGIN_GENERATION") {
		t.Fatalf("expected generation parse error naming variable, got: %v", err)
	}
}

func TestLoadConfigFromEnvAuthTokenReadErrorNamesPath(t *testing.T) {
	clearPluginEnv(t)
	setValidPluginEnv(t)
	missing := filepath.Join(t.TempDir(), "missing-token")
	if err := os.Setenv("RPC_PLUGIN_SYSTEM_AUTH_TOKEN_FILE", missing); err != nil {
		t.Fatalf("set token file: %v", err)
	}

	_, err := LoadConfigFromEnv()
	if err == nil {
		t.Fatal("expected read error")
	}
	if !strings.Contains(err.Error(), "RPC_PLUGIN_SYSTEM_AUTH_TOKEN_FILE") || !strings.Contains(err.Error(), missing) {
		t.Fatalf("read error should name variable and path, got: %v", err)
	}
}

func TestDetectCapabilitiesFromInterfaces(t *testing.T) {
	tests := []struct {
		name string
		core Core
		want []string
	}{
		{name: "base", core: sdkBaseCore{}, want: []string{"heartbeat", "shutdown"}},
		{name: "echo", core: sdkEchoCore{}, want: []string{"heartbeat", "shutdown", "echo"}},
		{name: "sleep", core: sdkSleepCore{}, want: []string{"heartbeat", "shutdown", "sleep"}},
		{name: "crash", core: sdkCrashCore{}, want: []string{"heartbeat", "shutdown", "crash"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := detectCapabilities(tt.core); !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("detectCapabilities() = %#v want %#v", got, tt.want)
			}
		})
	}
}

func TestDetectCapabilitiesUsesProviderVerbatim(t *testing.T) {
	core := &sdkTestCore{capabilities: []string{"custom", "expensive"}}
	if got := detectCapabilities(core); !reflect.DeepEqual(got, core.capabilities) {
		t.Fatalf("provider capabilities = %#v want %#v", got, core.capabilities)
	}
}

func TestLoggerDefaultsAndNilSafety(t *testing.T) {
	var nilLogger *Logger
	if nilLogger.Path() != "" {
		t.Fatalf("nil logger path = %q want empty", nilLogger.Path())
	}
	if err := nilLogger.Close(); err != nil {
		t.Fatalf("nil logger close: %v", err)
	}
	if err := nilLogger.Event(LogEvent{}); err != nil {
		t.Fatalf("nil logger event: %v", err)
	}

	dir := t.TempDir()
	cfg := Config{SocketPath: filepath.Join(dir, "plugin.sock"), PluginID: "plugin-a", GenerationID: 42}
	logger, err := NewLogger(cfg)
	if err != nil {
		t.Fatalf("NewLogger: %v", err)
	}
	if got, want := logger.Path(), filepath.Join(dir, DefaultPluginLogFileName); got != want {
		t.Fatalf("logger path = %q want %q", got, want)
	}
	if err := logger.Event(LogEvent{Event: EventPluginRequestHandled, Method: MethodEcho}); err != nil {
		t.Fatalf("write event: %v", err)
	}
	if err := logger.Close(); err != nil {
		t.Fatalf("close logger: %v", err)
	}
	if err := logger.Close(); err == nil {
		t.Fatal("second close should surface underlying close error")
	}

	data, err := os.ReadFile(logger.Path())
	if err != nil {
		t.Fatalf("read log: %v", err)
	}
	var event LogEvent
	if err := json.Unmarshal([]byte(strings.TrimSpace(string(data))), &event); err != nil {
		t.Fatalf("decode event %q: %v", data, err)
	}
	if event.Component != LogComponentPlugin || event.PluginID != "plugin-a" || event.GenerationID != 42 || event.PID == 0 || event.Level != LogLevelInfo || event.Method != MethodEcho {
		t.Fatalf("event defaults not applied: %+v", event)
	}
}

func TestNewLoggerReportsOpenFailure(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "not-a-dir")
	if err := os.WriteFile(file, []byte("x"), 0o600); err != nil {
		t.Fatalf("write marker: %v", err)
	}
	_, err := NewLogger(Config{SocketPath: filepath.Join(file, "plugin.sock"), PluginID: "p", GenerationID: 1})
	if err == nil || !strings.Contains(err.Error(), "open plugin event log") {
		t.Fatalf("expected logger open failure, got: %v", err)
	}
}

func TestServerAuthHookRejectsBeforeTokenUse(t *testing.T) {
	core := &sdkTestCore{authErr: fmt.Errorf("nope")}
	srv := &server{core: core, cfg: Config{PluginID: "cfg", GenerationID: 1, AuthToken: "secret"}}
	var out AuthResponse
	if err := srv.Auth(AuthRequest{Token: "secret"}, &out); err == nil || !strings.Contains(err.Error(), "nope") {
		t.Fatalf("expected hook rejection, got: %v", err)
	}
	core.authErr = nil
	if err := srv.Auth(AuthRequest{Token: "secret"}, &out); err != nil {
		t.Fatalf("auth after hook rejection: %v", err)
	}
}

func TestServerAuthAndCapabilitiesUseIdentityHook(t *testing.T) {
	core := &sdkTestCore{pluginID: "identity", generationID: 99, capabilities: []string{"custom"}}
	srv := &server{core: core, cfg: Config{PluginID: "cfg", GenerationID: 1, AuthToken: "secret"}, capabilities: detectCapabilities(core)}
	var authOut AuthResponse
	if err := srv.Auth(AuthRequest{Token: "secret"}, &authOut); err != nil {
		t.Fatalf("auth: %v", err)
	}
	if authOut.PluginID != "identity" || authOut.GenerationID != 99 || authOut.Version != "1.2.3" {
		t.Fatalf("unexpected auth response: %+v", authOut)
	}
	var caps CapabilitiesResponse
	if err := srv.Capabilities(Empty{}, &caps); err != nil {
		t.Fatalf("capabilities: %v", err)
	}
	if caps.PluginID != "identity" || caps.GenerationID != 99 || !reflect.DeepEqual(caps.Capabilities, []string{"custom"}) {
		t.Fatalf("unexpected capabilities: %+v", caps)
	}
}

func TestServerOptionalRPCMethodsDispatchAndReturnErrors(t *testing.T) {
	sentinel := fmt.Errorf("boom")
	core := &sdkTestCore{pluginID: "p", generationID: 2}
	srv := authenticatedServer(core)

	var echo EchoResponse
	if err := srv.Echo(EchoRequest{Message: "hello"}, &echo); err != nil || echo.Message != "hello" || !reflect.DeepEqual(core.echoes, []string{"hello"}) {
		t.Fatalf("echo err=%v out=%+v calls=%#v", err, echo, core.echoes)
	}
	if err := srv.Sleep(SleepRequest{Duration: 5 * time.Millisecond}, &Empty{}); err != nil || !reflect.DeepEqual(core.sleeps, []time.Duration{5 * time.Millisecond}) {
		t.Fatalf("sleep err=%v calls=%#v", err, core.sleeps)
	}
	if err := srv.Crash(CrashRequest{Code: 9}, &Empty{}); err != nil || !reflect.DeepEqual(core.crashes, []int{9}) {
		t.Fatalf("crash err=%v calls=%#v", err, core.crashes)
	}
	if err := srv.Shutdown(Empty{}, &Empty{}); err != nil || core.shutdowns != 1 {
		t.Fatalf("shutdown err=%v calls=%d", err, core.shutdowns)
	}

	core.echoErr = sentinel
	if err := srv.Echo(EchoRequest{Message: "bad"}, &EchoResponse{}); !errors.Is(err, sentinel) {
		t.Fatalf("echo error = %v want sentinel", err)
	}
	core.sleepErr = sentinel
	if err := srv.Sleep(SleepRequest{}, &Empty{}); !errors.Is(err, sentinel) {
		t.Fatalf("sleep error = %v want sentinel", err)
	}
	core.crashErr = sentinel
	if err := srv.Crash(CrashRequest{}, &Empty{}); !errors.Is(err, sentinel) {
		t.Fatalf("crash error = %v want sentinel", err)
	}
	core.shutdownErr = sentinel
	if err := srv.Shutdown(Empty{}, &Empty{}); !errors.Is(err, sentinel) {
		t.Fatalf("shutdown error = %v want sentinel", err)
	}
}

func TestServerOptionalRPCMethodsReturnShutdownWhenUnsupported(t *testing.T) {
	srv := authenticatedServer(sdkBaseCore{})
	if err := srv.Echo(EchoRequest{}, &EchoResponse{}); !errors.Is(err, rpc.ErrShutdown) {
		t.Fatalf("unsupported echo err = %v want rpc.ErrShutdown", err)
	}
	if err := srv.Sleep(SleepRequest{}, &Empty{}); !errors.Is(err, rpc.ErrShutdown) {
		t.Fatalf("unsupported sleep err = %v want rpc.ErrShutdown", err)
	}
	if err := srv.Crash(CrashRequest{}, &Empty{}); !errors.Is(err, rpc.ErrShutdown) {
		t.Fatalf("unsupported crash err = %v want rpc.ErrShutdown", err)
	}
}

func TestServerHeartbeatReturnsCoreError(t *testing.T) {
	sentinel := fmt.Errorf("heartbeat failed")
	core := &sdkTestCore{heartbeatErr: sentinel}
	srv := authenticatedServer(core)
	if err := srv.Heartbeat(Empty{}, &HeartbeatResponse{}); !errors.Is(err, sentinel) {
		t.Fatalf("heartbeat error = %v want sentinel", err)
	}
	if core.heartbeats != 1 {
		t.Fatalf("heartbeat calls = %d want 1", core.heartbeats)
	}
}

func TestServerLogsRPCSuccessFailureAndPreAuthRejection(t *testing.T) {
	dir := t.TempDir()
	logger, err := NewLogger(Config{SocketPath: filepath.Join(dir, "plugin.sock"), PluginID: "p", GenerationID: 3})
	if err != nil {
		t.Fatalf("NewLogger: %v", err)
	}
	defer logger.Close()
	core := &sdkTestCore{pluginID: "p", generationID: 3, echoErr: fmt.Errorf("echo failed")}
	srv := &server{core: core, cfg: Config{PluginID: "p", GenerationID: 3, AuthToken: "secret"}, logger: logger}

	if err := srv.Echo(EchoRequest{}, &EchoResponse{}); !errors.Is(err, os.ErrPermission) {
		t.Fatalf("pre-auth echo err = %v want permission", err)
	}
	var authOut AuthResponse
	if err := srv.Auth(AuthRequest{Token: "secret"}, &authOut); err != nil {
		t.Fatalf("auth: %v", err)
	}
	if err := srv.Heartbeat(Empty{}, &HeartbeatResponse{}); err != nil {
		t.Fatalf("heartbeat: %v", err)
	}
	if err := srv.Echo(EchoRequest{}, &EchoResponse{}); err == nil || !strings.Contains(err.Error(), "echo failed") {
		t.Fatalf("echo failure = %v", err)
	}
	if err := logger.Close(); err != nil {
		t.Fatalf("close logger: %v", err)
	}

	data, err := os.ReadFile(logger.Path())
	if err != nil {
		t.Fatalf("read log: %v", err)
	}
	logText := string(data)
	for _, want := range []string{EventPluginAuthRejected, EventPluginAuthAttempt, EventPluginAuthAccepted, EventPluginRequestHandled, EventPluginRequestFailed} {
		if !strings.Contains(logText, want) {
			t.Fatalf("log missing %q in:\n%s", want, logText)
		}
	}
}

func TestServeAndServeWithConfigFailureAndClosePaths(t *testing.T) {
	clearPluginEnv(t)
	if err := Serve(sdkBaseCore{}); err == nil || !strings.Contains(err.Error(), "load plugin config from env") {
		t.Fatalf("Serve missing env err = %v", err)
	}
	if err := ServeWithConfig(Config{}, nil); err == nil || !strings.Contains(err.Error(), "plugin core is required") {
		t.Fatalf("ServeWithConfig nil core err = %v", err)
	}

	dir := t.TempDir()
	badSocketPath := filepath.Join(dir, "bad\x00sock")
	if err := ServeWithConfig(Config{SocketPath: badSocketPath, PluginID: "p", GenerationID: 1}, sdkBaseCore{}); err == nil || !strings.Contains(err.Error(), "listen unix socket") {
		t.Fatalf("ServeWithConfig listen err = %v", err)
	}

	if err := ServeWithConfig(Config{SocketPath: filepath.Join(t.TempDir(), "plugin.sock"), PluginID: "p", GenerationID: 1}, closeOnServeCore{}); err != nil {
		t.Fatalf("ServeWithConfig close listener path: %v", err)
	}
}

func TestServeReturnsServeWithConfigResult(t *testing.T) {
	clearPluginEnv(t)
	dir := t.TempDir()
	authFile := filepath.Join(dir, "token")
	if err := os.WriteFile(authFile, []byte("secret"), 0o600); err != nil {
		t.Fatalf("write token: %v", err)
	}
	for name, value := range map[string]string{
		"RPC_PLUGIN_SYSTEM_PLUGIN_SOCKET":     filepath.Join(dir, "plugin.sock"),
		"RPC_PLUGIN_SYSTEM_PLUGIN_ID":         "p",
		"RPC_PLUGIN_SYSTEM_PLUGIN_GENERATION": "1",
		"RPC_PLUGIN_SYSTEM_AUTH_TOKEN_FILE":   authFile,
	} {
		if err := os.Setenv(name, value); err != nil {
			t.Fatalf("set %s: %v", name, err)
		}
	}
	if err := Serve(closeOnServeCore{}); err != nil {
		t.Fatalf("Serve: %v", err)
	}
}

func TestServeWithConfigAcceptsOneConnectionBeforeClose(t *testing.T) {
	oldServeConn := serveRPCConn
	served := make(chan struct{})
	t.Cleanup(func() { serveRPCConn = oldServeConn })
	serveRPCConn = func(s *rpc.Server, conn net.Conn) {
		close(served)
		oldServeConn(s, conn)
	}

	if err := ServeWithConfig(Config{SocketPath: filepath.Join(t.TempDir(), "plugin.sock"), PluginID: "p", GenerationID: 1}, observedServeCore{served: served}); err != nil {
		t.Fatalf("ServeWithConfig one accepted connection: %v", err)
	}
}

func TestServeWithConfigReportsRPCRegistrationFailure(t *testing.T) {
	oldRegister := registerRPCService
	t.Cleanup(func() { registerRPCService = oldRegister })
	sentinel := fmt.Errorf("register failed")
	registerRPCService = func(*rpc.Server, string, any) error { return sentinel }

	err := ServeWithConfig(Config{SocketPath: filepath.Join(t.TempDir(), "plugin.sock"), PluginID: "p", GenerationID: 1}, sdkBaseCore{})
	if !errors.Is(err, sentinel) || !strings.Contains(err.Error(), "register rpc service") {
		t.Fatalf("ServeWithConfig registration err = %v, want wrapped sentinel", err)
	}
}

func TestExampleMinimalLoadsTemplateAndServes(t *testing.T) {
	clearPluginEnv(t)
	setValidPluginEnv(t)
	oldServe := serveExampleWithConfig
	t.Cleanup(func() { serveExampleWithConfig = oldServe })

	called := false
	serveExampleWithConfig = func(cfg Config, core Core) error {
		called = true
		if cfg.PluginID != "echo" || cfg.GenerationID != 7 || cfg.AuthToken != "secret-token" || cfg.SocketPath == "" {
			t.Fatalf("unexpected example config: %+v", cfg)
		}
		tpl, ok := core.(*TemplatePlugin)
		if !ok {
			t.Fatalf("example core = %T, want *TemplatePlugin", core)
		}
		if tpl.PluginID != "echo" || tpl.GenerationID != 7 || tpl.Version() != "0.1.0" {
			t.Fatalf("unexpected example template: %+v", tpl)
		}
		return nil
	}

	Example_minimal()
	if !called {
		t.Fatal("example did not serve")
	}
}

func TestExampleMinimalPanicsOnConfigLoadError(t *testing.T) {
	clearPluginEnv(t)
	defer func() {
		if r := recover(); r == nil {
			t.Fatal("expected config load panic")
		}
	}()
	Example_minimal()
}

func TestExampleMinimalPanicsOnServeError(t *testing.T) {
	clearPluginEnv(t)
	setValidPluginEnv(t)
	oldServe := serveExampleWithConfig
	t.Cleanup(func() { serveExampleWithConfig = oldServe })
	sentinel := fmt.Errorf("serve failed")
	serveExampleWithConfig = func(Config, Core) error { return sentinel }

	defer func() {
		r := recover()
		if r == nil {
			t.Fatal("expected serve panic")
		}
		if !errors.Is(r.(error), sentinel) {
			t.Fatalf("panic = %v, want sentinel", r)
		}
	}()
	Example_minimal()
}

func TestServerLoggingBranchesForAuthAndCapabilities(t *testing.T) {
	logger := newTestLogger(t)
	core := &sdkTestCore{pluginID: "p", generationID: 4, authErr: fmt.Errorf("hook rejected")}
	srv := &server{core: core, cfg: Config{PluginID: "p", GenerationID: 4, AuthToken: "secret"}, capabilities: []string{"x"}, logger: logger}

	if err := srv.Auth(AuthRequest{Token: "secret"}, &AuthResponse{}); err == nil || !strings.Contains(err.Error(), "hook rejected") {
		t.Fatalf("hook auth err = %v", err)
	}
	core.authErr = nil
	if err := srv.Auth(AuthRequest{Token: "bad"}, &AuthResponse{}); !errors.Is(err, os.ErrPermission) {
		t.Fatalf("bad auth err = %v", err)
	}
	if err := srv.Auth(AuthRequest{Token: "secret"}, &AuthResponse{}); err != nil {
		t.Fatalf("good auth: %v", err)
	}
	if err := srv.Auth(AuthRequest{Token: "secret"}, &AuthResponse{}); !errors.Is(err, os.ErrPermission) {
		t.Fatalf("replay auth err = %v", err)
	}
	if err := srv.Capabilities(Empty{}, &CapabilitiesResponse{}); err != nil {
		t.Fatalf("capabilities: %v", err)
	}
}

func TestServeWithConfigReportsLoggerCreationFailure(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "not-a-dir")
	if err := os.WriteFile(file, []byte("x"), 0o600); err != nil {
		t.Fatalf("write marker: %v", err)
	}
	if err := ServeWithConfig(Config{SocketPath: filepath.Join(file, "plugin.sock"), PluginID: "p", GenerationID: 1}, sdkBaseCore{}); err == nil || !strings.Contains(err.Error(), "create plugin logger") {
		t.Fatalf("ServeWithConfig logger err = %v", err)
	}
}

func TestServerShutdownLogsWhenAuthenticated(t *testing.T) {
	logger := newTestLogger(t)
	core := &sdkTestCore{}
	srv := &server{core: core, cfg: Config{PluginID: "p", GenerationID: 1, AuthToken: "secret"}, authUsed: true, logger: logger}
	if err := srv.Shutdown(Empty{}, &Empty{}); err != nil {
		t.Fatalf("shutdown: %v", err)
	}
	if err := logger.Close(); err != nil {
		t.Fatalf("close logger: %v", err)
	}
	data, err := os.ReadFile(logger.Path())
	if err != nil {
		t.Fatalf("read log: %v", err)
	}
	if !strings.Contains(string(data), EventPluginShutdownCalled) || !strings.Contains(string(data), EventPluginRequestHandled) {
		t.Fatalf("shutdown log missing events:\n%s", data)
	}
}

func TestServerPreAuthOptionalMethodsAndShutdownDoNotDispatch(t *testing.T) {
	logger := newTestLogger(t)
	core := &sdkTestCore{}
	srv := &server{core: core, cfg: Config{PluginID: "p", GenerationID: 1, AuthToken: "secret"}, logger: logger}
	if err := srv.Shutdown(Empty{}, &Empty{}); !errors.Is(err, os.ErrPermission) {
		t.Fatalf("shutdown before auth = %v", err)
	}
	if err := srv.Sleep(SleepRequest{}, &Empty{}); !errors.Is(err, os.ErrPermission) {
		t.Fatalf("sleep before auth = %v", err)
	}
	if err := srv.Crash(CrashRequest{}, &Empty{}); !errors.Is(err, os.ErrPermission) {
		t.Fatalf("crash before auth = %v", err)
	}
	if core.shutdowns != 0 || len(core.sleeps) != 0 || len(core.crashes) != 0 {
		t.Fatalf("pre-auth dispatch happened: shutdowns=%d sleeps=%#v crashes=%#v", core.shutdowns, core.sleeps, core.crashes)
	}
}

func newTestLogger(t *testing.T) *Logger {
	t.Helper()
	logger, err := NewLogger(Config{SocketPath: filepath.Join(t.TempDir(), "plugin.sock"), PluginID: "p", GenerationID: 1})
	if err != nil {
		t.Fatalf("NewLogger: %v", err)
	}
	t.Cleanup(func() { _ = logger.Close() })
	return logger
}

func TestTemplateDefaultVersionAndShutdown(t *testing.T) {
	p := NewTemplate(Config{PluginID: "p", GenerationID: 5}, "")
	if p.Version() != "0.1.0" {
		t.Fatalf("default version = %q", p.Version())
	}
	if err := p.Shutdown(Empty{}, &Empty{}); err != nil {
		t.Fatalf("shutdown: %v", err)
	}
}

func authenticatedServer(core Core) *server {
	return &server{core: core, cfg: Config{PluginID: "p", GenerationID: 1, AuthToken: "secret"}, authUsed: true}
}
