package cli

import (
	"bytes"
	"strings"
	"testing"

	"rpc_plugin_system/internal/kernel"
)

func TestWriteState(t *testing.T) {
	var buf bytes.Buffer
	state := kernel.State{PluginID: "echo", GenerationID: 2, Healthy: true, PID: 1234}
	if err := WriteState(&buf, state); err != nil {
		t.Fatalf("write state: %v", err)
	}
	out := buf.String()
	for _, needle := range []string{"\"PluginID\": \"echo\"", "\"GenerationID\": 2", "\"Healthy\": true"} {
		if !strings.Contains(out, needle) {
			t.Fatalf("output missing %q in %s", needle, out)
		}
	}
}
