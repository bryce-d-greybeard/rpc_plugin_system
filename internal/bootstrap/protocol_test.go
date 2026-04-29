package bootstrap

import "testing"

func TestProtocolVersionIsStable(t *testing.T) {
	if ProtocolVersion != 1 {
		t.Fatalf("ProtocolVersion = %d, want 1", ProtocolVersion)
	}
}
