package kernel

import (
	"context"
	"errors"
	"io"
	"net"
	"net/rpc"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"rpc_plugin_system/internal/eventlog"
	amruntime "rpc_plugin_system/internal/runtime"
	"rpc_plugin_system/test/testpluginapi"
)

type testRPCService struct{}

func (testRPCService) Echo(req testpluginapi.EchoRequest, resp *testpluginapi.EchoResponse) error {
	resp.Message = req.Message
	return nil
}

func (testRPCService) Shutdown(testpluginapi.Empty, *testpluginapi.Empty) error {
	return errors.New("shutdown failed")
}

type timeoutNetError struct{}

func (timeoutNetError) Error() string   { return "timeout" }
func (timeoutNetError) Timeout() bool   { return true }
func (timeoutNetError) Temporary() bool { return true }

func TestManagerHelperNilAndErrorBranches(t *testing.T) {
	var nilClient *rpcClient
	if err := nilClient.close(); err != nil {
		t.Fatalf("nil rpc client close = %v", err)
	}
	if err := (&rpcClient{}).close(); err != nil {
		t.Fatalf("empty rpc client close = %v", err)
	}

	left, right := net.Pipe()
	defer right.Close()
	if err := (&rpcClient{client: rpc.NewClient(left)}).close(); err != nil && !strings.Contains(err.Error(), "closed") {
		t.Fatalf("rpc-only client close = %v", err)
	}

	manager := &Manager{}
	manager.logEvent(eventlog.Event{Event: eventlog.EventManagerInitialized})
	manager.poisonClient(nil, 1, "method", "nil client", nil)
	if got := errorString(nil); got != "" {
		t.Fatalf("nil errorString = %q", got)
	}
	if got := errorString(errors.New("boom")); got != "boom" {
		t.Fatalf("errorString = %q", got)
	}
	if isRPCPoisonError(nil) {
		t.Fatal("nil error should not be rpc poison")
	}
	for _, err := range []error{rpc.ErrShutdown, io.EOF, io.ErrUnexpectedEOF, net.ErrClosed, timeoutNetError{}, &net.OpError{Op: "read", Net: "unix", Err: errors.New("reset")}} {
		if !isRPCPoisonError(err) {
			t.Fatalf("%T should be treated as rpc poison", err)
		}
	}
	if isRPCPoisonError(errors.New("plain")) {
		t.Fatal("plain error should not be rpc poison")
	}
	if err := manager.verifyPeerCred(nil, 123, 1); err == nil {
		t.Fatal("expected nil peercred client rejection")
	}
	pipeLeft, pipeRight := net.Pipe()
	defer pipeLeft.Close()
	defer pipeRight.Close()
	if err := manager.verifyPeerCred(&rpcClient{conn: pipeLeft}, 123, 1); err == nil {
		t.Fatal("expected non-unix peercred read rejection")
	}
}

func TestVerifyPeerCredRejectsWrongPID(t *testing.T) {
	runtimeDir := t.TempDir()
	socketPath := filepath.Join(runtimeDir, "peer.sock")
	listener, err := net.Listen("unix", socketPath)
	if err != nil {
		t.Fatalf("listen unix socket: %v", err)
	}
	defer listener.Close()

	accepted := make(chan net.Conn, 1)
	acceptErr := make(chan error, 1)
	go func() {
		conn, err := listener.Accept()
		if err != nil {
			acceptErr <- err
			return
		}
		accepted <- conn
	}()

	clientConn, err := net.Dial("unix", socketPath)
	if err != nil {
		t.Fatalf("dial unix socket: %v", err)
	}
	defer clientConn.Close()
	select {
	case conn := <-accepted:
		defer conn.Close()
	case err := <-acceptErr:
		t.Fatalf("accept unix socket: %v", err)
	case <-time.After(time.Second):
		t.Fatal("timed out accepting unix socket")
	}

	manager := &Manager{cfg: Config{RuntimeDir: runtimeDir, PluginID: "echo"}}
	if err := manager.verifyPeerCred(&rpcClient{conn: clientConn}, os.Getpid()+1, 1); err == nil || !strings.Contains(err.Error(), "peer pid mismatch") {
		t.Fatalf("expected peer pid mismatch, got %v", err)
	}
}

