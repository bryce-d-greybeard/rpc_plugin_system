package adminrpc

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/rpc"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"rpc_plugin_system/internal/kernel"
	"rpc_plugin_system/test/testpluginapi"
	"rpc_plugin_system/test/testroot"
)

func TestMethodsRequireHost(t *testing.T) {
	service := Service{}
	for _, tc := range []struct {
		name string
		run  func() error
	}{
		{name: "status", run: func() error {
			var out kernel.HostState
			return service.Status(Empty{}, &out)
		}},
		{name: "plugins", run: func() error {
			var out kernel.HostState
			return service.Plugins(Empty{}, &out)
		}},
		{name: "plugin", run: func() error {
			var out kernel.State
			return service.Plugin(PluginRequest{PluginID: "echo"}, &out)
		}},
		{name: "capabilities", run: func() error {
			var out map[string][]string
			return service.Capabilities(Empty{}, &out)
		}},
		{name: "routes", run: func() error {
			var out []kernel.Route
			return service.Routes(Empty{}, &out)
		}},
		{name: "heartbeat", run: func() error {
			var out testpluginapi.HeartbeatResponse
			return service.Heartbeat(HeartbeatRequest{PluginID: "echo"}, &out)
		}},
		{name: "echo", run: func() error {
			var out testpluginapi.EchoResponse
			return service.Echo(EchoRequest{PluginID: "echo", Message: "ping"}, &out)
		}},
		{name: "restart", run: func() error {
			var out kernel.State
			return service.Restart(RestartRequest{PluginID: "echo"}, &out)
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			err := tc.run()
			if err == nil || !strings.Contains(err.Error(), "host is required") {
				t.Fatalf("err = %v, want host is required", err)
			}
		})
	}
}

func TestPluginIDMethodsRejectInvalidPluginID(t *testing.T) {
	service := Service{Host: &kernel.Host{}}
	for _, tc := range []struct {
		name string
		run  func() error
	}{
		{name: "plugin", run: func() error {
			var out kernel.State
			return service.Plugin(PluginRequest{PluginID: "../echo"}, &out)
		}},
		{name: "heartbeat", run: func() error {
			var out testpluginapi.HeartbeatResponse
			return service.Heartbeat(HeartbeatRequest{PluginID: "echo/test"}, &out)
		}},
		{name: "echo", run: func() error {
			var out testpluginapi.EchoResponse
			return service.Echo(EchoRequest{PluginID: "../echo", Message: "ping"}, &out)
		}},
		{name: "restart", run: func() error {
			var out kernel.State
			return service.Restart(RestartRequest{PluginID: "echo/test"}, &out)
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			err := tc.run()
			if err == nil || !strings.Contains(err.Error(), "invalid plugin id") {
				t.Fatalf("err = %v, want invalid plugin id", err)
			}
		})
	}
}

