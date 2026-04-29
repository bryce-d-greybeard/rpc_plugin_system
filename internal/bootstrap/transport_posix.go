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
	requestPath  string
	responsePath string
}

func NewFIFOTransport(runtimeDir, pluginID, sessionID string) (*FIFOTransport, error) {
	if runtimeDir == "" || pluginID == "" || sessionID == "" {
		return nil, fmt.Errorf("runtime dir, plugin id, and session id are required")
	}
	requestPath := filepath.Join(runtimeDir, pluginID+"."+sessionID+".bootstrap.in")
	responsePath := filepath.Join(runtimeDir, pluginID+"."+sessionID+".bootstrap.out")
	_ = os.Remove(requestPath)
	_ = os.Remove(responsePath)
	if err := syscall.Mkfifo(requestPath, 0o600); err != nil {
		return nil, fmt.Errorf("mkfifo request: %w", err)
	}
	if err := syscall.Mkfifo(responsePath, 0o600); err != nil {
		_ = os.Remove(requestPath)
		return nil, fmt.Errorf("mkfifo response: %w", err)
	}
	return &FIFOTransport{requestPath: requestPath, responsePath: responsePath}, nil
}

func (t *FIFOTransport) Endpoint() string { return t.requestPath + ":" + t.responsePath }

func (t *FIFOTransport) OpenRequestWriter() (io.WriteCloser, error) {
	f, err := os.OpenFile(t.requestPath, os.O_WRONLY, 0)
	if err != nil {
		return nil, fmt.Errorf("open request fifo writer: %w", err)
	}
	return f, nil
}

func (t *FIFOTransport) OpenRequestReader() (io.ReadCloser, error) {
	f, err := os.OpenFile(t.requestPath, os.O_RDONLY, 0)
	if err != nil {
		return nil, fmt.Errorf("open request fifo reader: %w", err)
	}
	return f, nil
}

func (t *FIFOTransport) OpenResponseWriter() (io.WriteCloser, error) {
	f, err := os.OpenFile(t.responsePath, os.O_WRONLY, 0)
	if err != nil {
		return nil, fmt.Errorf("open response fifo writer: %w", err)
	}
	return f, nil
}

func (t *FIFOTransport) OpenResponseReader() (io.ReadCloser, error) {
	f, err := os.OpenFile(t.responsePath, os.O_RDONLY, 0)
	if err != nil {
		return nil, fmt.Errorf("open response fifo reader: %w", err)
	}
	return f, nil
}

func (t *FIFOTransport) Cleanup() error {
	for _, path := range []string{t.requestPath, t.responsePath} {
		if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
			return fmt.Errorf("remove fifo %s: %w", path, err)
		}
	}
	return nil
}
