package plugin

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoadConfigFromEnvMissingVarsAreExplicit(t *testing.T) {
	for _, name := range []string{
		"RPC_PLUGIN_SYSTEM_PLUGIN_SOCKET",
		"RPC_PLUGIN_SYSTEM_PLUGIN_ID",
		"RPC_PLUGIN_SYSTEM_PLUGIN_GENERATION",
		"RPC_PLUGIN_SYSTEM_AUTH_TOKEN_FILE",
	} {
		t.Run(name, func(t *testing.T) {
			clearPluginEnv(t)
			setValidPluginEnv(t)
			os.Unsetenv(name)

			_, err := LoadConfigFromEnv()
			if err == nil {
				t.Fatal("expected missing env error")
			}
			var missing ErrMissingEnv
			if !errors.As(err, &missing) {
				t.Fatalf("expected ErrMissingEnv, got %T: %v", err, err)
			}
			if missing.Name != name {
				t.Fatalf("missing env name = %q want %q", missing.Name, name)
			}
		})
	}
}

func TestLoadConfigFromEnvInvalidGenerationNamesVariable(t *testing.T) {
	clearPluginEnv(t)
	setValidPluginEnv(t)
	if err := os.Setenv("RPC_PLUGIN_SYSTEM_PLUGIN_GENERATION", "abc"); err != nil {
		t.Fatalf("set generation: %v", err)
	}

	_, err := LoadConfigFromEnv()
	if err == nil {
		t.Fatal("expected parse error")
	}
	if !strings.Contains(err.Error(), "RPC_PLUGIN_SYSTEM_PLUGIN_GENERATION") {
		t.Fatalf("parse error should name variable, got: %v", err)
	}
}

func TestNewTemplateBuildsMinimalHealthyPlugin(t *testing.T) {
	cfg := Config{PluginID: "echo", GenerationID: 7}
	p := NewTemplate(cfg, "1.2.3")

	if p.PluginID != "echo" || p.GenerationID != 7 || p.Version() != "1.2.3" || p.StartedAt.IsZero() {
		t.Fatalf("unexpected template plugin: %+v", p)
	}

	var hb HeartbeatResponse
	if err := p.Heartbeat(Empty{}, &hb); err != nil {
		t.Fatalf("heartbeat: %v", err)
	}
	if hb.PluginID != "echo" || hb.GenerationID != 7 || hb.Version != "1.2.3" || hb.Status != StatusHealthy {
		t.Fatalf("unexpected heartbeat: %+v", hb)
	}
}

func TestServerRejectsNonAuthMethodsBeforeAuth(t *testing.T) {
	srv := &server{core: NewTemplate(Config{PluginID: "echo", GenerationID: 7}, "1.2.3"), cfg: Config{PluginID: "echo", GenerationID: 7, AuthToken: "secret"}}

	var caps CapabilitiesResponse
	if err := srv.Capabilities(Empty{}, &caps); !errors.Is(err, os.ErrPermission) {
		t.Fatalf("capabilities before auth err = %v, want permission", err)
	}

	var hb HeartbeatResponse
	if err := srv.Heartbeat(Empty{}, &hb); !errors.Is(err, os.ErrPermission) {
		t.Fatalf("heartbeat before auth err = %v, want permission", err)
	}

	var authOut AuthResponse
	if err := srv.Auth(AuthRequest{Token: "secret"}, &authOut); err != nil {
		t.Fatalf("auth: %v", err)
	}

	if err := srv.Capabilities(Empty{}, &caps); err != nil {
		t.Fatalf("capabilities after auth: %v", err)
	}
	if caps.PluginID != "echo" || caps.GenerationID != 7 || caps.Version != "1.2.3" {
		t.Fatalf("unexpected capabilities response: %+v", caps)
	}

	if err := srv.Heartbeat(Empty{}, &hb); err != nil {
		t.Fatalf("heartbeat after auth: %v", err)
	}
}

func clearPluginEnv(t *testing.T) {
	t.Helper()
	for _, name := range []string{
		"RPC_PLUGIN_SYSTEM_PLUGIN_SOCKET",
		"RPC_PLUGIN_SYSTEM_PLUGIN_ID",
		"RPC_PLUGIN_SYSTEM_PLUGIN_GENERATION",
		"RPC_PLUGIN_SYSTEM_AUTH_TOKEN_FILE",
	} {
		if err := os.Unsetenv(name); err != nil {
			t.Fatalf("unset %s: %v", name, err)
		}
	}
}

func setValidPluginEnv(t *testing.T) {
	t.Helper()
	dir := t.TempDir()
	authFile := filepath.Join(dir, "token")
	if err := os.WriteFile(authFile, []byte("secret-token"), 0o600); err != nil {
		t.Fatalf("write auth file: %v", err)
	}
	values := map[string]string{
		"RPC_PLUGIN_SYSTEM_PLUGIN_SOCKET":     filepath.Join(dir, "plugin.sock"),
		"RPC_PLUGIN_SYSTEM_PLUGIN_ID":         "echo",
		"RPC_PLUGIN_SYSTEM_PLUGIN_GENERATION": "7",
		"RPC_PLUGIN_SYSTEM_AUTH_TOKEN_FILE":   authFile,
	}
	for name, value := range values {
		if err := os.Setenv(name, value); err != nil {
			t.Fatalf("set %s: %v", name, err)
		}
	}
}
