//go:build !linux

package runtime

import (
	"fmt"
	"net"
)

// PeerCred describes peer credentials for one connected Unix socket.
type PeerCred struct {
	PID int
	UID uint32
	GID uint32
}

// ReadPeerCred is unavailable on non-Linux targets in v1.
func ReadPeerCred(conn net.Conn) (PeerCred, error) {
	return PeerCred{}, fmt.Errorf("peercred is only supported on linux in v1")
}
