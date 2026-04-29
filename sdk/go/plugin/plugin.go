package plugin

import (
	"bytes"
	"fmt"
	"net"
	"net/rpc"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"rpc_plugin_system/internal/auth"
	"rpc_plugin_system/internal/bootstrap"
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
	Token           string
	SessionID       string
	PluginPublicKey []byte
}

type AuthResponse struct {
	PluginID     string
	Version      string
	GenerationID uint64
	SessionID    string
	SessionKey   []byte
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
	SocketPath           string
	PluginID             string
	GenerationID         uint64
	AuthToken            string
	BootstrapSessionID   string
	BootstrapEndpoint    string
	BootstrapKeyPair     *bootstrap.KeyPair
	BootstrapSessionKey  []byte
	BootstrapSessionKeys *bootstrap.Keys
}

// ErrMissingEnv reports one required plugin startup environment variable that was not set.
type ErrMissingEnv struct {
	Name string
}

func (e ErrMissingEnv) Error() string {
	return fmt.Sprintf("%s is required for plugin startup", e.Name)
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
	mu           sync.Mutex
	core         Core
	cfg          Config
	authUsed     bool
	capabilities []string
	logger       *Logger
}

func LoadConfigFromEnv() (Config, error) {
	sock := os.Getenv("RPC_PLUGIN_SYSTEM_PLUGIN_SOCKET")
	if sock == "" {
		return Config{}, ErrMissingEnv{Name: "RPC_PLUGIN_SYSTEM_PLUGIN_SOCKET"}
	}
	pluginID := os.Getenv("RPC_PLUGIN_SYSTEM_PLUGIN_ID")
	if pluginID == "" {
		return Config{}, ErrMissingEnv{Name: "RPC_PLUGIN_SYSTEM_PLUGIN_ID"}
	}
	genStr := os.Getenv("RPC_PLUGIN_SYSTEM_PLUGIN_GENERATION")
	if genStr == "" {
		return Config{}, ErrMissingEnv{Name: "RPC_PLUGIN_SYSTEM_PLUGIN_GENERATION"}
	}
	generationID, err := strconv.ParseUint(genStr, 10, 64)
	if err != nil {
		return Config{}, fmt.Errorf("parse RPC_PLUGIN_SYSTEM_PLUGIN_GENERATION %q: %w", genStr, err)
	}
	authFile := os.Getenv("RPC_PLUGIN_SYSTEM_AUTH_TOKEN_FILE")
	if authFile == "" {
		return Config{}, ErrMissingEnv{Name: "RPC_PLUGIN_SYSTEM_AUTH_TOKEN_FILE"}
	}
	authTokenRaw, err := os.ReadFile(authFile)
	if err != nil {
		return Config{}, fmt.Errorf("read RPC_PLUGIN_SYSTEM_AUTH_TOKEN_FILE %q: %w", authFile, err)
	}
	bootstrapSessionID := os.Getenv("RPC_PLUGIN_SYSTEM_BOOTSTRAP_SESSION_ID")
	bootstrapEndpoint := os.Getenv("RPC_PLUGIN_SYSTEM_BOOTSTRAP_ENDPOINT")
	cfg := Config{SocketPath: sock, PluginID: pluginID, GenerationID: generationID, AuthToken: string(authTokenRaw), BootstrapSessionID: bootstrapSessionID, BootstrapEndpoint: bootstrapEndpoint}
	if bootstrapEndpoint != "" && bootstrapSessionID != "" {
		if err := performBootstrapHandshake(&cfg); err != nil {
			return Config{}, err
		}
	}
	return cfg, nil
}

func performBootstrapHandshake(cfg *Config) error {
	parts := strings.Split(cfg.BootstrapEndpoint, ":")
	if len(parts) != 2 {
		return fmt.Errorf("invalid bootstrap endpoint")
	}
	reqReader, err := os.OpenFile(parts[0], os.O_RDONLY, 0)
	if err != nil {
		return fmt.Errorf("open bootstrap request fifo: %w", err)
	}
	record, err := bootstrap.ReadRecord(reqReader)
	_ = reqReader.Close()
	if err != nil {
		return err
	}
	if record.PluginID != cfg.PluginID || record.SessionID != cfg.BootstrapSessionID || record.Token != auth.Encode([]byte(cfg.AuthToken)) {
		return fmt.Errorf("bootstrap record mismatch: record plugin=%q session=%q token=%q cfg plugin=%q session=%q token=%q", record.PluginID, record.SessionID, record.Token, cfg.PluginID, cfg.BootstrapSessionID, auth.Encode([]byte(cfg.AuthToken)))
	}
	if len(record.SubstratePublicKey) == 0 {
		return fmt.Errorf("bootstrap substrate public key missing")
	}
	kp, err := bootstrap.GenerateKeyPair()
	if err != nil {
		return fmt.Errorf("generate plugin bootstrap keypair: %w", err)
	}
	sharedSecret, err := bootstrap.DeriveSharedSecret(kp.Private, record.SubstratePublicKey)
	if err != nil {
		return fmt.Errorf("derive plugin shared secret: %w", err)
	}
	rootKey, err := bootstrap.DeriveRootKey(sharedSecret, []byte(cfg.AuthToken), cfg.PluginID, cfg.BootstrapSessionID)
	if err != nil {
		return fmt.Errorf("derive plugin session root key: %w", err)
	}
	keys, err := bootstrap.NewKeys(rootKey)
	if err != nil {
		return fmt.Errorf("derive plugin initial session keys: %w", err)
	}
	keys, err = bootstrap.Rekey(keys)
	if err != nil {
		return fmt.Errorf("refresh plugin session keys: %w", err)
	}
	respWriter, err := os.OpenFile(parts[1], os.O_WRONLY, 0)
	if err != nil {
		return fmt.Errorf("open bootstrap response fifo: %w", err)
	}
	defer respWriter.Close()
	response := bootstrap.NewResponse(cfg.PluginID, cfg.BootstrapSessionID, record.Token, kp.Public)
	if err := bootstrap.WriteResponse(respWriter, response); err != nil {
		return err
	}
	cfg.BootstrapKeyPair = kp
	cfg.BootstrapSessionKey = keys.SendKey
	cfg.BootstrapSessionKeys = keys
	return nil
}