func TestMonitorLoopFailureBranches(t *testing.T) {
	closed := &Manager{cfg: Config{PluginID: "echo", HeartbeatEvery: time.Millisecond}, closedCh: make(chan struct{})}
	close(closed.closedCh)
	closedCtx, closedCancel := context.WithTimeout(context.Background(), time.Second)
	defer closedCancel()
	closed.MonitorLoop(closedCtx)

	restarting := &Manager{cfg: Config{PluginID: "echo", HeartbeatEvery: time.Millisecond}, closedCh: make(chan struct{})}
	restartCtx, restartCancel := context.WithTimeout(context.Background(), 5*time.Millisecond)
	defer restartCancel()
	restarting.MonitorLoop(restartCtx)
}

func TestManagerClosedAndNotStartedErrors(t *testing.T) {
	closed := &Manager{cfg: Config{PluginID: "echo"}, closed: true, closedCh: make(chan struct{})}
	if err := closed.Start(); err == nil {
		t.Fatal("expected start on closed manager to fail")
	}
	if _, err := closed.Heartbeat(); err == nil {
		t.Fatal("expected heartbeat on closed manager to fail")
	}
	if _, err := closed.Echo("msg"); err == nil {
		t.Fatal("expected echo on closed manager to fail")
	}
	if err := closed.Sleep(time.Millisecond); err == nil {
		t.Fatal("expected sleep on closed manager to fail")
	}
	if err := closed.Crash(1); err == nil {
		t.Fatal("expected crash on closed manager to fail")
	}
	if err := closed.Restart(); err == nil {
		t.Fatal("expected restart on closed manager to fail")
	}
	if err := closed.Kill(); err == nil {
		t.Fatal("expected kill on closed manager to fail")
	}

	notStarted := &Manager{cfg: Config{PluginID: "echo"}, closedCh: make(chan struct{})}
	if _, err := notStarted.Heartbeat(); err == nil {
		t.Fatal("expected heartbeat on not-started manager to fail")
	}
	if _, err := notStarted.Echo("msg"); err == nil {
		t.Fatal("expected echo on not-started manager to fail")
	}
	if err := notStarted.Sleep(time.Millisecond); err == nil {
		t.Fatal("expected sleep on not-started manager to fail")
	}
	if err := notStarted.Crash(1); err == nil {
		t.Fatal("expected crash on not-started manager to fail")
	}
	if err := notStarted.call(nil, 0, 0, "missing", nil, nil); err == nil {
		t.Fatal("expected nil rpc client call rejection")
	}
}

func TestManagerNewDefaultsAndValidation(t *testing.T) {
	pluginBin := buildPlugin(t)
	runtimeDir := t.TempDir()
	logPath := filepath.Join(runtimeDir, "events.jsonl")

	if _, err := New(Config{}); err == nil {
		t.Fatal("expected missing manager config rejection")
	}
	runtimeFile := filepath.Join(runtimeDir, "runtime-file")
	if err := os.WriteFile(runtimeFile, []byte("not a dir"), 0o600); err != nil {
		t.Fatalf("write runtime file: %v", err)
	}
	if _, err := New(Config{RuntimeDir: runtimeFile, PluginPath: pluginBin, PluginID: "echo", EventLogPath: filepath.Join(runtimeDir, "ensure-dir-events.jsonl")}); err == nil {
		t.Fatal("expected runtime dir creation on file path to fail")
	}
	eventLogDir := filepath.Join(runtimeDir, "eventlog-dir")
	if err := os.Mkdir(eventLogDir, 0o700); err != nil {
		t.Fatalf("mkdir eventlog path: %v", err)
	}
	if _, err := New(Config{RuntimeDir: filepath.Join(runtimeDir, "eventlog-runtime"), PluginPath: pluginBin, PluginID: "echo", EventLogPath: eventLogDir}); err == nil {
		t.Fatal("expected event logger creation on directory path to fail")
	}
	manager, err := New(Config{RuntimeDir: runtimeDir, PluginPath: pluginBin, PluginID: "echo", EventLogPath: logPath})
	if err != nil {
		t.Fatalf("new manager with default timeouts: %v", err)
	}
	defer manager.Close()
	if manager.cfg.DialTimeout == 0 || manager.cfg.CallTimeout == 0 || manager.cfg.HeartbeatEvery == 0 {
		t.Fatalf("manager defaults not populated: %+v", manager.cfg)
	}
}

