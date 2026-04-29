//go:build windows

package bootstrap

import (
	"fmt"
	"io"
)

type NamedPipeTransport struct {
	endpoint string
}

func NewNamedPipeTransport(runtimeDir, pluginID, sessionID string) (*NamedPipeTransport, error) {
	if pluginID == "" || sessionID == "" {
		return nil, fmt.Errorf("plugin id and session id are required")
	}
	return &NamedPipeTransport{endpoint: `\\.\\pipe\\rpc-plugin-system-` + pluginID + `-` + sessionID}, nil
}

func (t *NamedPipeTransport) Endpoint() string { return t.endpoint }
func (t *NamedPipeTransport) OpenWriter() (io.WriteCloser, error) { return nil, fmt.Errorf("not implemented on windows") }
func (t *NamedPipeTransport) OpenReader() (io.ReadCloser, error) { return nil, fmt.Errorf("not implemented on windows") }
func (t *NamedPipeTransport) Cleanup() error { return nil }
