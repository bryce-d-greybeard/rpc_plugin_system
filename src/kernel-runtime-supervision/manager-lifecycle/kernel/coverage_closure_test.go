package kernel

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"rpc_plugin_system/control-plane-ops/event-log-write/eventlog"
	"rpc_plugin_system/plugin-authoring-sdk/test-plugin-api/testpluginapi"
)

func TestHostRestartPluginRejectsRoutedTypeMismatch(t *testing.T) {
	old := routeHostCall
	routeHostCall = func(*Host, string, RoutedCall, any) (RoutedResponse, error) {
		return RoutedResponse{Body: "not state"}, nil
	}
	defer func() { routeHostCall = old }()

	if _, err := (&Host{}).RestartPlugin("echo"); err == nil || !strings.Contains(err.Error(), "type mismatch") {
		t.Fatalf("expected routed response type mismatch, got %v", err)
	}
}

func TestManagerStartReportsTokenCreationFailure(t *testing.T) {
	pluginBin := buildPlugin(t)
	runtimeDir := t.TempDir()
	manager, err := New(Config{RuntimeDir: runtimeDir, PluginPath: pluginBin, PluginID: "echo", EventLogPath: filepath.Join(runtimeDir, "events.jsonl")})
	if err != nil {
		t.Fatalf("new manager: %v", err)
	}
	defer manager.Close()

	old := newBootstrapToken
	newBootstrapToken = func() ([]byte, error) { return nil, errors.New("token failed") }
	defer func() { newBootstrapToken = old }()

	if err := manager.Start(); err == nil || !strings.Contains(err.Error(), "token failed") {
		t.Fatalf("expected token creation failure, got %v", err)
	}
}

func TestManagerStartCleansUpPeerCredVerifierFailure(t *testing.T) {
	pluginBin := buildPlugin(t)
	runtimeDir := t.TempDir()
	manager, err := New(Config{RuntimeDir: runtimeDir, PluginPath: pluginBin, PluginID: "echo", EventLogPath: filepath.Join(runtimeDir, "events.jsonl"), DialTimeout: time.Second, CallTimeout: time.Second})
	if err != nil {
		t.Fatalf("new manager: %v", err)
	}
	defer manager.Close()

	old := verifyManagerPeerCred
	verifyManagerPeerCred = func(*Manager, *rpcClient, int, uint64) error { return errors.New("peer verifier failed") }
	defer func() { verifyManagerPeerCred = old }()

	if err := manager.Start(); err == nil || !strings.Contains(err.Error(), "peer verifier failed") {
		t.Fatalf("expected peer verifier failure, got %v", err)
	}
}

func TestManagerStartCapabilitiesFailureBranches(t *testing.T) {
	cases := []struct {
		name string
		call func(m *Manager, generation uint64, method string, reply any) error
		want string
	}{
		{
			name: "rpc error",
			call: func(_ *Manager, _ uint64, method string, _ any) error {
				if method == testpluginapi.MethodCapabilities {
					return errors.New("capabilities transport failed")
				}
				return nil
			},
			want: "capabilities rpc",
		},
		{
			name: "plugin id mismatch",
			call: func(m *Manager, generation uint64, method string, reply any) error {
				if method == testpluginapi.MethodCapabilities {
					*outAsCaps(t, reply) = testpluginapi.CapabilitiesResponse{PluginID: "wrong", GenerationID: generation}
				}
				return nil
			},
			want: "plugin id mismatch",
		},
		{
			name: "generation mismatch",
			call: func(m *Manager, generation uint64, method string, reply any) error {
				if method == testpluginapi.MethodCapabilities {
					*outAsCaps(t, reply) = testpluginapi.CapabilitiesResponse{PluginID: m.cfg.PluginID, GenerationID: generation + 1}
				}
				return nil
			},
			want: "generation mismatch",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			pluginBin := buildPlugin(t)
			runtimeDir := t.TempDir()
			manager, err := New(Config{RuntimeDir: runtimeDir, PluginPath: pluginBin, PluginID: "echo", EventLogPath: filepath.Join(runtimeDir, "events.jsonl"), DialTimeout: time.Second, CallTimeout: time.Second})
			if err != nil {
				t.Fatalf("new manager: %v", err)
			}
			defer manager.Close()

			oldCall := managerRPCCall
			managerRPCCall = func(m *Manager, client *rpcClient, generation, epoch uint64, method string, args any, reply any) error {
				if method == testpluginapi.MethodAuth {
					*outAsAuth(t, reply) = testpluginapi.AuthResponse{PluginID: m.cfg.PluginID, GenerationID: generation}
					return nil
				}
				return tc.call(m, generation, method, reply)
			}
			defer func() { managerRPCCall = oldCall }()

			if err := manager.Start(); err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("expected %q failure, got %v", tc.want, err)
			}
		})
	}
}

