package testpluginapi

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"
)

var legacyEnvKeys = []string{
	BehaviorConfigEnv,
	"RPC_PLUGIN_SYSTEM_PLUGIN_BEHAVIOR_ID",
	"RPC_PLUGIN_SYSTEM_PLUGIN_BEHAVIOR_VERSION",
	"RPC_PLUGIN_SYSTEM_PLUGIN_BEHAVIOR_GENERATION_OFFSET",
	"RPC_PLUGIN_SYSTEM_PLUGIN_BEHAVIOR_CAPABILITIES_MODE",
	"RPC_PLUGIN_SYSTEM_PLUGIN_BEHAVIOR_HEARTBEAT_STATUS",
	"RPC_PLUGIN_SYSTEM_PLUGIN_BEHAVIOR_HEARTBEAT_ERRORS",
	"RPC_PLUGIN_SYSTEM_PLUGIN_BEHAVIOR_HEARTBEAT_DELAY_MS",
	"RPC_PLUGIN_SYSTEM_PLUGIN_BEHAVIOR_SLEEP_SCALE",
	"RPC_PLUGIN_SYSTEM_PLUGIN_BEHAVIOR_CRASH_ON_ECHO",
	"RPC_PLUGIN_SYSTEM_PLUGIN_BEHAVIOR_SHUTDOWN_DELAY_MS",
	"RPC_PLUGIN_SYSTEM_PLUGIN_BEHAVIOR_FAIL_AUTH",
	"RPC_PLUGIN_SYSTEM_PLUGIN_BEHAVIOR_CLOSE_ON_ACCEPT",
	"RPC_PLUGIN_SYSTEM_PLUGIN_BEHAVIOR_CLOSE_ON_ECHO",
}

func clearBehaviorEnv(t *testing.T) {
	t.Helper()
	for _, key := range legacyEnvKeys {
		t.Setenv(key, "")
	}
}

func TestEnvDurations(t *testing.T) {
	t.Parallel()

	if got := (Env{HeartbeatDelayMS: -1}).HeartbeatDelay(); got != 0 {
		t.Fatalf("negative heartbeat delay = %s, want 0", got)
	}
	if got := (Env{HeartbeatDelayMS: 25}).HeartbeatDelay(); got != 25*time.Millisecond {
		t.Fatalf("heartbeat delay = %s, want 25ms", got)
	}

	if got := (Env{}).ShutdownDelay(); got != 50*time.Millisecond {
		t.Fatalf("default shutdown delay = %s, want 50ms", got)
	}
	if got := (Env{ShutdownDelayMS: -1}).ShutdownDelay(); got != 50*time.Millisecond {
		t.Fatalf("negative shutdown delay = %s, want 50ms", got)
	}
	if got := (Env{ShutdownDelayMS: 125}).ShutdownDelay(); got != 125*time.Millisecond {
		t.Fatalf("shutdown delay = %s, want 125ms", got)
	}
}

func TestLoadEnvReadsLegacyBehaviorControls(t *testing.T) {
	clearBehaviorEnv(t)
	t.Setenv("RPC_PLUGIN_SYSTEM_PLUGIN_BEHAVIOR_ID", "plugin-a")
	t.Setenv("RPC_PLUGIN_SYSTEM_PLUGIN_BEHAVIOR_VERSION", "9.8.7")
	t.Setenv("RPC_PLUGIN_SYSTEM_PLUGIN_BEHAVIOR_GENERATION_OFFSET", "42")
	t.Setenv("RPC_PLUGIN_SYSTEM_PLUGIN_BEHAVIOR_CAPABILITIES_MODE", "empty")
	t.Setenv("RPC_PLUGIN_SYSTEM_PLUGIN_BEHAVIOR_HEARTBEAT_STATUS", string(StatusDegraded))
	t.Setenv("RPC_PLUGIN_SYSTEM_PLUGIN_BEHAVIOR_HEARTBEAT_ERRORS", "3")
	t.Setenv("RPC_PLUGIN_SYSTEM_PLUGIN_BEHAVIOR_HEARTBEAT_DELAY_MS", "250")
	t.Setenv("RPC_PLUGIN_SYSTEM_PLUGIN_BEHAVIOR_SLEEP_SCALE", "0.25")
	t.Setenv("RPC_PLUGIN_SYSTEM_PLUGIN_BEHAVIOR_CRASH_ON_ECHO", "true")
	t.Setenv("RPC_PLUGIN_SYSTEM_PLUGIN_BEHAVIOR_SHUTDOWN_DELAY_MS", "75")
	t.Setenv("RPC_PLUGIN_SYSTEM_PLUGIN_BEHAVIOR_FAIL_AUTH", "true")
	t.Setenv("RPC_PLUGIN_SYSTEM_PLUGIN_BEHAVIOR_CLOSE_ON_ACCEPT", "true")
	t.Setenv("RPC_PLUGIN_SYSTEM_PLUGIN_BEHAVIOR_CLOSE_ON_ECHO", "true")

	got := LoadEnv()
	want := Env{
		PluginID:         "plugin-a",
		Version:          "9.8.7",
		GenerationOffset: 42,
		CapabilitiesMode: "empty",
		HeartbeatStatus:  StatusDegraded,
		HeartbeatErrors:  3,
		HeartbeatDelayMS: 250,
		SleepScale:       0.25,
		CrashOnEcho:      true,
		ShutdownDelayMS:  75,
		FailAuth:         true,
		CloseOnAccept:    true,
		CloseOnEcho:      true,
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("LoadEnv() = %#v, want %#v", got, want)
	}
}

