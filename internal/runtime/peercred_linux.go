//go:build linux

package runtime

import (
	"fmt"
	"net"

	"golang.org/x/sys/unix"
)

// PeerCred describes Linux peer credentials for one connected Unix socket.
type PeerCred struct {
	PID int
	UID uint32
	GID uint32
}

// ReadPeerCred returns kernel-reported peer credentials for one Unix socket connection.
func ReadPeerCred(conn net.Conn) (PeerCred, error) {
	unixConn, ok := conn.(*net.UnixConn)
	if !ok {
		return PeerCred{}, fmt.Errorf("peercred requires unix conn, got %T", conn)
	}
	rawConn, err := unixConn.SyscallConn()
	if err != nil {
		return PeerCred{}, fmt.Errorf("peercred syscall conn: %w", err)
	}
	var out PeerCred
	var credErr error
	if err := rawConn.Control(func(fd uintptr) {
		cred, err := unix.GetsockoptUcred(int(fd), unix.SOL_SOCKET, unix.SO_PEERCRED)
		if err != nil {
			credErr = fmt.Errorf("getsockopt SO_PEERCRED: %w", err)
			return
		}
		out = PeerCred{PID: int(cred.Pid), UID: cred.Uid, GID: cred.Gid}
	}); err != nil {
		return PeerCred{}, fmt.Errorf("peercred control: %w", err)
	}
	if credErr != nil {
		return PeerCred{}, credErr
	}
	return out, nil
}
