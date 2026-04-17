// Package kernel provides process supervision for rpc_plugin_system plugins.
package kernel

import (
	"errors"
	"fmt"
	"io"
	"net"
	"net/rpc"
	"os"
	"os/exec"
	"sync"
	"syscall"
	"time"

	"rpc_plugin_system/internal/auth"
	"rpc_plugin_system/internal/eventlog"
	amruntime "rpc_plugin_system/internal/runtime"
	"rpc_plugin_system/internal/testpluginapi"
)

// Config defines how a plugin manager supervises one plugin executable.
type Config struct {
	RuntimeDir       string
	PluginPath       string
	PluginID         string
		DialTimeout      time.Duration
	CallTimeout      time.Duration
	HeartbeatEvery   time.Duration
	EventLogPath     string
}

// State reports the current known runtime state of a plugin.
type State struct {
	PluginID     string
	GenerationID uint64
	Healthy      bool
	Capabilities []string
	SocketPath   string
	PID          int
}

// rpcClient wraps one RPC client and its underlying connection so the manager
// can enforce per-call deadlines and tear down blocked calls by closing the
// socket.
type rpcClient struct {
	client *rpc.Client
	conn   net.Conn
}

var errRPCPoisoned = errors.New("rpc client poisoned")

// Manager supervises one plugin executable and its RPC connection.
type Manager struct {
	cfg       Config
	mu        sync.RWMutex
	state     State
	cmd       *exec.Cmd
	client    *rpcClient
	log       *eventlog.Logger
	closed    bool
	closedCh  chan struct{}
	closeOnce sync.Once
}

// New constructs a plugin manager.
func New(cfg Config) (*Manager, error) {
	if cfg.RuntimeDir == "" || cfg.PluginPath == "" || cfg.PluginID == "" || cfg.EventLogPath == "" {
		return nil, errors.New("runtime dir, plugin path, plugin id, and event log path are required")
	}
	if err := amruntime.ValidateExecutable(cfg.PluginPath); err != nil {
		return nil, err
	}
	if err := amruntime.EnsureDir(cfg.RuntimeDir); err != nil {
		return nil, err
	}
	logger, err := eventlog.New(cfg.EventLogPath)
	if err != nil {
		return nil, err
	}
	if cfg.DialTimeout == 0 {
		cfg.DialTimeout = 3 * time.Second
	}
	if cfg.CallTimeout == 0 {
		cfg.CallTimeout = 2 * time.Second
	}
	if cfg.HeartbeatEvery == 0 {
		cfg.HeartbeatEvery = 2 * time.Second
	}
	return &Manager{cfg: cfg, log: logger, closedCh: make(chan struct{})}, nil
}

// Close shuts down manager-owned resources.
func (m *Manager) Close() error {
	m.closeOnce.Do(func() {
		_ = m.stopCurrent(true)
		m.mu.Lock()
		m.closed = true
		close(m.closedCh)
		m.mu.Unlock()
		m.cleanupRuntimeArtifacts()
	})
	return m.log.Close()
}

// Start launches the configured plugin, authenticates it, and opens the RPC connection.
func (m *Manager) Start() error {
	m.mu.Lock()
	if m.closed {
		m.mu.Unlock()
		return errors.New("manager closed")
	}
	generation := m.state.GenerationID + 1
	m.state.GenerationID = generation
	m.mu.Unlock()

	socketPath := amruntime.SocketPath(m.cfg.RuntimeDir, m.cfg.PluginID)
	authPath := amruntime.AuthPath(m.cfg.RuntimeDir, m.cfg.PluginID)
	_ = os.Remove(socketPath)
	_ = os.Remove(authPath)

	token, err := auth.NewToken()
	if err != nil {
		return err
	}
	if err := os.WriteFile(authPath, token, 0o600); err != nil {
		return fmt.Errorf("write auth token file: %w", err)
	}

	cmd := exec.Command(m.cfg.PluginPath)
	cmd.Env = append(os.Environ(),
		"RPC_PLUGIN_SYSTEM_PLUGIN_SOCKET="+socketPath,
		"RPC_PLUGIN_SYSTEM_PLUGIN_ID="+m.cfg.PluginID,
		fmt.Sprintf("RPC_PLUGIN_SYSTEM_PLUGIN_GENERATION=%d", generation),
		"RPC_PLUGIN_SYSTEM_AUTH_TOKEN_FILE="+authPath,
	)
	if err := cmd.Start(); err != nil {
		m.mu.Lock()
		m.state.GenerationID = generation - 1
		m.mu.Unlock()
		return fmt.Errorf("start plugin: %w", err)
	}

	client, err := m.dial(socketPath)
	if err != nil {
		m.cleanupFailedStart(cmd, nil, generation)
		return err
	}
	var authResp testpluginapi.AuthResponse
	if err := m.call(client, generation, testpluginapi.MethodAuth, testpluginapi.AuthRequest{Token: string(token)}, &authResp); err != nil {
		m.cleanupFailedStart(cmd, client, generation)
		return fmt.Errorf("auth rpc: %w", err)
	}
	if authResp.PluginID != m.cfg.PluginID {
		m.cleanupFailedStart(cmd, client, generation)
		return fmt.Errorf("plugin id mismatch: got %q want %q", authResp.PluginID, m.cfg.PluginID)
	}
	if authResp.GenerationID != generation {
		m.cleanupFailedStart(cmd, client, generation)
		return fmt.Errorf("generation mismatch: got %d want %d", authResp.GenerationID, generation)
	}
	_ = os.Remove(authPath)

	var caps testpluginapi.CapabilitiesResponse
	if err := m.call(client, generation, testpluginapi.MethodCapabilities, testpluginapi.Empty{}, &caps); err != nil {
		m.cleanupFailedStart(cmd, client, generation)
		return fmt.Errorf("capabilities rpc: %w", err)
	}
	if caps.PluginID != m.cfg.PluginID {
		m.cleanupFailedStart(cmd, client, generation)
		return fmt.Errorf("plugin id mismatch: got %q want %q", caps.PluginID, m.cfg.PluginID)
	}
	if caps.GenerationID != generation {
		m.cleanupFailedStart(cmd, client, generation)
		return fmt.Errorf("generation mismatch: got %d want %d", caps.GenerationID, generation)
	}

	m.mu.Lock()
	m.cmd = cmd
	m.client = client
	m.state = State{
		PluginID:     caps.PluginID,
		GenerationID: caps.GenerationID,
		Healthy:      true,
		Capabilities: caps.Capabilities,
		SocketPath:   socketPath,
		PID:          cmd.Process.Pid,
	}
	m.mu.Unlock()
	_ = m.log.Write(eventlog.Event{Type: "plugin_started", Plugin: m.cfg.PluginID, Message: fmt.Sprintf("pid=%d generation=%d", cmd.Process.Pid, generation)})
	return nil
}

