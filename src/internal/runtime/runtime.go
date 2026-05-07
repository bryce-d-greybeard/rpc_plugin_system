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

var (
	chmod    = os.Chmod
	mkdirAll = os.MkdirAll
)

// EnsureDir creates a private runtime directory for plugin sockets and auth files.
func EnsureDir(root string) error {
	if root == "" {
		return fmt.Errorf("runtime dir is required")
	}
	info, err := os.Lstat(root)
	if err == nil {
		if info.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("runtime dir must not be a symlink: %s", root)
		}
		if !info.IsDir() {
			return fmt.Errorf("runtime dir is not a directory: %s", root)
		}
	} else if !os.IsNotExist(err) {
		return fmt.Errorf("stat runtime dir: %w", err)
	}
	if err := mkdirAll(root, dirMode); err != nil {
		return fmt.Errorf("create runtime dir: %w", err)
	}
	if err := chmod(root, dirMode); err != nil {
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
	if err := chmod(socketPath, socketMode); err != nil {
		_ = listener.Close()
		_ = os.Remove(socketPath)
		return nil, fmt.Errorf("chmod unix socket: %w", err)
	}
	return listener, nil
}

// ValidateExecutable ensures a configured plugin path points to one executable file.
func ValidateExecutable(path string) error {
	if path == "" {
		return fmt.Errorf("executable path is required")
	}
	if !filepath.IsAbs(path) {
		return fmt.Errorf("plugin executable path must be absolute: %s", path)
	}
	info, err := os.Lstat(path)
	if err != nil {
		return fmt.Errorf("stat executable: %w", err)
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return fmt.Errorf("plugin executable must not be a symlink: %s", path)
	}
	if !info.Mode().IsRegular() {
		return fmt.Errorf("plugin executable must be a regular file: %s", path)
	}
	if info.Mode()&0o111 == 0 {
		return fmt.Errorf("executable path is not executable: %s", path)
	}
	if info.Mode().Perm()&0o022 != 0 {
		return fmt.Errorf("plugin executable must not be group/world writable: %s", path)
	}
	return nil
}
