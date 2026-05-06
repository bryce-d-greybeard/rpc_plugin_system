package kernel

import (
	"path/filepath"
	"testing"

	"rpc_plugin_system/plugin-authoring-sdk/test-plugin-api/testpluginapi"
)

func writeBehaviorConfig(t *testing.T, runtimeDir string, cfg testpluginapi.Env) string {
	t.Helper()
	path := filepath.Join(runtimeDir, "behavior.json")
	if err := testpluginapi.WriteConfig(path, cfg); err != nil {
		t.Fatalf("write behavior config: %v", err)
	}
	return path
}
