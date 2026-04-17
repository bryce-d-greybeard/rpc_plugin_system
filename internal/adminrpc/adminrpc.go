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
)

const (
	// ServiceName is the exported admin RPC service name.
	ServiceName = "Admin"
	// MethodStatus returns the current supervised plugin state.
	MethodStatus = ServiceName + ".Status"
	// MethodRestart restarts the supervised plugin and returns the new state.
	MethodRestart = ServiceName + ".Restart"
)

// Empty is a convenience admin RPC request type.
type Empty struct{}

// Service exposes a small admin RPC surface backed by one kernel manager.
type Service struct {
	Manager *kernel.Manager
}

// Status returns the current manager state.
func (s *Service) Status(_ Empty, out *kernel.State) error {
	if s.Manager == nil {
		return fmt.Errorf("manager is required")
	}
	*out = s.Manager.State()
	return nil
}

// Restart restarts the supervised plugin and returns the new state.
func (s *Service) Restart(_ Empty, out *kernel.State) error {
	if s.Manager == nil {
		return fmt.Errorf("manager is required")
	}
	if err := s.Manager.Restart(); err != nil {
		return err
	}
	*out = s.Manager.State()
	return nil
}

// Serve exposes the admin RPC service on one Unix socket until the context ends or the listener fails.
func Serve(ctx context.Context, socketPath string, manager *kernel.Manager) error {
	listener, err := runtime.ListenUnix(socketPath)
	if err != nil {
		return fmt.Errorf("listen admin socket: %w", err)
	}
	defer func() {
		_ = listener.Close()
		_ = os.Remove(socketPath)
	}()

	server := rpc.NewServer()
	if err := server.RegisterName(ServiceName, &Service{Manager: manager}); err != nil {
		return fmt.Errorf("register admin rpc: %w", err)
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

// Dial connects an admin RPC client to one Unix socket path.
func Dial(socketPath string, timeout time.Duration) (*rpc.Client, error) {
	conn, err := net.DialTimeout("unix", socketPath, timeout)
	if err != nil {
		return nil, fmt.Errorf("dial admin socket: %w", err)
	}
	return rpc.NewClient(conn), nil
}
