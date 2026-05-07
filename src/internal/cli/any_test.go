package cli

import (
	"bytes"
	"errors"
	"strings"
	"testing"
)

type errorWriter struct {
	err error
}

func (w errorWriter) Write([]byte) (int, error) {
	return 0, w.err
}

type failAfterWriter struct {
	remaining int
	err       error
}

func (w *failAfterWriter) Write(p []byte) (int, error) {
	if w.remaining == 0 {
		return 0, w.err
	}
	w.remaining--
	return len(p), nil
}

func TestWriteAny(t *testing.T) {
	var buf bytes.Buffer
	value := map[string]any{"route": "echo", "methods": []string{"heartbeat", "echo"}}
	if err := WriteAny(&buf, value); err != nil {
		t.Fatalf("write any: %v", err)
	}
	out := buf.String()
	for _, want := range []string{"\"route\": \"echo\"", "\"heartbeat\"", "\"echo\""} {
		if !strings.Contains(out, want) {
			t.Fatalf("output missing %q in %s", want, out)
		}
	}
}

func TestWriteAnyEncodeError(t *testing.T) {
	wantErr := errors.New("disk full")
	err := WriteAny(errorWriter{err: wantErr}, map[string]string{"route": "echo"})
	if err == nil {
		t.Fatalf("expected error")
	}
	if !strings.Contains(err.Error(), "encode response") || !errors.Is(err, wantErr) {
		t.Fatalf("error = %v, want wrapped encode response", err)
	}
}