func Serve(core Core) error {
	cfg, err := LoadConfigFromEnv()
	if err != nil {
		return fmt.Errorf("load plugin config from env: %w", err)
	}
	return ServeWithConfig(cfg, core)
}

func ServeWithConfig(cfg Config, core Core) error {
	if core == nil {
		return fmt.Errorf("plugin core is required")
	}
	logger, err := NewLogger(cfg)
	if err != nil {
		return fmt.Errorf("create plugin logger: %w", err)
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
		serveConn := conn
		if cfg.BootstrapSessionKeys != nil {
			secureConn, secureErr := bootstrap.NewSecureConn(conn, cfg.BootstrapSessionKeys.RecvKey, cfg.BootstrapSessionKeys.SendKey)
			if secureErr != nil {
				_ = conn.Close()
				_ = logger.Event(LogEvent{Level: LogLevelError, Event: EventPluginServeStopped, Message: "secure rpc wrapper failed", Error: secureErr.Error()})
				return fmt.Errorf("wrap secure rpc conn: %w", secureErr)
			}
			serveConn = secureConn
		}
		go rpcServer.ServeConn(serveConn)
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
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.authUsed {
		if s.logger != nil {
			_ = s.logger.Event(LogEvent{Level: LogLevelWarn, Event: EventPluginAuthRejected, Method: MethodAuth, Message: "plugin auth replay rejected", Reason: "auth already used"})
		}
		return os.ErrPermission
	}
	if s.cfg.BootstrapSessionID != "" {
		if in.SessionID != s.cfg.BootstrapSessionID {
			if s.logger != nil {
				_ = s.logger.Event(LogEvent{Level: LogLevelWarn, Event: EventPluginAuthRejected, Method: MethodAuth, Message: "plugin auth session rejected", Reason: "session mismatch"})
			}
			return os.ErrPermission
		}
		if s.cfg.BootstrapKeyPair == nil || !bytes.Equal(in.PluginPublicKey, s.cfg.BootstrapKeyPair.Public) {
			if s.logger != nil {
				_ = s.logger.Event(LogEvent{Level: LogLevelWarn, Event: EventPluginAuthRejected, Method: MethodAuth, Message: "plugin auth public key rejected", Reason: "plugin public key mismatch"})
			}
			return os.ErrPermission
		}
	} else if !auth.EqualToken([]byte(in.Token), []byte(s.cfg.AuthToken)) {
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
	resp := AuthResponse{PluginID: pluginID, Version: s.core.Version(), GenerationID: generationID}
	if s.cfg.BootstrapSessionID != "" && in.SessionID == s.cfg.BootstrapSessionID {
		resp.SessionID = s.cfg.BootstrapSessionID
		resp.SessionKey = append([]byte(nil), s.cfg.BootstrapSessionKey...)
	}
	*out = resp
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
	if err := s.requireAuth(MethodHeartbeat); err != nil {
		return err
	}
	err := s.core.Heartbeat(in, out)
	s.logRPC(MethodHeartbeat, err)
	return err
}

func (s *server) Shutdown(in Empty, out *Empty) error {
	if err := s.requireAuth(MethodShutdown); err != nil {
		return err
	}
	if s.logger != nil {
		_ = s.logger.Event(LogEvent{Event: EventPluginShutdownCalled, Method: MethodShutdown, Message: "plugin shutdown requested"})
	}
	err := s.core.Shutdown(in, out)
	s.logRPC(MethodShutdown, err)
	return err
}

func (s *server) Echo(in EchoRequest, out *EchoResponse) error {
	if err := s.requireAuth(MethodEcho); err != nil {
		return err
	}
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
	if err := s.requireAuth(MethodSleep); err != nil {
		return err
	}
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
	if err := s.requireAuth(MethodCrash); err != nil {
		return err
	}
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

func (s *server) requireAuth(method string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.authUsed {
		return nil
	}
	err := os.ErrPermission
	if s.logger != nil {
		_ = s.logger.Event(LogEvent{Level: LogLevelWarn, Event: EventPluginAuthRejected, Method: method, Message: "plugin rpc rejected before auth", Reason: "auth required", Error: err.Error()})
	}
	s.logRPC(method, err)
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
