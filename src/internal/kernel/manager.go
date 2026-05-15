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
	"rpc_plugin_system/internal/providerbundle"
	amruntime "rpc_plugin_system/internal/runtime"
	"rpc_plugin_system/test/testpluginapi"
)

// Config defines how a plugin manager supervises one plugin executable.
type Config struct {
	RuntimeDir     string
	PluginPath     string
	PluginID       string
	DialTimeout    time.Duration
	CallTimeout    time.Duration
	HeartbeatEvery time.Duration
	EventLogPath   string

	// ProviderBundleMetadata is inert declared bundle metadata for this plugin.
	// It is projection input only; it is not authority and is exposed only when
	// the plugin id and generation match the current manager generation.
	ProviderBundleMetadata *providerbundle.ProviderBundleMetadata

	// ProviderBundleMetadataRefresh is an explicit inert config refresh path for
	// declared bundle metadata keyed by authenticated plugin generation. It does
	// not execute provider code, admit bundle surfaces, or mint authority.
	ProviderBundleMetadataRefresh *ProviderBundleMetadataRefreshConfig

	// DisableProviderBundleAdminPathRedaction is an explicit unsafe/debug escape
	// hatch. The default redacts host-private paths from admin DTOs.
	DisableProviderBundleAdminPathRedaction bool
}

// ProviderBundleMetadataRefreshConfig carries explicit generation-keyed inert
// metadata supplied by host configuration. The manager treats it as projection
// input only.
type ProviderBundleMetadataRefreshConfig struct {
	ByGeneration map[int64]providerbundle.ProviderBundleMetadata
}

// State reports the current known runtime state of a plugin.
//
// PID is the currently observed plugin process id when one is known to the
// manager. A false Healthy value means the generation is not currently trusted
// for routed work. It does not necessarily prove the process has already exited.
type State struct {
	PluginID     string
	GenerationID uint64
	Healthy      bool
	Capabilities []string
	SocketPath   string
	PID          int

	// DeclaredProviderBundleMetadata is admin-safe inert provider bundle
	// metadata. Nil preserves the existing no-bundle status shape.
	DeclaredProviderBundleMetadata *providerbundle.ProviderBundleAdminDTO `json:",omitempty"`
}

// CoreState reports core-facing state facts. It deliberately does not reuse
// State because admin status must not carry raw core-only projection material.
type CoreState struct {
	PluginID     string
	GenerationID uint64
	Healthy      bool

	DeclaredProviderBundleMetadata *providerbundle.DeclaredProviderBundleCoreDTO `json:",omitempty"`
}

// rpcClient wraps one RPC client and its underlying connection so the manager
// can enforce per-call deadlines and tear down blocked calls by closing the
// socket.
type rpcClient struct {
	client *rpc.Client
	conn   net.Conn
}

var errRPCPoisoned = errors.New("rpc client poisoned")

var (
	newBootstrapToken = auth.NewToken
	managerRPCCall    = func(m *Manager, client *rpcClient, generation, epoch uint64, method string, args any, reply any) error {
		return m.call(client, generation, epoch, method, args, reply)
	}
	verifyManagerPeerCred = func(m *Manager, client *rpcClient, wantPID int, generation uint64) error {
		return m.verifyPeerCred(client, wantPID, generation)
	}
	waitProcess = func(process *os.Process) (*os.ProcessState, error) {
		return process.Wait()
	}
	signalProcess = func(process *os.Process, signal os.Signal) error {
		return process.Signal(signal)
	}
	stopManagerCurrent = func(m *Manager, markClosed bool, reason string) error {
		return m.stopCurrent(markClosed, reason)
	}
)

// Manager supervises one plugin executable and its RPC connection.
type Manager struct {
	cfg         Config
	lifecycleMu sync.Mutex
	mu          sync.RWMutex
	state       State
	cmd         *exec.Cmd
	client      *rpcClient
	rpcEpoch    uint64
	log         *eventlog.Logger
	closed      bool
	closedCh    chan struct{}
	closeOnce   sync.Once
	closeErr    error
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
	if cfg.ProviderBundleMetadata != nil {
		metadata := cfg.ProviderBundleMetadata.Clone()
		cfg.ProviderBundleMetadata = &metadata
	}
	cfg.ProviderBundleMetadataRefresh = cloneProviderBundleMetadataRefresh(cfg.ProviderBundleMetadataRefresh)
	manager := &Manager{cfg: cfg, log: logger, closedCh: make(chan struct{})}
	manager.logEvent(eventlog.Event{
		Level:      eventlog.LevelInfo,
		Component:  eventlog.ComponentKernel,
		Event:      eventlog.EventManagerInitialized,
		PluginID:   cfg.PluginID,
		Message:    "manager initialized",
		SocketPath: amruntime.SocketPath(cfg.RuntimeDir, cfg.PluginID),
		Details: map[string]any{
			"plugin_path": cfg.PluginPath,
			"runtime_dir": cfg.RuntimeDir,
		},
	})
	return manager, nil
}

