package main

import (
	"bytes"
	"errors"
	"fmt"
	"net"
	"net/rpc"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"rpc_plugin_system/internal/adminrpc"
	"rpc_plugin_system/internal/eventlog"
	"rpc_plugin_system/internal/kernel"
	"rpc_plugin_system/test/testpluginapi"
)

type errWriter struct{}

func (errWriter) Write([]byte) (int, error) { return 0, errors.New("write failed") }

type adminTestService struct {
	fail string
}

func (s *adminTestService) maybe(method string) error {
	if s.fail == method {
		return fmt.Errorf("forced %s failure", method)
	}
	return nil
}

func (s *adminTestService) Plugins(_ adminrpc.Empty, out *kernel.HostState) error {
	if err := s.maybe("Plugins"); err != nil {
		return err
	}
	*out = kernel.HostState{Plugins: []kernel.State{{PluginID: "echo", Healthy: true}}, CapabilityMap: map[string][]string{"echo": {"echo"}}, Routes: []kernel.Route{{PluginID: "echo", Healthy: true, Mode: "direct"}}}
	return nil
}

func (s *adminTestService) Plugin(in adminrpc.PluginRequest, out *kernel.State) error {
	if err := s.maybe("Plugin"); err != nil {
		return err
	}
	*out = kernel.State{PluginID: in.PluginID, Healthy: true}
	return nil
}

func (s *adminTestService) Capabilities(_ adminrpc.Empty, out *map[string][]string) error {
	if err := s.maybe("Capabilities"); err != nil {
		return err
	}
	*out = map[string][]string{"echo": {"echo"}}
	return nil
}

func (s *adminTestService) Routes(_ adminrpc.Empty, out *[]kernel.Route) error {
	if err := s.maybe("Routes"); err != nil {
		return err
	}
	*out = []kernel.Route{{PluginID: "echo", Healthy: true, Mode: "direct"}}
	return nil
}

func (s *adminTestService) Heartbeat(in adminrpc.HeartbeatRequest, out *testpluginapi.HeartbeatResponse) error {
	if err := s.maybe("Heartbeat"); err != nil {
		return err
	}
	*out = testpluginapi.HeartbeatResponse{PluginID: in.PluginID, Status: "healthy"}
	return nil
}

func (s *adminTestService) Echo(in adminrpc.EchoRequest, out *testpluginapi.EchoResponse) error {
	if err := s.maybe("Echo"); err != nil {
		return err
	}
	*out = testpluginapi.EchoResponse{Message: in.Message}
	return nil
}

func (s *adminTestService) Restart(in adminrpc.RestartRequest, out *kernel.State) error {
	if err := s.maybe("Restart"); err != nil {
		return err
	}
	*out = kernel.State{PluginID: in.PluginID, GenerationID: 2, Healthy: true}
	return nil
}

func startAdminServer(t *testing.T, svc *adminTestService) string {
	t.Helper()
	runtimeDir := t.TempDir()
	listener, err := net.Listen("unix", filepath.Join(runtimeDir, "admin.sock"))
	if err != nil {
		t.Fatalf("listen unix: %v", err)
	}
	t.Cleanup(func() { _ = listener.Close() })
	server := rpc.NewServer()
	if err := server.RegisterName(adminrpc.ServiceName, svc); err != nil {
		t.Fatalf("register admin service: %v", err)
	}
	go func() {
		for {
			conn, err := listener.Accept()
			if err != nil {
				return
			}
			go server.ServeConn(conn)
		}
	}()
	return runtimeDir
}

func testEnv(runtimeDir string) runEnv {
	return runEnv{Prog: "rpcpluginctl", TempDir: func() string { return filepath.Dir(runtimeDir) }, Now: func() time.Time { return time.Date(2026, 5, 6, 12, 0, 0, 0, time.UTC) }}
}

func TestMainUsesExitSeam(t *testing.T) {
	oldArgs := os.Args
	oldExit := exit
	defer func() { os.Args = oldArgs; exit = oldExit }()
	os.Args = []string{"rpcpluginctl", "help"}
	var got int
	exit = func(code int) { got = code }
	main()
	if got != 0 {
		t.Fatalf("exit code = %d, want 0", got)
	}
}

