package kernel

import (
	"strings"
	"testing"
)

func TestValidatePluginID(t *testing.T) {
	for _, id := range []string{"echo", "echo-1", "echo_1", "echo.v1", "1echo"} {
		t.Run("accept "+id, func(t *testing.T) {
			if err := ValidatePluginID(id); err != nil {
				t.Fatalf("ValidatePluginID(%q): %v", id, err)
			}
		})
	}
	for _, tc := range []struct {
		id   string
		want string
	}{
		{id: "", want: "plugin id is required"},
		{id: "../echo", want: "invalid plugin id"},
		{id: "echo/test", want: "invalid plugin id"},
		{id: "-echo", want: "invalid plugin id"},
	} {
		t.Run("reject "+tc.id, func(t *testing.T) {
			err := ValidatePluginID(tc.id)
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("ValidatePluginID(%q) err = %v, want substring %q", tc.id, err, tc.want)
			}
		})
	}
}

func TestNewHostRejectsInvalidPluginIDBeforePluginPathValidation(t *testing.T) {
	_, err := NewHost(HostConfig{
		RuntimeDir: t.TempDir(),
		Plugins:    []PluginConfig{{PluginID: "../echo", PluginPath: "/missing/plugin"}},
	})
	if err == nil || !strings.Contains(err.Error(), "invalid plugin id") {
		t.Fatalf("NewHost err = %v, want invalid plugin id", err)
	}
}
