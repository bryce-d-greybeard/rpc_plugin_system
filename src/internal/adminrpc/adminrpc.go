// Package adminrpc provides a small Unix-socket RPC surface for daemon inspection and control.
package adminrpc

import (
	"context"
	"fmt"
	"net"
	"net/rpc"
	"os"
	"time"

	"rpc_plugin_system/internal/kernel"
	"rpc_plugin_system/internal/runtime"
	"rpc_plugin_system/test/testpluginapi"
)

const (
	// ServiceName is the exported admin RPC service name.
	ServiceName = "Admin"
	// MethodStatus returns the current supervised plugin state.
	MethodStatus = ServiceName + ".Status"
	// MethodRestart restarts one supervised plugin and returns the new state.
	MethodRestart = ServiceName + ".Restart"
	// MethodPlugins returns all known plugin states and the capability map.
	MethodPlugins = ServiceName + ".Plugins"
	// MethodPlugin returns one plugin state by plugin id.
	MethodPlugin = ServiceName + ".Plugin"
	// MethodCapabilities returns the current capability map.
	MethodCapabilities = ServiceName + ".Capabilities"
	// MethodRoutes returns the current direct-routing table.
	MethodRoutes = ServiceName + ".Routes"
	// MethodEcho routes one echo request to a target plugin id.
	MethodEcho = ServiceName + ".Echo"
	// MethodHeartbeat routes one heartbeat request to a target plugin id.
	MethodHeartbeat = ServiceName + ".Heartbeat"
)

// Empty is a convenience admin RPC request type.
type Empty struct{}

// PluginRequest targets one plugin by plugin id.
type PluginRequest struct {
	PluginID string
}

// RestartRequest targets one plugin restart by plugin id.
type RestartRequest = PluginRequest

// EchoRequest routes a message to one plugin by plugin id.
type EchoRequest struct {
	PluginID string
	Message  string
}

// HeartbeatRequest routes one heartbeat request to one plugin by plugin id.
type HeartbeatRequest = PluginRequest

// Service exposes a small admin RPC surface backed by one kernel host.
type Service struct {
	Host *kernel.Host
}

// Status returns the full host state.
func (s *Service) Status(_ Empty, out *kernel.HostState) error {
	if s.Host == nil {
		return fmt.Errorf("host is required")
	}
	*out = s.Host.State()
	return nil
}

// Plugins returns the full host state.
func (s *Service) Plugins(_ Empty, out *kernel.HostState) error {
	if s.Host == nil {
		return fmt.Errorf("host is required")
	}
	*out = s.Host.State()
	return nil
}

// Plugin returns one plugin state by plugin id.
func (s *Service) Plugin(in PluginRequest, out *kernel.State) error {
	if s.Host == nil {
		return fmt.Errorf("host is required")
	}
	if err := kernel.ValidatePluginID(in.PluginID); err != nil {
		return err
	}
	state, err := s.Host.Plugin(in.PluginID)
	if err != nil {
		return err
	}
	*out = state
	return nil
}

// Capabilities returns the current capability map.
func (s *Service) Capabilities(_ Empty, out *map[string][]string) error {
	if s.Host == nil {
		return fmt.Errorf("host is required")
	}
	caps := s.Host.CapabilityMap()
	*out = caps
	return nil
}

// Routes returns the current direct-routing table.
func (s *Service) Routes(_ Empty, out *[]kernel.Route) error {
	if s.Host == nil {
		return fmt.Errorf("host is required")
	}
	routes := s.Host.Routes()
	*out = routes
	return nil
}

// Echo routes one echo request to a target plugin and returns the response.
func (s *Service) Echo(in EchoRequest, out *testpluginapi.EchoResponse) error {
	if s.Host == nil {
		return fmt.Errorf("host is required")
	}
	if err := kernel.ValidatePluginID(in.PluginID); err != nil {
		return err
	}
	message, err := s.Host.Echo(in.PluginID, in.Message)
	if err != nil {
		return err
	}
	*out = testpluginapi.EchoResponse{Message: message}
	return nil
}

// Heartbeat routes one heartbeat request to a target plugin and returns the response.
func (s *Service) Heartbeat(in HeartbeatRequest, out *testpluginapi.HeartbeatResponse) error {
	if s.Host == nil {
		return fmt.Errorf("host is required")
	}
	if err := kernel.ValidatePluginID(in.PluginID); err != nil {
		return err
	}
	state, err := s.Host.Heartbeat(in.PluginID)
	if err != nil {
		return err
	}
	*out = state
	return nil
}

// Restart restarts one plugin and returns that plugin state.
func (s *Service) Restart(in RestartRequest, out *kernel.State) error {
	if s.Host == nil {
		return fmt.Errorf("host is required")
	}
	if err := kernel.ValidatePluginID(in.PluginID); err != nil {
		return err
	}
	state, err := s.Host.RestartPlugin(in.PluginID)
	if err != nil {
		return err
	}
	*out = state
	return nil
}

type listenUnixFunc func(string) (net.Listener, error)
type registerAdminServiceFunc func(*rpc.Server, *kernel.Host) error

// Serve exposes the admin RPC service on one Unix socket until the context ends or the listener fails.
func Serve(ctx context.Context, socketPath string, host *kernel.Host) error {
	return serve(ctx, socketPath, host, runtime.ListenUnix, registerAdminService)
}

func serve(ctx context.Context, socketPath string, host *kernel.Host, listenUnix listenUnixFunc, registerService registerAdminServiceFunc) error {
	listener, err := listenUnix(socketPath)
	if err != nil {
		return fmt.Errorf("listen admin socket: %w", err)
	}
	defer func() {
		_ = listener.Close()
		_ = os.Remove(socketPath)
	}()

	server := rpc.NewServer()
	if err := registerService(server, host); err != nil {
		return err
	}

	go func() {
		<-ctx.Done()
		_ = listener.Close()
	}()

	for {
		conn, err := listener.Accept()
		if err != nil {
			select {
			case <-ctx.Done():
				return nil
			default:
				return fmt.Errorf("accept admin socket: %w", err)
			}
		}
		go server.ServeConn(conn)
	}
}

func registerAdminService(server *rpc.Server, host *kernel.Host) error {
	if err := server.RegisterName(ServiceName, &Service{Host: host}); err != nil {
		return fmt.Errorf("register admin rpc: %w", err)
	}
	return nil
}

// Dial connects an admin RPC client to one Unix socket path.
func Dial(socketPath string, timeout time.Duration) (*rpc.Client, error) {
	conn, err := net.DialTimeout("unix", socketPath, timeout)
	if err != nil {
		return nil, fmt.Errorf("dial admin socket: %w", err)
	}
	return rpc.NewClient(conn), nil
}
