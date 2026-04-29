package bootstrap

import (
	"bytes"
	"io"
	"net"
	"testing"
	"time"
)

func TestSecureConnRekeyBoundaries(t *testing.T) {
	policy := DefaultRekeyPolicy()
	if err := checkRekeyBoundary(policy, time.Now().UTC().Add(-61*time.Minute), 0, 0, 0); err == nil {
		t.Fatal("expected age limit error")
	}
	if err := checkRekeyBoundary(policy, time.Now().UTC(), policy.MaxFramesPerDirection, 0, 0); err == nil {
		t.Fatal("expected frame limit error")
	}
	if err := checkRekeyBoundary(policy, time.Now().UTC(), 0, policy.MaxBytesPerDirection, 1); err == nil {
		t.Fatal("expected byte limit error")
	}
}

func TestSecureConnRoundTrip(t *testing.T) {
	client, server := testSecureConnPair(t, DefaultRekeyPolicy())
	defer client.Close()
	defer server.Close()

	want := []byte("hello over secure conn")
	errCh := make(chan error, 1)
	go func() {
		buf := make([]byte, len(want))
		if _, err := io.ReadFull(server, buf); err != nil {
			errCh <- err
			return
		}
		if !bytes.Equal(buf, want) {
			errCh <- io.ErrUnexpectedEOF
			return
		}
		errCh <- nil
	}()

	if _, err := client.Write(want); err != nil {
		t.Fatalf("Write: %v", err)
	}
	if err := <-errCh; err != nil {
		t.Fatalf("ReadFull: %v", err)
	}
}

func TestSecureConnRekeysAcrossFrameBoundary(t *testing.T) {
	client, server := testSecureConnPair(t, RekeyPolicy{MaxFramesPerDirection: 2, MaxBytesPerDirection: 1 << 20, MaxConnectionAge: time.Hour})
	defer client.Close()
	defer server.Close()

	want := [][]byte{[]byte("one"), []byte("two"), []byte("three"), []byte("four")}
	errCh := make(chan error, 1)
	go func() {
		for _, msg := range want {
			buf := make([]byte, len(msg))
			if _, err := io.ReadFull(server, buf); err != nil {
				errCh <- err
				return
			}
			if !bytes.Equal(buf, msg) {
				errCh <- io.ErrUnexpectedEOF
				return
			}
		}
		errCh <- nil
	}()

	for _, msg := range want {
		if _, err := client.Write(msg); err != nil {
			t.Fatalf("Write(%q): %v", msg, err)
		}
	}
	if err := <-errCh; err != nil {
		t.Fatalf("ReadFull: %v", err)
	}
}

func TestSecureConnRekeysAcrossByteBoundary(t *testing.T) {
	client, server := testSecureConnPair(t, RekeyPolicy{MaxFramesPerDirection: 16, MaxBytesPerDirection: 20, MaxConnectionAge: time.Hour})
	defer client.Close()
	defer server.Close()

	want := [][]byte{[]byte("abcd"), []byte("efgh"), []byte("ijkl")}
	errCh := make(chan error, 1)
	go func() {
		for _, msg := range want {
			buf := make([]byte, len(msg))
			if _, err := io.ReadFull(server, buf); err != nil {
				errCh <- err
				return
			}
			if !bytes.Equal(buf, msg) {
				errCh <- io.ErrUnexpectedEOF
				return
			}
		}
		errCh <- nil
	}()

	for _, msg := range want {
		if _, err := client.Write(msg); err != nil {
			t.Fatalf("Write(%q): %v", msg, err)
		}
	}
	if err := <-errCh; err != nil {
		t.Fatalf("ReadFull: %v", err)
	}
}

func testSecureConnPair(t *testing.T, policy RekeyPolicy) (net.Conn, net.Conn) {
	t.Helper()
	root := bytes.Repeat([]byte{0x42}, RootKeySize)
	keys, err := NewKeys(root)
	if err != nil {
		t.Fatalf("NewKeys: %v", err)
	}
	keys, err = Rekey(keys)
	if err != nil {
		t.Fatalf("Rekey: %v", err)
	}

	a, b := net.Pipe()
	client, err := NewLabeledSecureConnWithPolicy(a, keys.SendKey, keys.RecvKey, 1, "echo", 7, "s1", "kernel-to-plugin", "plugin-to-kernel", policy)
	if err != nil {
		t.Fatalf("NewSecureConn client: %v", err)
	}
	server, err := NewLabeledSecureConnWithPolicy(b, keys.RecvKey, keys.SendKey, 1, "echo", 7, "s1", "plugin-to-kernel", "kernel-to-plugin", policy)
	if err != nil {
		t.Fatalf("NewSecureConn server: %v", err)
	}
	return client, server
}