// Heartbeat queries plugin health.
func (m *Manager) Heartbeat() (testpluginapi.HeartbeatResponse, error) {
	m.mu.RLock()
	client := m.client
	generation := m.state.GenerationID
	closed := m.closed
	m.mu.RUnlock()
	if closed {
		return testpluginapi.HeartbeatResponse{}, errors.New("manager closed")
	}
	var out testpluginapi.HeartbeatResponse
	if client == nil {
		return out, errors.New("plugin not started")
	}
	if err := m.call(client, generation, testpluginapi.MethodHeartbeat, testpluginapi.Empty{}, &out); err != nil {
		return out, err
	}
	m.mu.Lock()
	m.state.Healthy = out.Status == testpluginapi.StatusHealthy
	m.mu.Unlock()
	return out, nil
}

// Echo sends an echo request to the plugin.
func (m *Manager) Echo(message string) (string, error) {
	m.mu.RLock()
	client := m.client
	generation := m.state.GenerationID
	closed := m.closed
	m.mu.RUnlock()
	if closed {
		return "", errors.New("manager closed")
	}
	if client == nil {
		return "", errors.New("plugin not started")
	}
	var out testpluginapi.EchoResponse
	if err := m.call(client, generation, testpluginapi.MethodEcho, testpluginapi.EchoRequest{Message: message}, &out); err != nil {
		return "", err
	}
	return out.Message, nil
}

// Sleep calls the plugin sleep method.
func (m *Manager) Sleep(duration time.Duration) error {
	m.mu.RLock()
	client := m.client
	generation := m.state.GenerationID
	closed := m.closed
	m.mu.RUnlock()
	if closed {
		return errors.New("manager closed")
	}
	if client == nil {
		return errors.New("plugin not started")
	}
	return m.call(client, generation, testpluginapi.MethodSleep, testpluginapi.SleepRequest{Duration: duration}, &testpluginapi.Empty{})
}

// Crash asks the plugin to terminate itself with one exit code.
func (m *Manager) Crash(code int) error {
	m.mu.RLock()
	client := m.client
	generation := m.state.GenerationID
	closed := m.closed
	m.mu.RUnlock()
	if closed {
		return errors.New("manager closed")
	}
	if client == nil {
		return errors.New("plugin not started")
	}
	return m.call(client, generation, testpluginapi.MethodCrash, testpluginapi.CrashRequest{Code: code}, &testpluginapi.Empty{})
}

// Restart kills the current plugin process and starts a fresh generation.
func (m *Manager) Restart() error {
	m.mu.RLock()
	closed := m.closed
	m.mu.RUnlock()
	if closed {
		return errors.New("manager closed")
	}
	if err := m.Kill(); err != nil {
		return err
	}
	return m.Start()
}

// Kill terminates the current plugin process and tears down its RPC connection.
func (m *Manager) Kill() error {
	return m.stopCurrent(false)
}

// State returns a snapshot of the current manager state.
func (m *Manager) State() State {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.state
}

