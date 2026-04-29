//go:build !windows

package bootstrap

import (
	"io"
	"testing"
)

func TestFIFOTransportLifecycle(t *testing.T) {
	t.Parallel()
	tr, err := NewFIFOTransport(t.TempDir(), "echo", "session1")
	if err != nil {
		t.Fatalf("NewFIFOTransport: %v", err)
	}
	defer tr.Cleanup()

	done := make(chan error, 1)
	go func() {
		r, err := tr.OpenReader()
		if err != nil {
			done <- err
			return
		}
		defer r.Close()
		buf, err := io.ReadAll(r)
		if err != nil {
			done <- err
			return
		}
		if string(buf) != "hello" {
			done <- io.ErrUnexpectedEOF
			return
		}
		done <- nil
	}()

	w, err := tr.OpenWriter()
	if err != nil {
		t.Fatalf("OpenWriter: %v", err)
	}
	if _, err := w.Write([]byte("hello")); err != nil {
		t.Fatalf("Write: %v", err)
	}
	if err := w.Close(); err != nil {
		t.Fatalf("Close writer: %v", err)
	}
	if err := <-done; err != nil {
		t.Fatalf("reader: %v", err)
	}
}
