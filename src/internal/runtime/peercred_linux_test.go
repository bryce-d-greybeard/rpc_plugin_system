//go:build linux

package runtime

import (
	"errors"
	"net"
	"os"
	"strings"
	"syscall"
	"testing"

	"golang.org/x/sys/unix"
)

type fakeRawConn struct {
	controlErr error
}

func (c fakeRawConn) Control(f func(uintptr)) error {
	if c.controlErr != nil {
		return c.controlErr
	}
	f(0)
	return nil
}

func (c fakeRawConn) Read(func(uintptr) bool) error  { return nil }
func (c fakeRawConn) Write(func(uintptr) bool) error { return nil }

func newTestUnixConn(t *testing.T) *net.UnixConn {
	t.Helper()
	path := SocketPath(t.TempDir(), "peercred-seam")
	listener, err := ListenUnix(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = listener.Close() })

	accepted := make(chan net.Conn, 1)
	acceptErr := make(chan error, 1)
	go func() {
		conn, err := listener.Accept()
		if err != nil {
			acceptErr <- err
			return
		}
		accepted <- conn
	}()

	client, err := net.Dial("unix", path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = client.Close() })

	select {
	case err := <-acceptErr:
		t.Fatal(err)
	case conn := <-accepted:
		t.Cleanup(func() { _ = conn.Close() })
		return conn.(*net.UnixConn)
	}
	panic("unreachable")
}

func TestReadPeerCredRejectsNonUnixConn(t *testing.T) {
	server, client := net.Pipe()
	defer server.Close()
	defer client.Close()

	_, err := ReadPeerCred(server)
	if err == nil {
		t.Fatal("expected non-unix connection rejection")
	}
	if !strings.Contains(err.Error(), "requires unix conn") {
		t.Fatalf("expected unix conn error, got %v", err)
	}
}

func TestReadPeerCredReportsSyscallConnFailure(t *testing.T) {
	conn := newTestUnixConn(t)
	sentinel := errors.New("syscall conn failed")
	oldSyscallConn := unixConnSyscallConn
	unixConnSyscallConn = func(*net.UnixConn) (syscall.RawConn, error) {
		return nil, sentinel
	}
	t.Cleanup(func() { unixConnSyscallConn = oldSyscallConn })

	_, err := ReadPeerCred(conn)
	if err == nil {
		t.Fatal("expected SyscallConn failure")
	}
	if !strings.Contains(err.Error(), "peercred syscall conn") || !errors.Is(err, sentinel) {
		t.Fatalf("expected wrapped SyscallConn error, got %v", err)
	}
}

func TestReadPeerCredReportsGetsockoptFailure(t *testing.T) {
	conn := newTestUnixConn(t)
	sentinel := errors.New("getsockopt failed")
	oldSyscallConn := unixConnSyscallConn
	oldGetsockoptUcred := getsockoptUcred
	unixConnSyscallConn = func(*net.UnixConn) (syscall.RawConn, error) {
		return fakeRawConn{}, nil
	}
	getsockoptUcred = func(int, int, int) (*unix.Ucred, error) {
		return nil, sentinel
	}
	t.Cleanup(func() {
		unixConnSyscallConn = oldSyscallConn
		getsockoptUcred = oldGetsockoptUcred
	})

	_, err := ReadPeerCred(conn)
	if err == nil {
		t.Fatal("expected GetsockoptUcred failure")
	}
	if !strings.Contains(err.Error(), "getsockopt SO_PEERCRED") || !errors.Is(err, sentinel) {
		t.Fatalf("expected wrapped GetsockoptUcred error, got %v", err)
	}
}

func TestReadPeerCredRejectsClosedUnixConn(t *testing.T) {
	path := SocketPath(t.TempDir(), "closed-peercred")
	listener, err := ListenUnix(path)
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()

	accepted := make(chan net.Conn, 1)
	acceptErr := make(chan error, 1)
	go func() {
		conn, err := listener.Accept()
		if err != nil {
			acceptErr <- err
			return
		}
		accepted <- conn
	}()

	client, err := net.Dial("unix", path)
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()

	var conn net.Conn
	select {
	case err := <-acceptErr:
		t.Fatal(err)
	case conn = <-accepted:
	}
	unixConn := conn.(*net.UnixConn)
	if err := unixConn.Close(); err != nil {
		t.Fatal(err)
	}

	_, err = ReadPeerCred(unixConn)
	if err == nil {
		t.Fatal("expected closed unix connection rejection")
	}
	if !strings.Contains(err.Error(), "peercred control") {
		t.Fatalf("expected peercred control error, got %v", err)
	}
}

func TestReadPeerCredReturnsUnixPeerCredentials(t *testing.T) {
	path := SocketPath(t.TempDir(), "peercred")
	listener, err := ListenUnix(path)
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()

	type result struct {
		cred PeerCred
		err  error
	}
	accepted := make(chan result, 1)
	go func() {
		conn, err := listener.Accept()
		if err != nil {
			accepted <- result{err: err}
			return
		}
		defer conn.Close()
		cred, err := ReadPeerCred(conn)
		accepted <- result{cred: cred, err: err}
	}()

	client, err := net.Dial("unix", path)
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()

	got := <-accepted
	if got.err != nil {
		t.Fatalf("ReadPeerCred failed: %v", got.err)
	}
	if got.cred.PID != os.Getpid() {
		t.Fatalf("PID = %d, want %d", got.cred.PID, os.Getpid())
	}
	if got.cred.UID != uint32(os.Getuid()) {
		t.Fatalf("UID = %d, want %d", got.cred.UID, os.Getuid())
	}
	if got.cred.GID != uint32(os.Getgid()) {
		t.Fatalf("GID = %d, want %d", got.cred.GID, os.Getgid())
	}
}
