package bootstrap

import (
	"bytes"
	"io"
	"net"
	"testing"
)

func TestSecureConnRoundTrip(t *testing.T) {
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
	defer a.Close()
	defer b.Close()

	client, err := NewLabeledSecureConn(a, keys.SendKey, keys.RecvKey, 1, "kernel-to-plugin", "plugin-to-kernel")
	if err != nil {
		t.Fatalf("NewSecureConn client: %v", err)
	}
	server, err := NewLabeledSecureConn(b, keys.RecvKey, keys.SendKey, 1, "plugin-to-kernel", "kernel-to-plugin")
	if err != nil {
		t.Fatalf("NewSecureConn server: %v", err)
	}

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