func TestManagerStartReportsAuthFileWriteFailure(t *testing.T) {
	pluginBin := buildPlugin(t)
	runtimeDir := t.TempDir()
	logPath := filepath.Join(runtimeDir, "events.jsonl")
	manager, err := New(Config{RuntimeDir: runtimeDir, PluginPath: pluginBin, PluginID: "echo", EventLogPath: logPath})
	if err != nil {
		t.Fatalf("new manager: %v", err)
	}
	defer manager.Close()
	if err := os.RemoveAll(runtimeDir); err != nil {
		t.Fatalf("remove runtime dir before start: %v", err)
	}
	if err := manager.Start(); err == nil || !strings.Contains(err.Error(), "write auth token file") {
		t.Fatalf("expected auth token write failure, got %v", err)
	}
}

func TestManagerCallRejectsStaleSuccessResponse(t *testing.T) {
	left, right := net.Pipe()
	server := rpc.NewServer()
	if err := server.RegisterName(testpluginapi.ServiceName, testRPCService{}); err != nil {
		t.Fatalf("register test rpc service: %v", err)
	}
	go server.ServeConn(right)
	defer left.Close()

	client := &rpcClient{client: rpc.NewClient(left), conn: left}
	manager := &Manager{cfg: Config{PluginID: "echo", CallTimeout: time.Second}, client: client, state: State{PluginID: "echo", GenerationID: 2}, closedCh: make(chan struct{})}
	var out testpluginapi.EchoResponse
	if err := manager.call(client, 1, 0, testpluginapi.MethodEcho, testpluginapi.EchoRequest{Message: "stale"}, &out); err == nil || !strings.Contains(err.Error(), "stale response rejected") {
		t.Fatalf("expected stale response rejection, got %v", err)
	}
}

func TestStopCurrentLogsShutdownFailure(t *testing.T) {
	left, right := net.Pipe()
	server := rpc.NewServer()
	if err := server.RegisterName(testpluginapi.ServiceName, testRPCService{}); err != nil {
		t.Fatalf("register test rpc service: %v", err)
	}
	go server.ServeConn(right)

	client := &rpcClient{client: rpc.NewClient(left), conn: left}
	manager := &Manager{cfg: Config{PluginID: "echo", CallTimeout: time.Second}, client: client, state: State{PluginID: "echo", GenerationID: 1, Healthy: true}, closedCh: make(chan struct{})}
	if err := manager.stopCurrent(false, "test shutdown failure"); err != nil {
		t.Fatalf("stop current with shutdown rpc failure: %v", err)
	}
}

func TestCleanupRuntimeArtifactsReportsRemoveFailures(t *testing.T) {
	runtimeDir := t.TempDir()
	logPath := filepath.Join(runtimeDir, "events.jsonl")
	logger, err := eventlog.New(logPath)
	if err != nil {
		t.Fatalf("new event logger: %v", err)
	}
	manager := &Manager{cfg: Config{RuntimeDir: runtimeDir, PluginID: "echo"}, log: logger, closedCh: make(chan struct{})}
	manager.state = State{PluginID: "echo", GenerationID: 1, SocketPath: amruntime.SocketPath(runtimeDir, "echo")}

	socketPath := amruntime.SocketPath(runtimeDir, "echo")
	authPath := amruntime.AuthPath(runtimeDir, "echo")
	for _, path := range []string{socketPath, authPath} {
		if err := os.Mkdir(path, 0o700); err != nil {
			t.Fatalf("mkdir removal trap %s: %v", path, err)
		}
		if err := os.WriteFile(filepath.Join(path, "child"), []byte("still here"), 0o600); err != nil {
			t.Fatalf("write removal trap child: %v", err)
		}
	}

	manager.cleanupRuntimeArtifacts(1)
	if err := logger.Close(); err != nil {
		t.Fatalf("close event logger: %v", err)
	}
	events := readEvents(t, logPath)
	assertEvent(t, events, eventlog.EventRuntimeCleanupFailed, func(event eventlog.Event) bool {
		failed, ok := event.Details["failed"].(map[string]any)
		return ok && len(failed) == 2
	})
}
