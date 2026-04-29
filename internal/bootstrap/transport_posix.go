//go:build !windows

package bootstrap

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"syscall"
)

type FIFOTransport struct {
	path string
}

func NewFIFOTransport(runtimeDir, pluginID, sessionID string) (*FIFOTransport, error) {
	if runtimeDir == "" || pluginID == "" || sessionID == "" {
		return nil, fmt.Errorf("runtime dir, plugin id, and session id are required")
	}
	path := filepath.Join(runtimeDir, pluginID+"."+sessionID+".bootstrap")
	_ = os.Remove(path)
	if err := syscall.Mkfifo(path, 0o600); err != nil {
		return nil, fmt.Errorf("mkfifo: %w", err)
	}
	return &FIFOTransport{path: path}, nil
}

func (t *FIFOTransport) Endpoint() string { return t.path }

func (t *FIFOTransport) OpenWriter() (io.WriteCloser, error) {
	f, err := os.OpenFile(t.path, os.O_WRONLY, 0)
	if err != nil {
		return nil, fmt.Errorf("open fifo writer: %w", err)
	}
	return f, nil
}

func (t *FIFOTransport) OpenReader() (io.ReadCloser, error) {
	f, err := os.OpenFile(t.path, os.O_RDONLY, 0)
	if err != nil {
		return nil, fmt.Errorf("open fifo reader: %w", err)
	}
	return f, nil
}

func (t *FIFOTransport) Cleanup() error {
	if err := os.Remove(t.path); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("remove fifo: %w", err)
	}
	return nil
}