func TestPluginIDMethodsReturnHostErrors(t *testing.T) {
	service := Service{Host: &kernel.Host{}}
	for _, tc := range []struct {
		name string
		run  func() error
	}{
		{name: "plugin", run: func() error {
			var out kernel.State
			return service.Plugin(PluginRequest{PluginID: "missing"}, &out)
		}},
		{name: "heartbeat", run: func() error {
			var out testpluginapi.HeartbeatResponse
			return service.Heartbeat(HeartbeatRequest{PluginID: "missing"}, &out)
		}},
		{name: "echo", run: func() error {
			var out testpluginapi.EchoResponse
			return service.Echo(EchoRequest{PluginID: "missing", Message: "ping"}, &out)
		}},
		{name: "restart", run: func() error {
			var out kernel.State
			return service.Restart(RestartRequest{PluginID: "missing"}, &out)
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			err := tc.run()
			if err == nil || !strings.Contains(err.Error(), "unknown plugin id: missing") {
				t.Fatalf("err = %v, want unknown plugin id", err)
			}
		})
	}
}

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

	var plugins kernel.HostState
	if err := client.Call(MethodPlugins, Empty{}, &plugins); err != nil {
		t.Fatalf("plugins rpc: %v", err)
	}
	if len(plugins.Plugins) != 1 || plugins.Plugins[0].PluginID != "echo" || plugins.Plugins[0].GenerationID != firstState.GenerationID || plugins.Plugins[0].PID == 0 {
		t.Fatalf("unexpected plugins state: %+v", plugins)
	}
	if len(plugins.CapabilityMap["echo"]) != 1 || plugins.CapabilityMap["echo"][0] != "echo" {
		t.Fatalf("unexpected plugins capability map: %+v", plugins.CapabilityMap)
	}
	if len(plugins.Routes) != 1 || plugins.Routes[0].PluginID != "echo" {
		t.Fatalf("unexpected plugins routes: %+v", plugins.Routes)
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

func TestServeReturnsListenError(t *testing.T) {
	want := errors.New("listen boom")
	err := serve(context.Background(), filepath.Join(t.TempDir(), "admin.sock"), nil, func(string) (net.Listener, error) {
		return nil, want
	}, registerAdminService)
	if !errors.Is(err, want) || !strings.Contains(err.Error(), "listen admin socket") {
		t.Fatalf("err = %v, want wrapped listen error", err)
	}
}

func TestServeReturnsRegisterError(t *testing.T) {
	want := errors.New("register boom")
	err := serve(context.Background(), filepath.Join(t.TempDir(), "admin.sock"), nil, func(string) (net.Listener, error) {
		return failingListener{err: errors.New("unused")}, nil
	}, func(*rpc.Server, *kernel.Host) error {
		return want
	})
	if !errors.Is(err, want) {
		t.Fatalf("err = %v, want register error", err)
	}
}

func TestServeReturnsAcceptErrorWhenContextActive(t *testing.T) {
	want := errors.New("accept boom")
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	err := serve(ctx, filepath.Join(t.TempDir(), "admin.sock"), nil, func(string) (net.Listener, error) {
		return failingListener{err: want}, nil
	}, registerAdminService)
	if !errors.Is(err, want) || !strings.Contains(err.Error(), "accept admin socket") {
		t.Fatalf("err = %v, want wrapped accept error", err)
	}
}

func TestServeReturnsNilWhenAcceptFailsAfterContextCancel(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	listener := newClosingListener(errors.New("closed"))

	err := serve(ctx, filepath.Join(t.TempDir(), "admin.sock"), nil, func(string) (net.Listener, error) {
		return listener, nil
	}, registerAdminService)
	if err != nil {
		t.Fatalf("err = %v, want nil", err)
	}
}

func TestRegisterAdminServiceReturnsDuplicateNameError(t *testing.T) {
	server := rpc.NewServer()
	if err := registerAdminService(server, nil); err != nil {
		t.Fatalf("first register: %v", err)
	}
	err := registerAdminService(server, nil)
	if err == nil || !strings.Contains(err.Error(), "register admin rpc") {
		t.Fatalf("err = %v, want register admin rpc error", err)
	}
}

type failingListener struct {
	err error
}

func (l failingListener) Accept() (net.Conn, error) {
	return nil, l.err
}

func (l failingListener) Close() error {
	return nil
}

func (l failingListener) Addr() net.Addr {
	return fakeAddr("admin")
}

type closingListener struct {
	err       error
	closed    chan struct{}
	closeOnce sync.Once
}

func newClosingListener(err error) *closingListener {
	return &closingListener{err: err, closed: make(chan struct{})}
}

func (l *closingListener) Accept() (net.Conn, error) {
	<-l.closed
	return nil, l.err
}

func (l *closingListener) Close() error {
	l.closeOnce.Do(func() {
		close(l.closed)
	})
	return nil
}

func (l *closingListener) Addr() net.Addr {
	return fakeAddr("admin")
}

type fakeAddr string

func (a fakeAddr) Network() string {
	return "unix"
}

func (a fakeAddr) String() string {
	return string(a)
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
		cmd := exec.Command("go", "build", "-buildvcs=false", "-o", pluginBuildPath, "./examples/rpcplugin-echo")
		cmd.Dir = testroot.SourceRoot(t)
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