func TestRunUsageAndParseFailures(t *testing.T) {
	for _, tc := range []struct {
		name string
		args []string
		code int
		want string
	}{
		{name: "no args", code: 1, want: "usage:"},
		{name: "no command", args: []string{"-bogus"}, code: 1, want: "usage:"},
		{name: "flag parse", args: []string{"status", "-runtime-dir"}, code: 2, want: "flag needs an argument"},
		{name: "extra args", args: []string{"status", "extra"}, code: 1, want: "unexpected extra args"},
		{name: "validation", args: []string{"plugin"}, code: 1, want: "-plugin-id is required"},
		{name: "unknown command", args: []string{"bogus"}, code: 1, want: "unknown command"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var stdout, stderr bytes.Buffer
			if got := run(tc.args, runEnv{}, &stdout, &stderr); got != tc.code {
				t.Fatalf("code = %d want %d; stderr=%q", got, tc.code, stderr.String())
			}
			if !strings.Contains(stderr.String(), tc.want) {
				t.Fatalf("stderr = %q want %q", stderr.String(), tc.want)
			}
		})
	}
}

func TestRunHelp(t *testing.T) {
	var stdout, stderr bytes.Buffer
	if got := run([]string{"help"}, runEnv{Prog: "ctl"}, &stdout, &stderr); got != 0 {
		t.Fatalf("code = %d stderr=%q", got, stderr.String())
	}
	if !strings.Contains(stdout.String(), "usage: ctl") {
		t.Fatalf("stdout = %q", stdout.String())
	}
}

func TestRunAdminCommands(t *testing.T) {
	runtimeDir := startAdminServer(t, &adminTestService{})
	for _, tc := range []struct {
		name string
		args []string
		want string
	}{
		{name: "status", args: []string{"status"}, want: "CapabilityMap"},
		{name: "plugins", args: []string{"plugins"}, want: "Plugins"},
		{name: "plugin", args: []string{"plugin", "-plugin-id", "echo"}, want: "\"PluginID\": \"echo\""},
		{name: "capabilities", args: []string{"capabilities"}, want: "echo"},
		{name: "routes", args: []string{"routes"}, want: "direct"},
		{name: "heartbeat", args: []string{"heartbeat", "-plugin-id", "echo"}, want: "healthy"},
		{name: "echo", args: []string{"echo", "-plugin-id", "echo", "-message", "hello"}, want: "hello"},
		{name: "restart", args: []string{"restart", "-plugin-id", "echo"}, want: "GenerationID"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			args := append([]string{"-runtime-dir", runtimeDir}, tc.args...)
			var stdout, stderr bytes.Buffer
			if got := run(args, testEnv(runtimeDir), &stdout, &stderr); got != 0 {
				t.Fatalf("code = %d stderr=%q", got, stderr.String())
			}
			if !strings.Contains(stdout.String(), tc.want) {
				t.Fatalf("stdout = %q want %q", stdout.String(), tc.want)
			}
		})
	}
}

func TestRunAdminDialCallAndWriteFailures(t *testing.T) {
	var stdout, stderr bytes.Buffer
	missing := filepath.Join(t.TempDir(), "missing")
	if got := run([]string{"-runtime-dir", missing, "status"}, runEnv{}, &stdout, &stderr); got != 1 || !strings.Contains(stderr.String(), "dial admin socket") {
		t.Fatalf("dial failure code=%d stderr=%q", got, stderr.String())
	}

	for _, tc := range []struct {
		method string
		args   []string
	}{
		{method: "Plugins", args: []string{"status"}},
		{method: "Plugin", args: []string{"plugin", "-plugin-id", "echo"}},
		{method: "Capabilities", args: []string{"capabilities"}},
		{method: "Routes", args: []string{"routes"}},
		{method: "Heartbeat", args: []string{"heartbeat", "-plugin-id", "echo"}},
		{method: "Echo", args: []string{"echo", "-plugin-id", "echo"}},
		{method: "Restart", args: []string{"restart", "-plugin-id", "echo"}},
	} {
		runtimeDir := startAdminServer(t, &adminTestService{fail: tc.method})
		stdout.Reset()
		stderr.Reset()
		full := append([]string{"-runtime-dir", runtimeDir}, tc.args...)
		if got := run(full, testEnv(runtimeDir), &stdout, &stderr); got != 1 || !strings.Contains(stderr.String(), "forced "+tc.method+" failure") {
			t.Fatalf("call failure method=%s code=%d stderr=%q", tc.method, got, stderr.String())
		}
	}

	runtimeDir := startAdminServer(t, &adminTestService{})
	for _, args := range [][]string{
		{"status"}, {"plugin", "-plugin-id", "echo"}, {"capabilities"}, {"routes"}, {"heartbeat", "-plugin-id", "echo"}, {"echo", "-plugin-id", "echo"}, {"restart", "-plugin-id", "echo"},
	} {
		stderr.Reset()
		full := append([]string{"-runtime-dir", runtimeDir}, args...)
		if got := run(full, testEnv(runtimeDir), errWriter{}, &stderr); got != 1 || !strings.Contains(stderr.String(), "write failed") {
			t.Fatalf("write failure args=%v code=%d stderr=%q", args, got, stderr.String())
		}
	}
}

