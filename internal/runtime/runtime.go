// Package runtime provides runtime directory and path helpers.
package runtime

import (
	"fmt"
	"net"
	"os"
	"path/filepath"
)

const (
	dirMode    = 0o700
	socketMode = 0o600
)

// EnsureDir creates a private runtime directory for plugin sockets and auth files.
func EnsureDir(root string) error {
	if err := os.MkdirAll(root, dirMode); err != nil {
		return fmt.Errorf("create runtime dir: %w", err)
	}
	if err := os.Chmod(root, dirMode); err != nil {
		return fmt.Errorf("chmod runtime dir: %w", err)
	}
	return nil
}

// SocketPath returns the Unix socket path for one plugin.
func SocketPath(root, pluginID string) string {
	return filepath.Join(root, pluginID+".sock")
}

// AuthPath returns the auth challenge path for one plugin.
func AuthPath(root, pluginID string) string {
	return filepath.Join(root, pluginID+".auth")
}

// ListenUnix binds one Unix socket path with private permissions.
func ListenUnix(socketPath string) (net.Listener, error) {
	_ = os.Remove(socketPath)
	listener, err := net.Listen("unix", socketPath)
	if err != nil {
		return nil, fmt.Errorf("listen unix socket: %w", err)
	}
	if err := os.Chmod(socketPath, socketMode); err != nil {
		_ = listener.Close()
		_ = os.Remove(socketPath)
		return nil, fmt.Errorf("chmod unix socket: %w", err)
	}
	return listener, nil
}

// ValidateExecutable ensures a configured plugin path points to one executable file.
func ValidateExecutable(path string) error {
	info, err := os.Stat(path)
	if err != nil {
		return fmt.Errorf("stat executable: %w", err)
	}
	if info.IsDir() {
		return fmt.Errorf("executable path is a directory: %s", path)
	}
	if info.Mode()&0o111 == 0 {
		return fmt.Errorf("executable path is not executable: %s", path)
	}
	return nil
}
