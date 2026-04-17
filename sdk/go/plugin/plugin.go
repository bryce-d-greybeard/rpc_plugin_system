package plugin

import (
	"fmt"
	"net"
	"net/rpc"
	"os"
	"strconv"
	"time"

	"rpc_plugin_system/internal/auth"
	"rpc_plugin_system/internal/runtime"
)

const ServiceName = "TestPlugin"

const (
	MethodAuth         = ServiceName + ".Auth"
	MethodCapabilities = ServiceName + ".Capabilities"
	MethodHeartbeat    = ServiceName + ".Heartbeat"
	MethodShutdown     = ServiceName + ".Shutdown"
	MethodEcho         = ServiceName + ".Echo"
	MethodSleep        = ServiceName + ".Sleep"
	MethodCrash        = ServiceName + ".Crash"
)

type Status string

const (
	StatusStarting   Status = "starting"
	StatusHealthy    Status = "healthy"
	StatusDegraded   Status = "degraded"
	StatusOverloaded Status = "overloaded"
	StatusUnhealthy  Status = "unhealthy"
)

type Empty struct{}

type AuthRequest struct {
	Token string
}

type AuthResponse struct {
	PluginID     string
	Version      string
	GenerationID uint64
}

type CapabilitiesResponse struct {
	PluginID     string
	Version      string
	GenerationID uint64
	Capabilities []string
}

type HeartbeatResponse struct {
	PluginID              string
	Version               string
	GenerationID          uint64
	UptimeSeconds         int64
	Status                Status
	CurrentWorkCount      int
	LastSuccessfulUnixSec int64
	RecentErrorCount      int
}

type EchoRequest struct{ Message string }
type EchoResponse struct{ Message string }
type SleepRequest struct{ Duration time.Duration }
type CrashRequest struct{ Code int }

type Config struct {
	SocketPath   string
	PluginID     string
	GenerationID uint64
	AuthToken    string
}

type Core interface {
	Version() string
	Heartbeat(Empty, *HeartbeatResponse) error
	Shutdown(Empty, *Empty) error
}

type Echo interface {
	Echo(EchoRequest, *EchoResponse) error
}
type Sleep interface {
	Sleep(SleepRequest, *Empty) error
}
type Crash interface {
	Crash(CrashRequest, *Empty) error
}
type capabilityProvider interface{ Capabilities() []string }
type authHook interface{ OnAuthAttempt(AuthRequest) error }
type acceptHook interface{ OnServeListener(net.Listener) }
type identityHook interface{ Identity() (string, uint64) }

type server struct {
	core         Core
	cfg          Config
	authUsed     bool
	capabilities []string
	logger       *Logger
}

func LoadConfigFromEnv() (Config, error) {
	sock := os.Getenv("RPC_PLUGIN_SYSTEM_PLUGIN_SOCKET")
	if sock == "" {
		return Config{}, fmt.Errorf("RPC_PLUGIN_SYSTEM_PLUGIN_SOCKET is required")
	}
	pluginID := os.Getenv("RPC_PLUGIN_SYSTEM_PLUGIN_ID")
	if pluginID == "" {
		return Config{}, fmt.Errorf("RPC_PLUGIN_SYSTEM_PLUGIN_ID is required")
	}
	genStr := os.Getenv("RPC_PLUGIN_SYSTEM_PLUGIN_GENERATION")
	generationID, err := strconv.ParseUint(genStr, 10, 64)
	if err != nil {
		return Config{}, fmt.Errorf("parse generation: %w", err)
	}
	authFile := os.Getenv("RPC_PLUGIN_SYSTEM_AUTH_TOKEN_FILE")
	if authFile == "" {
		return Config{}, fmt.Errorf("RPC_PLUGIN_SYSTEM_AUTH_TOKEN_FILE is required")
	}
	authTokenRaw, err := os.ReadFile(authFile)
	if err != nil {
		return Config{}, fmt.Errorf("read auth token: %w", err)
	}
	return Config{SocketPath: sock, PluginID: pluginID, GenerationID: generationID, AuthToken: string(authTokenRaw)}, nil
}

func Serve(core Core) error {
	cfg, err := LoadConfigFromEnv()
	if err != nil {
		return err
	}
	return ServeWithConfig(cfg, core)
}

func ServeWithConfig(cfg Config, core Core) error {
	logger, err := NewLogger(cfg)
	if err != nil {
		return err
	}
	defer logger.Close()
	_ = logger.Event(LogEvent{Event: EventPluginBootStarted, Message: "plugin boot starting"})
	_ = logger.Event(LogEvent{Event: EventPluginConfigLoaded, Message: "plugin config loaded", Details: map[string]any{"socket_path": cfg.SocketPath}})

	listener, err := runtime.ListenUnix(cfg.SocketPath)
	if err != nil {
		_ = logger.Event(LogEvent{Level: LogLevelError, Event: EventPluginBootFailed, Message: "plugin listener startup failed", Error: err.Error()})
		return fmt.Errorf("listen unix socket: %w", err)
	}
	if hook, ok := core.(acceptHook); ok {
		hook.OnServeListener(listener)
	}
	defer listener.Close()
	_ = logger.Event(LogEvent{Event: EventPluginListenerStarted, Message: "plugin listener ready", SocketPath: cfg.SocketPath})

	srv := &server{core: core, cfg: cfg, capabilities: detectCapabilities(core), logger: logger}
	rpcServer := rpc.NewServer()
	if err := rpcServer.RegisterName(ServiceName, srv); err != nil {
		_ = logger.Event(LogEvent{Level: LogLevelError, Event: EventPluginBootFailed, Message: "rpc service registration failed", Error: err.Error()})
		return fmt.Errorf("register rpc service: %w", err)
	}
	for {
		conn, err := listener.Accept()
		if err != nil {
			_ = logger.Event(LogEvent{Event: EventPluginServeStopped, Message: "plugin listener stopped", Reason: "listener closed", Error: err.Error()})
			return nil
		}
		go rpcServer.ServeConn(conn)
	}
}