func outAsAuth(t *testing.T, reply any) *testpluginapi.AuthResponse {
	t.Helper()
	out, ok := reply.(*testpluginapi.AuthResponse)
	if !ok {
		t.Fatalf("reply type = %T, want auth response", reply)
	}
	return out
}

func outAsCaps(t *testing.T, reply any) *testpluginapi.CapabilitiesResponse {
	t.Helper()
	out, ok := reply.(*testpluginapi.CapabilitiesResponse)
	if !ok {
		t.Fatalf("reply type = %T, want capabilities response", reply)
	}
	return out
}

func TestManagerHeartbeatRejectsStaleSuccessAndLogsHealthyTransition(t *testing.T) {
	client := &rpcClient{}
	manager := &Manager{cfg: Config{PluginID: "echo", CallTimeout: time.Second}, client: client, state: State{PluginID: "echo", GenerationID: 1, Healthy: true}, closedCh: make(chan struct{})}

	oldCall := managerRPCCall
	managerRPCCall = func(m *Manager, _ *rpcClient, _ uint64, _ uint64, method string, _ any, reply any) error {
		if method != testpluginapi.MethodHeartbeat {
			return fmt.Errorf("unexpected method %s", method)
		}
		*outAsHeartbeat(t, reply) = testpluginapi.HeartbeatResponse{PluginID: m.cfg.PluginID, GenerationID: 1, Status: testpluginapi.StatusHealthy}
		m.mu.Lock()
		m.rpcEpoch++
		m.mu.Unlock()
		return nil
	}
	if _, err := manager.Heartbeat(); err == nil || !strings.Contains(err.Error(), "stale heartbeat rejected") {
		t.Fatalf("expected stale heartbeat rejection, got %v", err)
	}

	manager.state.Healthy = false
	manager.rpcEpoch = 0
	managerRPCCall = func(m *Manager, _ *rpcClient, _ uint64, _ uint64, method string, _ any, reply any) error {
		if method != testpluginapi.MethodHeartbeat {
			return fmt.Errorf("unexpected method %s", method)
		}
		*outAsHeartbeat(t, reply) = testpluginapi.HeartbeatResponse{PluginID: m.cfg.PluginID, GenerationID: 1, Status: testpluginapi.StatusHealthy}
		return nil
	}
	defer func() { managerRPCCall = oldCall }()
	if _, err := manager.Heartbeat(); err != nil {
		t.Fatalf("healthy transition heartbeat: %v", err)
	}
	if !manager.State().Healthy {
		t.Fatal("manager should be healthy after successful heartbeat")
	}
}

func outAsHeartbeat(t *testing.T, reply any) *testpluginapi.HeartbeatResponse {
	t.Helper()
	out, ok := reply.(*testpluginapi.HeartbeatResponse)
	if !ok {
		t.Fatalf("reply type = %T, want heartbeat response", reply)
	}
	return out
}

func TestManagerCloseMarksClosedWhenStopSeamLeavesItOpen(t *testing.T) {
	runtimeDir := t.TempDir()
	logger, err := eventlog.New(filepath.Join(runtimeDir, "events.jsonl"))
	if err != nil {
		t.Fatalf("new logger: %v", err)
	}
	manager := &Manager{cfg: Config{RuntimeDir: runtimeDir, PluginID: "echo"}, log: logger, closedCh: make(chan struct{})}
	oldStop := stopManagerCurrent
	stopManagerCurrent = func(*Manager, bool, string) error { return nil }
	defer func() { stopManagerCurrent = oldStop }()
	if err := manager.Close(); err != nil {
		t.Fatalf("close with stop seam: %v", err)
	}
	if !manager.closed || manager.rpcEpoch != 1 {
		t.Fatalf("close did not mark manager closed: closed=%v epoch=%d", manager.closed, manager.rpcEpoch)
	}
}