// Close shuts down manager-owned resources.
func (m *Manager) Close() error {
	m.lifecycleMu.Lock()
	defer m.lifecycleMu.Unlock()
	m.closeOnce.Do(func() {
		m.closeErr = stopManagerCurrent(m, true, "manager close")
		m.mu.Lock()
		if !m.closed {
			m.closed = true
			m.rpcEpoch++
		}
		close(m.closedCh)
		m.mu.Unlock()
		m.cleanupRuntimeArtifacts(0)
		if err := m.log.Close(); m.closeErr == nil {
			m.closeErr = err
		}
	})
	return m.closeErr
}

// Start launches the configured plugin, authenticates it, and opens the RPC connection.
func (m *Manager) Start() error {
	m.lifecycleMu.Lock()
	defer m.lifecycleMu.Unlock()
	return m.startLocked()
}

func (m *Manager) startLocked() error {
	m.mu.Lock()
	if m.closed {
		m.mu.Unlock()
		return errors.New("manager closed")
	}
	if m.cmd != nil || m.client != nil {
		m.mu.Unlock()
		return errors.New("plugin already started")
	}
	generation := m.state.GenerationID + 1
	socketPath := amruntime.SocketPath(m.cfg.RuntimeDir, m.cfg.PluginID)
	authPath := amruntime.AuthPath(m.cfg.RuntimeDir, m.cfg.PluginID)
	m.state = State{
		PluginID:     m.cfg.PluginID,
		GenerationID: generation,
		SocketPath:   socketPath,
	}
	m.mu.Unlock()

	_ = os.Remove(socketPath)
	_ = os.Remove(authPath)

	m.logEvent(eventlog.Event{
		Level:        eventlog.LevelInfo,
		Component:    eventlog.ComponentKernel,
		Event:        eventlog.EventPluginStartRequested,
		PluginID:     m.cfg.PluginID,
		GenerationID: generation,
		SocketPath:   socketPath,
		Message:      "starting plugin process",
		Details: map[string]any{
			"plugin_path": m.cfg.PluginPath,
		},
	})

	token, err := newBootstrapToken()
	if err != nil {
		m.rollbackGeneration(generation)
		m.logEvent(eventlog.Event{
			Level:        eventlog.LevelError,
			Component:    eventlog.ComponentAuth,
			Event:        eventlog.EventPluginStartFailed,
			PluginID:     m.cfg.PluginID,
			GenerationID: generation,
			SocketPath:   socketPath,
			Message:      "failed to create bootstrap token",
			Error:        err.Error(),
		})
		return err
	}
	if err := os.WriteFile(authPath, token, 0o600); err != nil {
		m.rollbackGeneration(generation)
		wrapped := fmt.Errorf("write auth token file: %w", err)
		m.logEvent(eventlog.Event{
			Level:        eventlog.LevelError,
			Component:    eventlog.ComponentAuth,
			Event:        eventlog.EventPluginStartFailed,
			PluginID:     m.cfg.PluginID,
			GenerationID: generation,
			SocketPath:   socketPath,
			Message:      "failed to write bootstrap token file",
			Error:        wrapped.Error(),
		})
		return wrapped
	}

	cmd := exec.Command(m.cfg.PluginPath)
	cmd.Env = []string{
		"RPC_PLUGIN_SYSTEM_PLUGIN_SOCKET=" + socketPath,
		"RPC_PLUGIN_SYSTEM_PLUGIN_ID=" + m.cfg.PluginID,
		fmt.Sprintf("RPC_PLUGIN_SYSTEM_PLUGIN_GENERATION=%d", generation),
		"RPC_PLUGIN_SYSTEM_AUTH_TOKEN_FILE=" + authPath,
	}
	if behaviorPath := os.Getenv(testpluginapi.BehaviorConfigEnv); behaviorPath != "" {
		cmd.Env = append(cmd.Env, testpluginapi.BehaviorConfigEnv+"="+behaviorPath)
	}
	if shutdownMarker := os.Getenv(testpluginapi.ShutdownMarkerEnv); shutdownMarker != "" {
		cmd.Env = append(cmd.Env, testpluginapi.ShutdownMarkerEnv+"="+shutdownMarker)
	}
	if err := cmd.Start(); err != nil {
		m.rollbackGeneration(generation)
		wrapped := fmt.Errorf("start plugin: %w", err)
		m.logEvent(eventlog.Event{
			Level:        eventlog.LevelError,
			Component:    eventlog.ComponentKernel,
			Event:        eventlog.EventPluginStartFailed,
			PluginID:     m.cfg.PluginID,
			GenerationID: generation,
			SocketPath:   socketPath,
			Message:      "plugin process failed to start",
			Error:        wrapped.Error(),
		})
		return wrapped
	}

	client, err := m.dial(socketPath, generation)
	if err != nil {
		m.cleanupFailedStart(cmd, nil, generation, "dial failure", err)
		return err
	}
	if err := verifyManagerPeerCred(m, client, cmd.Process.Pid, generation); err != nil {
		m.logEvent(eventlog.Event{
			Level:        eventlog.LevelError,
			Component:    eventlog.ComponentAuth,
			Event:        eventlog.EventAuthFailed,
			PluginID:     m.cfg.PluginID,
			GenerationID: generation,
			PID:          cmd.Process.Pid,
			SocketPath:   socketPath,
			Message:      "plugin peer credentials verification failed",
			Error:        err.Error(),
		})
		m.cleanupFailedStart(cmd, client, generation, "peer credential mismatch", err)
		return err
	}

	m.logEvent(eventlog.Event{
		Level:        eventlog.LevelInfo,
		Component:    eventlog.ComponentAuth,
		Event:        eventlog.EventAuthStarted,
		PluginID:     m.cfg.PluginID,
		GenerationID: generation,
		PID:          cmd.Process.Pid,
		SocketPath:   socketPath,
		Message:      "authenticating plugin bootstrap token",
	})
	var authResp testpluginapi.AuthResponse
	if err := managerRPCCall(m, client, generation, 0, testpluginapi.MethodAuth, testpluginapi.AuthRequest{Token: string(token)}, &authResp); err != nil {
		wrapped := fmt.Errorf("auth rpc: %w", err)
		m.logEvent(eventlog.Event{
			Level:        eventlog.LevelError,
			Component:    eventlog.ComponentAuth,
			Event:        eventlog.EventAuthFailed,
			PluginID:     m.cfg.PluginID,
			GenerationID: generation,
			PID:          cmd.Process.Pid,
			SocketPath:   socketPath,
			Message:      "plugin authentication failed",
			Error:        wrapped.Error(),
		})
		m.cleanupFailedStart(cmd, client, generation, "auth rpc failure", wrapped)
		return wrapped
	}
	if authResp.PluginID != m.cfg.PluginID {
		err := fmt.Errorf("plugin id mismatch: got %q want %q", authResp.PluginID, m.cfg.PluginID)
		m.logEvent(eventlog.Event{
			Level:        eventlog.LevelError,
			Component:    eventlog.ComponentAuth,
			Event:        eventlog.EventAuthFailed,
			PluginID:     m.cfg.PluginID,
			GenerationID: generation,
			PID:          cmd.Process.Pid,
			SocketPath:   socketPath,
			Message:      "plugin authentication returned wrong plugin id",
			Error:        err.Error(),
		})
		m.cleanupFailedStart(cmd, client, generation, "auth plugin id mismatch", err)
		return err
	}
	if authResp.GenerationID != generation {
		err := fmt.Errorf("generation mismatch: got %d want %d", authResp.GenerationID, generation)
		m.logEvent(eventlog.Event{
			Level:        eventlog.LevelError,
			Component:    eventlog.ComponentAuth,
			Event:        eventlog.EventAuthFailed,
			PluginID:     m.cfg.PluginID,
			GenerationID: generation,
			PID:          cmd.Process.Pid,
			SocketPath:   socketPath,
			Message:      "plugin authentication returned wrong generation",
			Error:        err.Error(),
		})
		m.cleanupFailedStart(cmd, client, generation, "auth generation mismatch", err)
		return err
	}
	m.logEvent(eventlog.Event{
		Level:        eventlog.LevelInfo,
		Component:    eventlog.ComponentAuth,
		Event:        eventlog.EventAuthSucceeded,
		PluginID:     m.cfg.PluginID,
		GenerationID: generation,
		PID:          cmd.Process.Pid,
		SocketPath:   socketPath,
		Message:      "plugin authentication succeeded",
	})
	_ = os.Remove(authPath)

	m.logEvent(eventlog.Event{
		Level:        eventlog.LevelInfo,
		Component:    eventlog.ComponentKernel,
		Event:        eventlog.EventCapabilitiesStarted,
		PluginID:     m.cfg.PluginID,
		GenerationID: generation,
		PID:          cmd.Process.Pid,
		SocketPath:   socketPath,
		Message:      "loading plugin capabilities",
	})
	var caps testpluginapi.CapabilitiesResponse
	if err := managerRPCCall(m, client, generation, 0, testpluginapi.MethodCapabilities, testpluginapi.Empty{}, &caps); err != nil {
		wrapped := fmt.Errorf("capabilities rpc: %w", err)
		m.logEvent(eventlog.Event{
			Level:        eventlog.LevelError,
			Component:    eventlog.ComponentKernel,
			Event:        eventlog.EventCapabilitiesFailed,
			PluginID:     m.cfg.PluginID,
			GenerationID: generation,
			PID:          cmd.Process.Pid,
			SocketPath:   socketPath,
			Message:      "loading plugin capabilities failed",
			Error:        wrapped.Error(),
		})
		m.cleanupFailedStart(cmd, client, generation, "capabilities rpc failure", wrapped)
		return wrapped
	}
	if caps.PluginID != m.cfg.PluginID {
		err := fmt.Errorf("plugin id mismatch: got %q want %q", caps.PluginID, m.cfg.PluginID)
		m.logEvent(eventlog.Event{
			Level:        eventlog.LevelError,
			Component:    eventlog.ComponentKernel,
			Event:        eventlog.EventCapabilitiesFailed,
			PluginID:     m.cfg.PluginID,
			GenerationID: generation,
			PID:          cmd.Process.Pid,
			SocketPath:   socketPath,
			Message:      "capabilities returned wrong plugin id",
			Error:        err.Error(),
		})
		m.cleanupFailedStart(cmd, client, generation, "capabilities plugin id mismatch", err)
		return err
	}
	if caps.GenerationID != generation {
		err := fmt.Errorf("generation mismatch: got %d want %d", caps.GenerationID, generation)
		m.logEvent(eventlog.Event{
			Level:        eventlog.LevelError,
			Component:    eventlog.ComponentKernel,
			Event:        eventlog.EventCapabilitiesFailed,
			PluginID:     m.cfg.PluginID,
			GenerationID: generation,
			PID:          cmd.Process.Pid,
			SocketPath:   socketPath,
			Message:      "capabilities returned wrong generation",
			Error:        err.Error(),
		})
		m.cleanupFailedStart(cmd, client, generation, "capabilities generation mismatch", err)
		return err
	}
	for _, cap := range caps.Capabilities {
		if cap == "__capabilities_error__" {
			err := fmt.Errorf("capabilities rpc: capabilities failure requested")
			m.logEvent(eventlog.Event{
				Level:        eventlog.LevelError,
				Component:    eventlog.ComponentKernel,
				Event:        eventlog.EventCapabilitiesFailed,
				PluginID:     m.cfg.PluginID,
				GenerationID: generation,
				PID:          cmd.Process.Pid,
				SocketPath:   socketPath,
				Message:      "plugin requested synthetic capabilities failure",
				Error:        err.Error(),
			})
			m.cleanupFailedStart(cmd, client, generation, "capabilities sentinel failure", err)
			return err
		}
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
	m.logEvent(eventlog.Event{
		Level:        eventlog.LevelInfo,
		Component:    eventlog.ComponentKernel,
		Event:        eventlog.EventCapabilitiesLoaded,
		PluginID:     caps.PluginID,
		GenerationID: caps.GenerationID,
		PID:          cmd.Process.Pid,
		SocketPath:   socketPath,
		Message:      "plugin capabilities loaded",
		Details: map[string]any{
			"capabilities": caps.Capabilities,
		},
	})
	m.logEvent(eventlog.Event{
		Level:        eventlog.LevelInfo,
		Component:    eventlog.ComponentKernel,
		Event:        eventlog.EventPluginStarted,
		PluginID:     caps.PluginID,
		GenerationID: caps.GenerationID,
		PID:          cmd.Process.Pid,
		SocketPath:   socketPath,
		Message:      fmt.Sprintf("plugin started and healthy, capabilities=%v", caps.Capabilities),
	})
	return nil
}

// Heartbeat queries plugin health.
func (m *Manager) Heartbeat() (testpluginapi.HeartbeatResponse, error) {
	m.mu.RLock()
	client := m.client
	generation := m.state.GenerationID
	epoch := m.rpcEpoch
	closed := m.closed
	pid := m.state.PID
	socketPath := m.state.SocketPath
	m.mu.RUnlock()
	if closed {
		return testpluginapi.HeartbeatResponse{}, errors.New("manager closed")
	}
	var out testpluginapi.HeartbeatResponse
	if client == nil {
		return out, errors.New("plugin not started")
	}
	if err := managerRPCCall(m, client, generation, epoch, testpluginapi.MethodHeartbeat, testpluginapi.Empty{}, &out); err != nil {
		m.logEvent(eventlog.Event{
			Level:        eventlog.LevelWarn,
			Component:    eventlog.ComponentKernel,
			Event:        eventlog.EventHeartbeatFailed,
			PluginID:     m.cfg.PluginID,
			GenerationID: generation,
			PID:          pid,
			SocketPath:   socketPath,
			Method:       testpluginapi.MethodHeartbeat,
			Message:      "heartbeat failed",
			Error:        err.Error(),
		})
		return out, err
	}
	healthy := out.Status == testpluginapi.StatusHealthy
	m.mu.Lock()
	if m.closed || m.client != client || m.rpcEpoch != epoch || m.state.GenerationID != generation {
		m.mu.Unlock()
		return out, fmt.Errorf("stale heartbeat rejected: generation=%d epoch=%d", generation, epoch)
	}
	wasHealthy := m.state.Healthy
	m.state.Healthy = healthy
	m.mu.Unlock()
	if healthy {
		if !wasHealthy {
			m.logEvent(eventlog.Event{
				Level:        eventlog.LevelInfo,
				Component:    eventlog.ComponentKernel,
				Event:        eventlog.EventHeartbeatHealthy,
				PluginID:     m.cfg.PluginID,
				GenerationID: generation,
				PID:          pid,
				SocketPath:   socketPath,
				Method:       testpluginapi.MethodHeartbeat,
				Message:      "heartbeat transitioned to healthy",
			})
		}
	} else {
		m.logEvent(eventlog.Event{
			Level:        eventlog.LevelWarn,
			Component:    eventlog.ComponentKernel,
			Event:        eventlog.EventHeartbeatUnhealthy,
			PluginID:     m.cfg.PluginID,
			GenerationID: generation,
			PID:          pid,
			SocketPath:   socketPath,
			Method:       testpluginapi.MethodHeartbeat,
			Message:      "heartbeat returned unhealthy status",
			Reason:       string(out.Status),
		})
	}
	return out, nil
}

// Echo sends an echo request to the plugin.
func (m *Manager) Echo(message string) (string, error) {
	m.mu.RLock()
	client := m.client
	generation := m.state.GenerationID
	epoch := m.rpcEpoch
	closed := m.closed
	m.mu.RUnlock()
	if closed {
		return "", errors.New("manager closed")
	}
	if client == nil {
		return "", errors.New("plugin not started")
	}
	var out testpluginapi.EchoResponse
	if err := managerRPCCall(m, client, generation, epoch, testpluginapi.MethodEcho, testpluginapi.EchoRequest{Message: message}, &out); err != nil {
		return "", err
	}
	return out.Message, nil
}

// Sleep calls the plugin sleep method.
func (m *Manager) Sleep(duration time.Duration) error {
	m.mu.RLock()
	client := m.client
	generation := m.state.GenerationID
	epoch := m.rpcEpoch
	closed := m.closed
	m.mu.RUnlock()
	if closed {
		return errors.New("manager closed")
	}
	if client == nil {
		return errors.New("plugin not started")
	}
	return managerRPCCall(m, client, generation, epoch, testpluginapi.MethodSleep, testpluginapi.SleepRequest{Duration: duration}, &testpluginapi.Empty{})
}

// Crash asks the plugin to terminate itself with one exit code.
func (m *Manager) Crash(code int) error {
	m.mu.RLock()
	client := m.client
	generation := m.state.GenerationID
	epoch := m.rpcEpoch
	closed := m.closed
	m.mu.RUnlock()
	if closed {
		return errors.New("manager closed")
	}
	if client == nil {
		return errors.New("plugin not started")
	}
	return managerRPCCall(m, client, generation, epoch, testpluginapi.MethodCrash, testpluginapi.CrashRequest{Code: code}, &testpluginapi.Empty{})
}

// Restart kills the current plugin process and starts a fresh generation.
func (m *Manager) Restart() error {
	m.lifecycleMu.Lock()
	defer m.lifecycleMu.Unlock()

	m.mu.RLock()
	closed := m.closed
	generation := m.state.GenerationID
	pid := m.state.PID
	socketPath := m.state.SocketPath
	m.mu.RUnlock()
	if closed {
		return errors.New("manager closed")
	}
	m.logEvent(eventlog.Event{
		Level:        eventlog.LevelWarn,
		Component:    eventlog.ComponentKernel,
		Event:        eventlog.EventRestartRequested,
		PluginID:     m.cfg.PluginID,
		GenerationID: generation,
		PID:          pid,
		SocketPath:   socketPath,
		Message:      "restart requested",
	})
	if err := m.stopCurrent(false, "restart requested"); err != nil {
		m.logEvent(eventlog.Event{
			Level:        eventlog.LevelError,
			Component:    eventlog.ComponentKernel,
			Event:        eventlog.EventRestartFailed,
			PluginID:     m.cfg.PluginID,
			GenerationID: generation,
			PID:          pid,
			SocketPath:   socketPath,
			Message:      "restart failed during stop phase",
			Error:        err.Error(),
		})
		return err
	}
	if err := m.startLocked(); err != nil {
		m.logEvent(eventlog.Event{
			Level:        eventlog.LevelError,
			Component:    eventlog.ComponentKernel,
			Event:        eventlog.EventRestartFailed,
			PluginID:     m.cfg.PluginID,
			GenerationID: generation,
			PID:          pid,
			SocketPath:   socketPath,
			Message:      "restart failed during start phase",
			Error:        err.Error(),
		})
		return err
	}
	state := m.State()
	m.logEvent(eventlog.Event{
		Level:        eventlog.LevelInfo,
		Component:    eventlog.ComponentKernel,
		Event:        eventlog.EventRestartSucceeded,
		PluginID:     state.PluginID,
		GenerationID: state.GenerationID,
		PID:          state.PID,
		SocketPath:   state.SocketPath,
		Message:      "restart succeeded",
	})
	return nil
}

// Kill terminates the current plugin process and tears down its RPC connection.
func (m *Manager) Kill() error {
	m.lifecycleMu.Lock()
	defer m.lifecycleMu.Unlock()
	return m.stopCurrent(false, "kill requested")
}

// State returns a snapshot of the current manager state.
func (m *Manager) State() State {
	m.mu.RLock()
	defer m.mu.RUnlock()
	state := m.state
	state.Capabilities = append([]string(nil), state.Capabilities...)
	state.DeclaredProviderBundleMetadata = m.providerBundleAdminDTOLocked(state)
	return state
}

// CoreState returns core-facing inert metadata facts for this manager. It does
// not expose admin-only process/socket/path detail.
func (m *Manager) CoreState() CoreState {
	m.mu.RLock()
	defer m.mu.RUnlock()
	state := CoreState{
		PluginID:     m.state.PluginID,
		GenerationID: m.state.GenerationID,
		Healthy:      m.state.Healthy,
	}
	state.DeclaredProviderBundleMetadata = m.providerBundleCoreDTOLocked(m.state)
	return state
}

func (m *Manager) providerBundleAdminDTOLocked(state State) *providerbundle.ProviderBundleAdminDTO {
	metadata := m.generationBoundProviderBundleMetadataLocked(state)
	if metadata == nil {
		return nil
	}
	dto := metadata.AdminDTO(!m.cfg.DisableProviderBundleAdminPathRedaction)
	return &dto
}

func (m *Manager) providerBundleCoreDTOLocked(state State) *providerbundle.DeclaredProviderBundleCoreDTO {
	metadata := m.generationBoundProviderBundleMetadataLocked(state)
	if metadata == nil {
		return nil
	}
	dto := metadata.CoreDTO()
	return &dto
}

func (m *Manager) generationBoundProviderBundleMetadataLocked(state State) *providerbundle.ProviderBundleMetadata {
	if m.cfg.ProviderBundleMetadataRefresh != nil {
		if metadata, ok := m.cfg.ProviderBundleMetadataRefresh.ByGeneration[int64(state.GenerationID)]; ok {
			metadata = metadata.Clone()
			if metadata.PluginID == state.PluginID && metadata.PluginGeneration == int64(state.GenerationID) {
				return &metadata
			}
		}
	}
	if m.cfg.ProviderBundleMetadata == nil {
		return nil
	}
	metadata := m.cfg.ProviderBundleMetadata.Clone()
	if metadata.PluginID != state.PluginID || metadata.PluginGeneration != int64(state.GenerationID) {
		return nil
	}
	return &metadata
}

func (m *Manager) dial(socketPath string, generation uint64) (*rpcClient, error) {
	m.logEvent(eventlog.Event{
		Level:        eventlog.LevelInfo,
		Component:    eventlog.ComponentRPC,
		Event:        eventlog.EventPluginDialStarted,
		PluginID:     m.cfg.PluginID,
		GenerationID: generation,
		SocketPath:   socketPath,
		Message:      "dialing plugin socket",
	})
	deadline := time.Now().Add(m.cfg.DialTimeout)
	for time.Now().Before(deadline) {
		conn, err := net.DialTimeout("unix", socketPath, 200*time.Millisecond)
		if err == nil {
			m.logEvent(eventlog.Event{
				Level:        eventlog.LevelInfo,
				Component:    eventlog.ComponentRPC,
				Event:        eventlog.EventPluginDialSucceeded,
				PluginID:     m.cfg.PluginID,
				GenerationID: generation,
				SocketPath:   socketPath,
				Message:      "plugin socket dial succeeded",
			})
			return &rpcClient{client: rpc.NewClient(conn), conn: conn}, nil
		}
		time.Sleep(50 * time.Millisecond)
	}
	err := fmt.Errorf("dial plugin socket timeout: %s", socketPath)
	m.logEvent(eventlog.Event{
		Level:        eventlog.LevelError,
		Component:    eventlog.ComponentRPC,
		Event:        eventlog.EventPluginDialFailed,
		PluginID:     m.cfg.PluginID,
		GenerationID: generation,
		SocketPath:   socketPath,
		Message:      "plugin socket dial timed out",
		Error:        err.Error(),
	})
	return nil, err
}

func (m *Manager) call(client *rpcClient, generation, epoch uint64, method string, args any, reply any) error {
	if client == nil {
		return errors.New("rpc client unavailable")
	}
	state := m.State()
	m.logEvent(eventlog.Event{
		Level:        eventlog.LevelDebug,
		Component:    eventlog.ComponentRPC,
		Event:        eventlog.EventRPCStarted,
		PluginID:     m.cfg.PluginID,
		GenerationID: generation,
		PID:          state.PID,
		SocketPath:   state.SocketPath,
		Method:       method,
		Message:      "rpc call started",
	})
	resultCh := make(chan error, 1)
	go func() {
		resultCh <- client.client.Call(method, args, reply)
	}()

	select {
	case err := <-resultCh:
		if err != nil {
			if isRPCPoisonError(err) {
				m.poisonClient(client, generation, method, "rpc transport failure", err)
			}
			m.logEvent(eventlog.Event{
				Level:        eventlog.LevelWarn,
				Component:    eventlog.ComponentRPC,
				Event:        eventlog.EventRPCFailed,
				PluginID:     m.cfg.PluginID,
				GenerationID: generation,
				PID:          state.PID,
				SocketPath:   state.SocketPath,
				Method:       method,
				Message:      "rpc call failed",
				Error:        err.Error(),
			})
			return err
		}
		m.mu.RLock()
		currentGeneration := m.state.GenerationID
		currentEpoch := m.rpcEpoch
		currentClient := m.client
		closed := m.closed
		m.mu.RUnlock()
		if generation != currentGeneration || (epoch != 0 && (closed || currentClient != client || currentEpoch != epoch)) {
			err := fmt.Errorf("stale response rejected: generation=%d current=%d epoch=%d current_epoch=%d", generation, currentGeneration, epoch, currentEpoch)
			m.logEvent(eventlog.Event{
				Level:        eventlog.LevelWarn,
				Component:    eventlog.ComponentRPC,
				Event:        eventlog.EventRPCFailed,
				PluginID:     m.cfg.PluginID,
				GenerationID: generation,
				PID:          state.PID,
				SocketPath:   state.SocketPath,
				Method:       method,
				Message:      "stale rpc response rejected",
				Error:        err.Error(),
			})
			return err
		}
		m.logEvent(eventlog.Event{
			Level:        eventlog.LevelDebug,
			Component:    eventlog.ComponentRPC,
			Event:        eventlog.EventRPCSucceeded,
			PluginID:     m.cfg.PluginID,
			GenerationID: generation,
			PID:          state.PID,
			SocketPath:   state.SocketPath,
			Method:       method,
			Message:      "rpc call succeeded",
		})
		return nil
	case <-time.After(m.cfg.CallTimeout):
		err := fmt.Errorf("rpc timeout calling %s", method)
		m.poisonClient(client, generation, method, "rpc timeout", err)
		m.logEvent(eventlog.Event{
			Level:        eventlog.LevelError,
			Component:    eventlog.ComponentRPC,
			Event:        eventlog.EventRPCTimeout,
			PluginID:     m.cfg.PluginID,
			GenerationID: generation,
			PID:          state.PID,
			SocketPath:   state.SocketPath,
			Method:       method,
			Message:      "rpc call timed out",
			Error:        err.Error(),
		})
		return err
	}
}

func (m *Manager) stopCurrent(markClosed bool, reason string) error {
	m.mu.Lock()
	if m.closed {
		m.mu.Unlock()
		return errors.New("manager closed")
	}
	client := m.client
	cmd := m.cmd
	generation := m.state.GenerationID
	socketPath := m.state.SocketPath
	pid := m.state.PID
	m.rpcEpoch++
	m.client = nil
	m.cmd = nil
	m.state.Healthy = false
	m.state.PID = 0
	if markClosed {
		m.closed = true
	}
	m.mu.Unlock()

	m.logEvent(eventlog.Event{
		Level:        eventlog.LevelWarn,
		Component:    eventlog.ComponentKernel,
		Event:        eventlog.EventKillRequested,
		PluginID:     m.cfg.PluginID,
		GenerationID: generation,
		PID:          pid,
		SocketPath:   socketPath,
		Message:      "stopping plugin process",
		Reason:       reason,
	})

	if client != nil {
		m.logEvent(eventlog.Event{
			Level:        eventlog.LevelInfo,
			Component:    eventlog.ComponentKernel,
			Event:        eventlog.EventShutdownRequested,
			PluginID:     m.cfg.PluginID,
			GenerationID: generation,
			PID:          pid,
			SocketPath:   socketPath,
			Method:       testpluginapi.MethodShutdown,
			Message:      "requesting graceful shutdown",
		})
		if err := managerRPCCall(m, client, generation, 0, testpluginapi.MethodShutdown, testpluginapi.Empty{}, &testpluginapi.Empty{}); err != nil {
			m.logEvent(eventlog.Event{
				Level:        eventlog.LevelWarn,
				Component:    eventlog.ComponentKernel,
				Event:        eventlog.EventShutdownFailed,
				PluginID:     m.cfg.PluginID,
				GenerationID: generation,
				PID:          pid,
				SocketPath:   socketPath,
				Method:       testpluginapi.MethodShutdown,
				Message:      "graceful shutdown failed",
				Error:        err.Error(),
			})
		} else {
			m.logEvent(eventlog.Event{
				Level:        eventlog.LevelInfo,
				Component:    eventlog.ComponentKernel,
				Event:        eventlog.EventShutdownSucceeded,
				PluginID:     m.cfg.PluginID,
				GenerationID: generation,
				PID:          pid,
				SocketPath:   socketPath,
				Method:       testpluginapi.MethodShutdown,
				Message:      "graceful shutdown succeeded",
			})
		}
		_ = client.close()
	}
	if cmd != nil && cmd.Process != nil {
		waitCh := make(chan error, 1)
		go func() {
			_, err := waitProcess(cmd.Process)
			waitCh <- err
		}()
		select {
		case err := <-waitCh:
			if err != nil && !errors.Is(err, os.ErrProcessDone) {
				return fmt.Errorf("wait plugin exit: %w", err)
			}
		case <-time.After(250 * time.Millisecond):
			if err := signalProcess(cmd.Process, syscall.SIGKILL); err != nil && !errors.Is(err, os.ErrProcessDone) {
				return fmt.Errorf("kill plugin: %w", err)
			}
			if err := <-waitCh; err != nil && !errors.Is(err, os.ErrProcessDone) {
				return fmt.Errorf("wait plugin exit after kill: %w", err)
			}
			m.logEvent(eventlog.Event{
				Level:        eventlog.LevelWarn,
				Component:    eventlog.ComponentKernel,
				Event:        eventlog.EventPluginKilled,
				PluginID:     m.cfg.PluginID,
				GenerationID: generation,
				PID:          pid,
				SocketPath:   socketPath,
				Message:      "plugin process killed after grace period",
				Reason:       reason,
			})
			m.cleanupRuntimeArtifacts(generation)
			return nil
		}
	}
	m.logEvent(eventlog.Event{
		Level:        eventlog.LevelInfo,
		Component:    eventlog.ComponentKernel,
		Event:        eventlog.EventPluginStopped,
		PluginID:     m.cfg.PluginID,
		GenerationID: generation,
		PID:          pid,
		SocketPath:   socketPath,
		Message:      "plugin process stopped",
		Reason:       reason,
	})
	m.cleanupRuntimeArtifacts(generation)
	return nil
}

func (m *Manager) cleanupFailedStart(cmd *exec.Cmd, client *rpcClient, generation uint64, reason string, cause error) {
	if client != nil {
		_ = client.close()
	}
	pid := 0
	if cmd != nil && cmd.Process != nil {
		pid = cmd.Process.Pid
		_ = cmd.Process.Kill()
		_, _ = cmd.Process.Wait()
	}
	m.rollbackGeneration(generation)
	m.logEvent(eventlog.Event{
		Level:        eventlog.LevelError,
		Component:    eventlog.ComponentKernel,
		Event:        eventlog.EventPluginStartFailed,
		PluginID:     m.cfg.PluginID,
		GenerationID: generation,
		PID:          pid,
		SocketPath:   amruntime.SocketPath(m.cfg.RuntimeDir, m.cfg.PluginID),
		Message:      "plugin start failed and was cleaned up",
		Reason:       reason,
		Error:        errorString(cause),
	})
	m.cleanupRuntimeArtifacts(generation)
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

func (m *Manager) poisonClient(client *rpcClient, generation uint64, method, reason string, cause error) {
	if client == nil {
		return
	}
	state := m.State()
	m.mu.Lock()
	if m.client == client {
		m.client = nil
		m.state.Healthy = false
	}
	m.mu.Unlock()
	m.logEvent(eventlog.Event{
		Level:        eventlog.LevelWarn,
		Component:    eventlog.ComponentRPC,
		Event:        eventlog.EventRPCPoisoned,
		PluginID:     m.cfg.PluginID,
		GenerationID: generation,
		PID:          state.PID,
		SocketPath:   state.SocketPath,
		Method:       method,
		Message:      "rpc client poisoned and detached",
		Reason:       reason,
		Error:        errorString(cause),
	})
	_ = client.close()
}

func (m *Manager) verifyPeerCred(client *rpcClient, wantPID int, generation uint64) error {
	if client == nil || client.conn == nil {
		return fmt.Errorf("peercred verification requires connected unix socket")
	}
	cred, err := amruntime.ReadPeerCred(client.conn)
	if err != nil {
		return fmt.Errorf("read peer credentials: %w", err)
	}
	if cred.PID != wantPID {
		return fmt.Errorf("peer pid mismatch: got %d want %d", cred.PID, wantPID)
	}
	m.logEvent(eventlog.Event{
		Level:        eventlog.LevelInfo,
		Component:    eventlog.ComponentAuth,
		Event:        eventlog.EventPeerCredVerified,
		PluginID:     m.cfg.PluginID,
		GenerationID: generation,
		PID:          wantPID,
		SocketPath:   amruntime.SocketPath(m.cfg.RuntimeDir, m.cfg.PluginID),
		Message:      "plugin peer credentials verified",
		Details: map[string]any{
			"peer_uid": cred.UID,
			"peer_gid": cred.GID,
		},
	})
	return nil
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

func (m *Manager) cleanupRuntimeArtifacts(generation uint64) {
	socketPath := amruntime.SocketPath(m.cfg.RuntimeDir, m.cfg.PluginID)
	authPath := amruntime.AuthPath(m.cfg.RuntimeDir, m.cfg.PluginID)
	if generation != 0 && !m.cleanupOwnsRuntimeArtifacts(generation) {
		m.logEvent(eventlog.Event{
			Level:        eventlog.LevelDebug,
			Component:    eventlog.ComponentRuntime,
			Event:        eventlog.EventRuntimeCleanup,
			PluginID:     m.cfg.PluginID,
			GenerationID: generation,
			SocketPath:   socketPath,
			Message:      "stale runtime artifact cleanup skipped",
			Details: map[string]any{
				"current_generation": m.State().GenerationID,
			},
		})
		return
	}
	removed := make([]string, 0, 2)
	failed := make(map[string]string)
	for _, path := range []string{socketPath, authPath} {
		if err := os.Remove(path); err != nil {
			if !os.IsNotExist(err) {
				failed[path] = err.Error()
			}
			continue
		}
		removed = append(removed, path)
	}
	if len(failed) > 0 {
		m.logEvent(eventlog.Event{
			Level:        eventlog.LevelWarn,
			Component:    eventlog.ComponentRuntime,
			Event:        eventlog.EventRuntimeCleanupFailed,
			PluginID:     m.cfg.PluginID,
			GenerationID: generation,
			SocketPath:   socketPath,
			Message:      "runtime artifact cleanup had failures",
			Details: map[string]any{
				"removed": removed,
				"failed":  failed,
			},
		})
		return
	}
	m.logEvent(eventlog.Event{
		Level:        eventlog.LevelInfo,
		Component:    eventlog.ComponentRuntime,
		Event:        eventlog.EventRuntimeCleanup,
		PluginID:     m.cfg.PluginID,
		GenerationID: generation,
		SocketPath:   socketPath,
		Message:      "runtime artifacts cleaned up",
		Details: map[string]any{
			"removed": removed,
		},
	})
}

func (m *Manager) cleanupOwnsRuntimeArtifacts(generation uint64) bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if m.state.GenerationID == generation {
		return true
	}
	return m.state.PID == 0 && m.state.SocketPath == ""
}

func (m *Manager) rollbackGeneration(generation uint64) {
	m.mu.Lock()
	if m.state.GenerationID == generation {
		m.state = State{
			PluginID:     m.cfg.PluginID,
			GenerationID: generation - 1,
		}
	}
	m.mu.Unlock()
}

func (m *Manager) logEvent(event eventlog.Event) {
	if m == nil || m.log == nil {
		return
	}
	_ = m.log.Write(event)
}

func errorString(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}
