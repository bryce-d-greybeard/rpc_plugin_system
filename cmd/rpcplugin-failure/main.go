package main

import (
	"errors"
	"log"
	"net"
	"net/rpc"
	"os"
	"strconv"
	"time"

	"rpc_plugin_system/internal/auth"
	"rpc_plugin_system/internal/runtime"
	"rpc_plugin_system/internal/testpluginapi"
)

type failurePlugin struct {
	pluginID     string
	version      string
	generationID uint64
	startedAt    time.Time
	behavior     testpluginapi.Env
	listener     net.Listener
	authToken    string
	authUsed     bool
}

func (p *failurePlugin) Auth(in testpluginapi.AuthRequest, out *testpluginapi.AuthResponse) error {
	if p.behavior.FailAuth {
		return errors.New("auth failure requested")
	}
	if p.authUsed || !auth.EqualToken([]byte(in.Token), []byte(p.authToken)) {
		return os.ErrPermission
	}
	p.authUsed = true
	pluginID := p.pluginID
	if p.behavior.PluginID != "" {
		pluginID = p.behavior.PluginID
	}
	*out = testpluginapi.AuthResponse{
		PluginID:     pluginID,
		Version:      p.version,
		GenerationID: uint64(int64(p.generationID) + p.behavior.GenerationOffset),
	}
	return nil
}

func (p *failurePlugin) Capabilities(_ testpluginapi.Empty, out *testpluginapi.CapabilitiesResponse) error {
	if p.behavior.CapabilitiesMode == "error" {
		return errors.New("capabilities failure requested")
	}
	caps := []string{"heartbeat", "echo", "sleep", "crash", "shutdown"}
	if p.behavior.CapabilitiesMode == "empty" {
		caps = nil
	}
	pluginID := p.pluginID
	if p.behavior.PluginID != "" {
		pluginID = p.behavior.PluginID
	}
	*out = testpluginapi.CapabilitiesResponse{
		PluginID:     pluginID,
		Version:      p.version,
		GenerationID: uint64(int64(p.generationID) + p.behavior.GenerationOffset),
		Capabilities: caps,
	}
	return nil
}

func (p *failurePlugin) Heartbeat(_ testpluginapi.Empty, out *testpluginapi.HeartbeatResponse) error {
	if p.behavior.HeartbeatErrors > 0 {
		p.behavior.HeartbeatErrors--
		return errors.New("heartbeat failure requested")
	}
	pluginID := p.pluginID
	if p.behavior.PluginID != "" {
		pluginID = p.behavior.PluginID
	}
	*out = testpluginapi.HeartbeatResponse{
		PluginID:      pluginID,
		Version:       p.version,
		GenerationID:  uint64(int64(p.generationID) + p.behavior.GenerationOffset),
		UptimeSeconds: int64(time.Since(p.startedAt).Seconds()),
		Status:        p.behavior.HeartbeatStatus,
	}
	return nil
}

func (p *failurePlugin) Echo(in testpluginapi.EchoRequest, out *testpluginapi.EchoResponse) error {
	if p.behavior.CrashOnEcho {
		os.Exit(9)
	}
	if p.behavior.CloseOnEcho && p.listener != nil {
		_ = p.listener.Close()
	}
	*out = testpluginapi.EchoResponse{Message: in.Message}
	return nil
}

func (p *failurePlugin) Sleep(in testpluginapi.SleepRequest, _ *testpluginapi.Empty) error {
	scale := p.behavior.SleepScale
	if scale <= 0 {
		scale = 1
	}
	time.Sleep(time.Duration(float64(in.Duration) * scale))
	return nil
}

func (p *failurePlugin) Crash(in testpluginapi.CrashRequest, _ *testpluginapi.Empty) error {
	os.Exit(in.Code)
	return nil
}

func (p *failurePlugin) Shutdown(_ testpluginapi.Empty, _ *testpluginapi.Empty) error {
	go func() {
		if marker := os.Getenv("RPC_PLUGIN_SYSTEM_PLUGIN_SHUTDOWN_MARKER"); marker != "" {
			_ = os.WriteFile(marker, []byte("shutdown\n"), 0o600)
		}
		time.Sleep(p.behavior.ShutdownDelay)
		os.Exit(0)
	}()
	return nil
}

func main() {
	sock := os.Getenv("RPC_PLUGIN_SYSTEM_PLUGIN_SOCKET")
	if sock == "" {
		log.Fatal("RPC_PLUGIN_SYSTEM_PLUGIN_SOCKET is required")
	}
	pluginID := os.Getenv("RPC_PLUGIN_SYSTEM_PLUGIN_ID")
	if pluginID == "" {
		pluginID = "failure"
	}
	genStr := os.Getenv("RPC_PLUGIN_SYSTEM_PLUGIN_GENERATION")
	generationID, err := strconv.ParseUint(genStr, 10, 64)
	if err != nil {
		log.Fatalf("parse generation: %v", err)
	}
	authFile := os.Getenv("RPC_PLUGIN_SYSTEM_AUTH_TOKEN_FILE")
	if authFile == "" {
		log.Fatal("RPC_PLUGIN_SYSTEM_AUTH_TOKEN_FILE is required")
	}
	authTokenRaw, err := os.ReadFile(authFile)
	if err != nil {
		log.Fatalf("read auth token: %v", err)
	}
	behavior := testpluginapi.LoadEnv()

	listener, err := runtime.ListenUnix(sock)
	if err != nil {
		log.Fatalf("listen unix socket: %v", err)
	}
	if behavior.CloseOnAccept {
		_ = listener.Close()
		return
	}
	defer listener.Close()

	server := rpc.NewServer()
	plugin := &failurePlugin{
		pluginID:     pluginID,
		version:      behavior.Version,
		generationID: generationID,
		startedAt:    time.Now(),
		behavior:     behavior,
		listener:     listener,
		authToken:    string(authTokenRaw),
	}
	if err := server.RegisterName(testpluginapi.ServiceName, plugin); err != nil {
		log.Fatalf("register rpc service: %v", err)
	}

	for {
		conn, err := listener.Accept()
		if err != nil {
			return
		}
		go server.ServeConn(conn)
	}
}