func TestManagerCloseReturnsLoggerCloseErrorWhenStopSucceeds(t *testing.T) {
	runtimeDir := t.TempDir()
	logger, err := eventlog.New(filepath.Join(runtimeDir, "events.jsonl"))
	if err != nil {
		t.Fatalf("new logger: %v", err)
	}
	if err := logger.Close(); err != nil {
		t.Fatalf("pre-close logger: %v", err)
	}
	manager := &Manager{cfg: Config{RuntimeDir: runtimeDir, PluginID: "echo"}, log: logger, closedCh: make(chan struct{})}
	if err := manager.Close(); err == nil {
		t.Fatal("expected logger close error")
	}
}

func TestManagerCloseAndRestartReturnStopErrors(t *testing.T) {
	oldWait := waitProcess
	waitProcess = func(*os.Process) (*os.ProcessState, error) { return nil, errors.New("wait failed") }
	defer func() { waitProcess = oldWait }()

	runtimeDir := t.TempDir()
	logger, err := eventlog.New(filepath.Join(runtimeDir, "events.jsonl"))
	if err != nil {
		t.Fatalf("new logger: %v", err)
	}
	closing := &Manager{cfg: Config{RuntimeDir: runtimeDir, PluginID: "echo"}, log: logger, cmd: &exec.Cmd{Process: &os.Process{Pid: os.Getpid()}}, state: State{PluginID: "echo", GenerationID: 1, PID: os.Getpid()}, closedCh: make(chan struct{})}
	if err := closing.Close(); err == nil || !strings.Contains(err.Error(), "wait plugin exit") {
		t.Fatalf("expected close wait error, got %v", err)
	}

	restarting := &Manager{cfg: Config{RuntimeDir: runtimeDir, PluginID: "echo"}, cmd: &exec.Cmd{Process: &os.Process{Pid: os.Getpid()}}, state: State{PluginID: "echo", GenerationID: 1, PID: os.Getpid()}, closedCh: make(chan struct{})}
	if err := restarting.Restart(); err == nil || !strings.Contains(err.Error(), "wait plugin exit") {
		t.Fatalf("expected restart stop error, got %v", err)
	}
}

func TestStopCurrentProcessKillFailureBranches(t *testing.T) {
	t.Run("signal failure", func(t *testing.T) {
		oldWait, oldSignal := waitProcess, signalProcess
		release := make(chan struct{})
		waitDone := make(chan struct{})
		waitProcess = func(*os.Process) (*os.ProcessState, error) {
			defer close(waitDone)
			<-release
			return nil, os.ErrProcessDone
		}
		signalProcess = func(*os.Process, os.Signal) error { return errors.New("signal failed") }
		defer func() {
			close(release)
			<-waitDone
			waitProcess, signalProcess = oldWait, oldSignal
		}()

		manager := &Manager{cfg: Config{PluginID: "echo"}, cmd: &exec.Cmd{Process: &os.Process{Pid: os.Getpid()}}, state: State{PluginID: "echo", GenerationID: 1, PID: os.Getpid()}, closedCh: make(chan struct{})}
		if err := manager.stopCurrent(false, "signal failure"); err == nil || !strings.Contains(err.Error(), "kill plugin") {
			t.Fatalf("expected signal failure, got %v", err)
		}
	})

	t.Run("wait after kill failure", func(t *testing.T) {
		oldWait, oldSignal := waitProcess, signalProcess
		release := make(chan struct{})
		waitProcess = func(*os.Process) (*os.ProcessState, error) {
			<-release
			return nil, errors.New("wait after kill failed")
		}
		signalProcess = func(*os.Process, os.Signal) error {
			close(release)
			return nil
		}
		defer func() { waitProcess, signalProcess = oldWait, oldSignal }()

		manager := &Manager{cfg: Config{PluginID: "echo"}, cmd: &exec.Cmd{Process: &os.Process{Pid: os.Getpid()}}, state: State{PluginID: "echo", GenerationID: 1, PID: os.Getpid()}, closedCh: make(chan struct{})}
		if err := manager.stopCurrent(false, "wait after kill failure"); err == nil || !strings.Contains(err.Error(), "wait plugin exit after kill") {
			t.Fatalf("expected wait after kill failure, got %v", err)
		}
	})
}
