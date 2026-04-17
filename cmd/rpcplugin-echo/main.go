package main

import (
	"log"
	"net/rpc"
	"os"
	"strconv"
	"time"

	"rpc_plugin_system/internal/auth"
	"rpc_plugin_system/internal/runtime"
	"rpc_plugin_system/internal/testpluginapi"
)

type testPlugin struct {
	pluginID     string
	version      string
	generationID uint64
	startedAt    time.Time
	authToken    string
	authUsed     bool
}

func (p *testPlugin) Auth(in testpluginapi.AuthRequest, out *testpluginapi.AuthResponse) error {
	if p.authUsed || !auth.EqualToken([]byte(in.Token), []byte(p.authToken)) {
		return os.ErrPermission
	}
	p.authUsed = true
	*out = testpluginapi.AuthResponse{
		PluginID:     p.pluginID,
		Version:      p.version,
		GenerationID: p.generationID,
	}
	return nil
}

func (p *testPlugin) Capabilities(_ testpluginapi.Empty, out *testpluginapi.CapabilitiesResponse) error {
	*out = testpluginapi.CapabilitiesResponse{
		PluginID:     p.pluginID,
		Version:      p.version,
		GenerationID: p.generationID,
		Capabilities: []string{"heartbeat", "echo", "sleep", "crash", "shutdown"},
	}
	return nil
}

func (p *testPlugin) Heartbeat(_ testpluginapi.Empty, out *testpluginapi.HeartbeatResponse) error {
	*out = testpluginapi.HeartbeatResponse{
		PluginID:      p.pluginID,
		Version:       p.version,
		GenerationID:  p.generationID,
		UptimeSeconds: int64(time.Since(p.startedAt).Seconds()),
		Status:        testpluginapi.StatusHealthy,
	}
	return nil
}

func (p *testPlugin) Echo(in testpluginapi.EchoRequest, out *testpluginapi.EchoResponse) error {
	*out = testpluginapi.EchoResponse{Message: in.Message}
	return nil
}

func (p *testPlugin) Sleep(in testpluginapi.SleepRequest, _ *testpluginapi.Empty) error {
	time.Sleep(in.Duration)
	return nil
}

func (p *testPlugin) Crash(in testpluginapi.CrashRequest, _ *testpluginapi.Empty) error {
	os.Exit(in.Code)
	return nil
}

func (p *testPlugin) Shutdown(_ testpluginapi.Empty, _ *testpluginapi.Empty) error {
	go func() {
		if marker := os.Getenv("RPC_PLUGIN_SYSTEM_PLUGIN_SHUTDOWN_MARKER"); marker != "" {
			_ = os.WriteFile(marker, []byte("shutdown\n"), 0o600)
		}
		time.Sleep(50 * time.Millisecond)
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
		pluginID = "echo"
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

	listener, err := runtime.ListenUnix(sock)
	if err != nil {
		log.Fatalf("listen unix socket: %v", err)
	}
	defer listener.Close()

	server := rpc.NewServer()
	plugin := &testPlugin{pluginID: pluginID, version: "0.1.0", generationID: generationID, startedAt: time.Now(), authToken: string(authTokenRaw)}
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