func (m *Manager) dial(socketPath string) (*rpcClient, error) {
	deadline := time.Now().Add(m.cfg.DialTimeout)
	for time.Now().Before(deadline) {
		conn, err := net.DialTimeout("unix", socketPath, 200*time.Millisecond)
		if err == nil {
			return &rpcClient{client: rpc.NewClient(conn), conn: conn}, nil
		}
		time.Sleep(50 * time.Millisecond)
	}
	return nil, fmt.Errorf("dial plugin socket timeout: %s", socketPath)
}

func (m *Manager) call(client *rpcClient, generation uint64, method string, args any, reply any) error {
	if client == nil {
		return errors.New("rpc client unavailable")
	}
	resultCh := make(chan error, 1)
	go func() {
		resultCh <- client.client.Call(method, args, reply)
	}()

	select {
	case err := <-resultCh:
		if err != nil {
			if isRPCPoisonError(err) {
				m.poisonClient(client)
			}
			return err
		}
		m.mu.RLock()
		currentGeneration := m.state.GenerationID
		m.mu.RUnlock()
		if generation != currentGeneration {
			return fmt.Errorf("stale response rejected: generation=%d current=%d", generation, currentGeneration)
		}
		return nil
	case <-time.After(m.cfg.CallTimeout):
		m.poisonClient(client)
		return fmt.Errorf("rpc timeout calling %s", method)
	}
}

func (m *Manager) stopCurrent(markClosed bool) error {
	m.mu.Lock()
	if m.closed {
		m.mu.Unlock()
		return errors.New("manager closed")
	}
	client := m.client
	cmd := m.cmd
	generation := m.state.GenerationID
	m.client = nil
	m.cmd = nil
	m.state.Healthy = false
	m.state.PID = 0
	if markClosed {
		m.closed = true
	}
	m.mu.Unlock()

	if client != nil {
		if err := m.call(client, generation, testpluginapi.MethodShutdown, testpluginapi.Empty{}, &testpluginapi.Empty{}); err != nil {
			_ = m.log.Write(eventlog.Event{Type: "plugin_shutdown_failed", Plugin: m.cfg.PluginID, Message: err.Error()})
		}
		_ = client.close()
	}
	if cmd != nil && cmd.Process != nil {
		waitCh := make(chan error, 1)
		go func() {
			_, err := cmd.Process.Wait()
			waitCh <- err
		}()
		select {
		case err := <-waitCh:
			if err != nil && !errors.Is(err, os.ErrProcessDone) {
				return fmt.Errorf("wait plugin exit: %w", err)
			}
		case <-time.After(250 * time.Millisecond):
			if err := cmd.Process.Signal(syscall.SIGKILL); err != nil && !errors.Is(err, os.ErrProcessDone) {
				return fmt.Errorf("kill plugin: %w", err)
			}
			if err := <-waitCh; err != nil && !errors.Is(err, os.ErrProcessDone) {
				return fmt.Errorf("wait plugin exit after kill: %w", err)
			}
			_ = m.log.Write(eventlog.Event{Type: "plugin_killed", Plugin: m.cfg.PluginID})
			m.cleanupRuntimeArtifacts()
			return nil
		}
	}
	_ = m.log.Write(eventlog.Event{Type: "plugin_stopped", Plugin: m.cfg.PluginID})
	m.cleanupRuntimeArtifacts()
	return nil
}

func (m *Manager) cleanupFailedStart(cmd *exec.Cmd, client *rpcClient, generation uint64) {
	if client != nil {
		_ = client.close()
	}
	if cmd != nil && cmd.Process != nil {
		_ = cmd.Process.Kill()
		_, _ = cmd.Process.Wait()
	}
	m.mu.Lock()
	if m.state.GenerationID == generation {
		m.state.GenerationID = generation - 1
	}
	m.mu.Unlock()
	m.cleanupRuntimeArtifacts()
}

func (c *rpcClient) close() error {
	if c == nil {
		return nil
	}
	if c.conn != nil {
		_ = c.conn.Close()
	}
	if c.client != nil {
		return c.client.Close()
	}
	return nil
}

func (m *Manager) poisonClient(client *rpcClient) {
	if client == nil {
		return
	}
	m.mu.Lock()
	if m.client == client {
		m.client = nil
		m.state.Healthy = false
		m.state.PID = 0
	}
	m.mu.Unlock()
	_ = client.close()
}

func isRPCPoisonError(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, rpc.ErrShutdown) || errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF) || errors.Is(err, net.ErrClosed) {
		return true
	}
	var netErr net.Error
	if errors.As(err, &netErr) {
		return true
	}
	var opErr *net.OpError
	return errors.As(err, &opErr)
}

func (m *Manager) cleanupRuntimeArtifacts() {
	socketPath := amruntime.SocketPath(m.cfg.RuntimeDir, m.cfg.PluginID)
	authPath := amruntime.AuthPath(m.cfg.RuntimeDir, m.cfg.PluginID)
	_ = os.Remove(socketPath)
	_ = os.Remove(authPath)
	}