func writeEventLog(t *testing.T, runtimeDir string) {
	t.Helper()
	logPath := filepath.Join(runtimeDir, "events.jsonl")
	log, err := eventlog.New(logPath)
	if err != nil {
		t.Fatalf("new event log: %v", err)
	}
	t.Cleanup(func() { _ = log.Close() })
	for _, ev := range []eventlog.Event{
		{Time: time.Date(2026, 5, 6, 11, 30, 0, 0, time.UTC), Level: eventlog.LevelInfo, Component: eventlog.ComponentKernel, Event: eventlog.EventManagerInitialized, PluginID: "echo", Message: "old"},
		{Time: time.Date(2026, 5, 6, 11, 55, 0, 0, time.UTC), Level: eventlog.LevelError, Component: eventlog.ComponentRPC, Event: eventlog.EventRPCFailed, PluginID: "echo", Method: "Echo", Error: "boom"},
	} {
		if err := log.Write(ev); err != nil {
			t.Fatalf("write event: %v", err)
		}
	}
}

func TestRunLogsCommands(t *testing.T) {
	runtimeDir := t.TempDir()
	writeEventLog(t, runtimeDir)
	for _, tc := range []struct {
		name string
		args []string
		want string
	}{
		{name: "text", args: []string{"logs", "-level", "error", "-since", "10m"}, want: "boom"},
		{name: "json", args: []string{"logs", "-format", "json", "-reverse=false", "-limit", "1"}, want: "manager_initialized"},
		{name: "summary text", args: []string{"logs", "-summary"}, want: "total: 2"},
		{name: "summary json", args: []string{"logs", "-summary", "-format", "json"}, want: "by_level"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			args := append([]string{"-runtime-dir", runtimeDir}, tc.args...)
			var stdout, stderr bytes.Buffer
			if got := run(args, testEnv(runtimeDir), &stdout, &stderr); got != 0 {
				t.Fatalf("code = %d stderr=%q", got, stderr.String())
			}
			if !strings.Contains(stdout.String(), tc.want) {
				t.Fatalf("stdout = %q want %q", stdout.String(), tc.want)
			}
		})
	}
}

func TestRunLogsFailures(t *testing.T) {
	var stdout, stderr bytes.Buffer
	missing := filepath.Join(t.TempDir(), "missing")
	if got := run([]string{"-runtime-dir", missing, "logs"}, runEnv{}, &stdout, &stderr); got != 1 || !strings.Contains(stderr.String(), "no such file") {
		t.Fatalf("resolve failure code=%d stderr=%q", got, stderr.String())
	}

	runtimeDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(runtimeDir, "events.jsonl"), []byte("not json\n"), 0o600); err != nil {
		t.Fatalf("write bad log: %v", err)
	}
	stdout.Reset()
	stderr.Reset()
	if got := run([]string{"-runtime-dir", runtimeDir, "logs"}, runEnv{}, &stdout, &stderr); got != 1 || !strings.Contains(stderr.String(), "decode event log") {
		t.Fatalf("read failure code=%d stderr=%q", got, stderr.String())
	}

	if err := os.WriteFile(filepath.Join(runtimeDir, "events.jsonl"), []byte(`{"time":"2026-05-06T12:00:00Z","level":"info","event":"x"}`+"\n"), 0o600); err != nil {
		t.Fatalf("write good log: %v", err)
	}
	for _, args := range [][]string{{"logs"}, {"logs", "-format", "json"}, {"logs", "-summary"}, {"logs", "-summary", "-format", "json"}} {
		stderr.Reset()
		full := append([]string{"-runtime-dir", runtimeDir}, args...)
		if got := run(full, runEnv{}, errWriter{}, &stderr); got != 1 || !strings.Contains(stderr.String(), "write failed") {
			t.Fatalf("write failure args=%v code=%d stderr=%q", args, got, stderr.String())
		}
	}
}