func TestLoadEnvUsesDefaultsForMissingAndMalformedValues(t *testing.T) {
	clearBehaviorEnv(t)
	t.Setenv("RPC_PLUGIN_SYSTEM_PLUGIN_BEHAVIOR_GENERATION_OFFSET", "not-an-int")
	t.Setenv("RPC_PLUGIN_SYSTEM_PLUGIN_BEHAVIOR_HEARTBEAT_ERRORS", "not-an-int")
	t.Setenv("RPC_PLUGIN_SYSTEM_PLUGIN_BEHAVIOR_HEARTBEAT_DELAY_MS", "not-an-int")
	t.Setenv("RPC_PLUGIN_SYSTEM_PLUGIN_BEHAVIOR_SLEEP_SCALE", "not-a-float")
	t.Setenv("RPC_PLUGIN_SYSTEM_PLUGIN_BEHAVIOR_SHUTDOWN_DELAY_MS", "not-an-int")
	t.Setenv("RPC_PLUGIN_SYSTEM_PLUGIN_BEHAVIOR_CRASH_ON_ECHO", "not-a-bool")
	t.Setenv("RPC_PLUGIN_SYSTEM_PLUGIN_BEHAVIOR_FAIL_AUTH", "not-a-bool")
	t.Setenv("RPC_PLUGIN_SYSTEM_PLUGIN_BEHAVIOR_CLOSE_ON_ACCEPT", "not-a-bool")
	t.Setenv("RPC_PLUGIN_SYSTEM_PLUGIN_BEHAVIOR_CLOSE_ON_ECHO", "not-a-bool")

	got := LoadEnv()
	want := Env{
		Version:          "0.1.0",
		CapabilitiesMode: "normal",
		HeartbeatStatus:  StatusHealthy,
		SleepScale:       1,
		ShutdownDelayMS:  50,
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("LoadEnv() = %#v, want defaults %#v", got, want)
	}
}

func TestWriteConfigAndLoadConfigRoundTripWithConfigDefaults(t *testing.T) {
	clearBehaviorEnv(t)
	path := filepath.Join(t.TempDir(), "behavior.json")

	in := Env{PluginID: "from-file", GenerationOffset: 7, ShutdownDelayMS: 10}
	if err := WriteConfig(path, in); err != nil {
		t.Fatalf("WriteConfig() error = %v", err)
	}

	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat written config: %v", err)
	}
	if got := info.Mode().Perm(); got != 0o600 {
		t.Fatalf("config permissions = %o, want 600", got)
	}

	t.Setenv(BehaviorConfigEnv, path)
	got := LoadConfig()
	want := in
	want.Version = "0.1.0"
	want.HeartbeatStatus = StatusHealthy
	want.SleepScale = 1
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("LoadConfig() = %#v, want %#v", got, want)
	}
}

func TestLoadConfigFallsBackToLegacyEnvWhenConfigUnavailableOrInvalid(t *testing.T) {
	for _, tc := range []struct {
		name string
		path func(t *testing.T) string
	}{
		{
			name: "missing file",
			path: func(t *testing.T) string { return filepath.Join(t.TempDir(), "missing.json") },
		},
		{
			name: "invalid json",
			path: func(t *testing.T) string {
				path := filepath.Join(t.TempDir(), "invalid.json")
				if err := os.WriteFile(path, []byte("{"), 0o600); err != nil {
					t.Fatalf("write invalid config: %v", err)
				}
				return path
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			clearBehaviorEnv(t)
			t.Setenv(BehaviorConfigEnv, tc.path(t))
			t.Setenv("RPC_PLUGIN_SYSTEM_PLUGIN_BEHAVIOR_ID", "from-env")
			t.Setenv("RPC_PLUGIN_SYSTEM_PLUGIN_BEHAVIOR_VERSION", "1.2.3")

			got := LoadConfig()
			if got.PluginID != "from-env" || got.Version != "1.2.3" {
				t.Fatalf("LoadConfig() fallback = %#v, want legacy env id/version", got)
			}
		})
	}
}

func TestWriteConfigReportsWriteErrors(t *testing.T) {
	dir := t.TempDir()
	if err := WriteConfig(dir, Env{}); err == nil {
		t.Fatalf("WriteConfig(directory) error = nil, want failure")
	}
}