func detectCapabilities(core Core) []string {
	if p, ok := core.(capabilityProvider); ok {
		return p.Capabilities()
	}
	caps := []string{"heartbeat", "shutdown"}
	if _, ok := core.(Echo); ok {
		caps = append(caps, "echo")
	}
	if _, ok := core.(Sleep); ok {
		caps = append(caps, "sleep")
	}
	if _, ok := core.(Crash); ok {
		caps = append(caps, "crash")
	}
	return caps
}

func (s *server) Auth(in AuthRequest, out *AuthResponse) error {
	if s.logger != nil {
		_ = s.logger.Event(LogEvent{Event: EventPluginAuthAttempt, Method: MethodAuth, Message: "plugin auth attempt received"})
	}
	if hook, ok := s.core.(authHook); ok {
		if err := hook.OnAuthAttempt(in); err != nil {
			if s.logger != nil {
				_ = s.logger.Event(LogEvent{Level: LogLevelWarn, Event: EventPluginAuthRejected, Method: MethodAuth, Message: "plugin auth hook rejected request", Error: err.Error()})
			}
			return err
		}
	}
	if s.authUsed || !auth.EqualToken([]byte(in.Token), []byte(s.cfg.AuthToken)) {
		if s.logger != nil {
			_ = s.logger.Event(LogEvent{Level: LogLevelWarn, Event: EventPluginAuthRejected, Method: MethodAuth, Message: "plugin auth token rejected", Reason: "used or mismatched token"})
		}
		return os.ErrPermission
	}
	s.authUsed = true
	pluginID := s.cfg.PluginID
	generationID := s.cfg.GenerationID
	if hook, ok := s.core.(identityHook); ok {
		pluginID, generationID = hook.Identity()
	}
	*out = AuthResponse{PluginID: pluginID, Version: s.core.Version(), GenerationID: generationID}
	if s.logger != nil {
		_ = s.logger.Event(LogEvent{Event: EventPluginAuthAccepted, Method: MethodAuth, Message: "plugin auth accepted", Details: map[string]any{"version": s.core.Version()}})
	}
	return nil
}

func (s *server) Capabilities(_ Empty, out *CapabilitiesResponse) error {
	pluginID := s.cfg.PluginID
	generationID := s.cfg.GenerationID
	if hook, ok := s.core.(identityHook); ok {
		pluginID, generationID = hook.Identity()
	}
	*out = CapabilitiesResponse{PluginID: pluginID, Version: s.core.Version(), GenerationID: generationID, Capabilities: s.capabilities}
	if s.logger != nil {
		_ = s.logger.Event(LogEvent{Event: EventPluginCapabilitiesRead, Method: MethodCapabilities, Message: "plugin capabilities reported", Details: map[string]any{"capabilities": s.capabilities}})
	}
	return nil
}

func (s *server) Heartbeat(in Empty, out *HeartbeatResponse) error {
	err := s.core.Heartbeat(in, out)
	s.logRPC(MethodHeartbeat, err)
	return err
}

func (s *server) Shutdown(in Empty, out *Empty) error {
	if s.logger != nil {
		_ = s.logger.Event(LogEvent{Event: EventPluginShutdownCalled, Method: MethodShutdown, Message: "plugin shutdown requested"})
	}
	err := s.core.Shutdown(in, out)
	s.logRPC(MethodShutdown, err)
	return err
}

func (s *server) Echo(in EchoRequest, out *EchoResponse) error {
	p, ok := s.core.(Echo)
	if !ok {
		err := rpc.ErrShutdown
		s.logRPC(MethodEcho, err)
		return err
	}
	err := p.Echo(in, out)
	s.logRPC(MethodEcho, err)
	return err
}

func (s *server) Sleep(in SleepRequest, out *Empty) error {
	p, ok := s.core.(Sleep)
	if !ok {
		err := rpc.ErrShutdown
		s.logRPC(MethodSleep, err)
		return err
	}
	err := p.Sleep(in, out)
	s.logRPC(MethodSleep, err)
	return err
}

func (s *server) Crash(in CrashRequest, out *Empty) error {
	p, ok := s.core.(Crash)
	if !ok {
		err := rpc.ErrShutdown
		s.logRPC(MethodCrash, err)
		return err
	}
	err := p.Crash(in, out)
	s.logRPC(MethodCrash, err)
	return err
}

func (s *server) logRPC(method string, err error) {
	if s.logger == nil {
		return
	}
	if err != nil {
		_ = s.logger.Event(LogEvent{Level: LogLevelWarn, Event: EventPluginRequestFailed, Method: method, Message: "plugin rpc handler failed", Error: err.Error()})
		return
	}
	_ = s.logger.Event(LogEvent{Event: EventPluginRequestHandled, Method: method, Message: "plugin rpc handler completed"})
}
