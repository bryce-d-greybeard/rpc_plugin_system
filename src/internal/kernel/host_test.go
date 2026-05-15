package kernel

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"rpc_plugin_system/internal/eventlog"
	"rpc_plugin_system/internal/providerbundle"
	"rpc_plugin_system/test/testpluginapi"
)

func TestHostStartStateAndRestartPlugin(t *testing.T) {
	pluginBin := buildPlugin(t)
	runtimeDir := t.TempDir()

	host, err := NewHost(HostConfig{
		RuntimeDir:     runtimeDir,
		DialTimeout:    2 * time.Second,
		CallTimeout:    200 * time.Millisecond,
		HeartbeatEvery: 100 * time.Millisecond,
		Plugins: []PluginConfig{
			{PluginID: "echo-a", PluginPath: pluginBin},
			{PluginID: "echo-b", PluginPath: pluginBin},
		},
	})
	if err != nil {
		t.Fatalf("new host: %v", err)
	}
	defer host.Close()

	if err := host.StartAll(); err != nil {
		t.Fatalf("start all: %v", err)
	}

	state := host.State()
	if len(state.Plugins) != 2 {
		t.Fatalf("plugin count = %d, want 2", len(state.Plugins))
	}
	for _, plugin := range state.Plugins {
		if !plugin.Healthy {
			t.Fatalf("plugin %s not healthy", plugin.PluginID)
		}
		wantPrefix := filepath.Join(runtimeDir, plugin.PluginID)
		if filepath.Dir(plugin.SocketPath) != wantPrefix {
			t.Fatalf("plugin %s socket dir = %s, want %s", plugin.PluginID, filepath.Dir(plugin.SocketPath), wantPrefix)
		}
	}
	if got := state.CapabilityMap["echo"]; len(got) != 2 {
		t.Fatalf("echo capability plugins = %v, want both plugins", got)
	}
	if len(state.Routes) != 2 {
		t.Fatalf("route count = %d, want 2", len(state.Routes))
	}

	before, err := host.Manager("echo-a")
	if err != nil {
		t.Fatalf("manager echo-a: %v", err)
	}
	beforeState := before.State()

	afterState, err := host.RestartPlugin("echo-a")
	if err != nil {
		t.Fatalf("restart plugin: %v", err)
	}
	if afterState.GenerationID <= beforeState.GenerationID {
		t.Fatalf("generation did not advance: got %d want > %d", afterState.GenerationID, beforeState.GenerationID)
	}

	other, err := host.Manager("echo-b")
	if err != nil {
		t.Fatalf("manager echo-b: %v", err)
	}
	if !other.State().Healthy {
		t.Fatal("other plugin should remain healthy")
	}
}

func TestCoreSnapshotCarriesDeclaredBundleProvenanceWithoutAuthorityMaterial(t *testing.T) {
	pluginBin := buildPlugin(t)
	runtimeDir := t.TempDir()
	metadata := kernelProviderBundleTestMetadata("echo", 1)
	metadata.BundleRootPath = "/srv/providers/echo/socket-owner"
	metadata.ManifestPath = "/srv/providers/echo/manifest.json"
	metadata.LuaAssets[0].Path = "/srv/providers/echo/lua/authority_use_ref.lua"
	metadata.RedactedErrorCode = "authority_use_ref_denied"
	metadata.RedactedErrorMessage = "session socket handle rejected"
	metadata.SubstrateEventCorrelation = "corr-123"

	host, err := NewHost(HostConfig{
		RuntimeDir:     runtimeDir,
		DialTimeout:    2 * time.Second,
		CallTimeout:    200 * time.Millisecond,
		HeartbeatEvery: 100 * time.Millisecond,
		Plugins: []PluginConfig{{
			PluginID:               "echo",
			PluginPath:             pluginBin,
			ProviderBundleMetadata: &metadata,
		}},
	})
	if err != nil {
		t.Fatalf("new host: %v", err)
	}
	defer host.Close()
	if err := host.StartAll(); err != nil {
		t.Fatalf("start all: %v", err)
	}

	snapshot := host.CoreSnapshot()
	if len(snapshot.Plugins) != 1 || snapshot.Plugins[0].DeclaredProviderBundleMetadata == nil {
		t.Fatalf("core snapshot missing declared metadata: %#v", snapshot)
	}
	dto := snapshot.Plugins[0].DeclaredProviderBundleMetadata
	if dto.Kind != "declared_provider_bundle_metadata" || dto.Name != "declared_provider_bundle_metadata" {
		t.Fatalf("core projection did not use declared metadata identity: %#v", dto)
	}
	if dto.PluginID != "echo" || dto.PluginGeneration != 1 || dto.SchemaVersion != "tool-skills.v1" || dto.ValidationStatus != providerbundle.ProviderBundleStatusDeclaredValid {
		t.Fatalf("core projection lost identity/status facts: %#v", dto)
	}
	if len(dto.LuaAssetDigests) != 1 || dto.LuaAssetDigests[0].Value == "" || dto.ManifestDigest.Value == "" || dto.SubstrateEventCorrelation != "corr-123" {
		t.Fatalf("core projection lost provenance/digest facts: %#v", dto)
	}
	encoded, err := json.Marshal(snapshot)
	if err != nil {
		t.Fatalf("marshal core snapshot: %v", err)
	}
	text := string(encoded)
	for _, forbidden := range []string{"/srv/providers", "authority_use_ref", "socket", "handle", "session"} {
		if strings.Contains(text, forbidden) {
			t.Fatalf("core snapshot leaked authority/path material %q in %s", forbidden, text)
		}
	}
}

func kernelProviderBundleTestMetadata(pluginID string, generation int64) providerbundle.ProviderBundleMetadata {
	return providerbundle.ProviderBundleMetadata{
		PluginID:         pluginID,
		PluginGeneration: generation,
		BundleRootPath:   "/srv/providers/echo/tool-skills",
		ManifestPath:     "/srv/providers/echo/tool-skills/manifest.json",
		LuaAssets: []providerbundle.ProviderBundleAsset{{
			Path: "/srv/providers/echo/tool-skills/lua/echo.lua",
			Digest: providerbundle.ProviderBundleDigest{
				Algorithm: "sha256",
				Value:     "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef",
			},
		}},
		ManifestDigest: providerbundle.ProviderBundleDigest{
			Algorithm: "sha256",
			Value:     "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef",
		},
		SchemaVersion:             "tool-skills.v1",
		ValidationStatus:          providerbundle.ProviderBundleStatusDeclaredValid,
		ObservedAt:                time.Unix(1700000000, 0).UTC(),
		SubstrateEventCorrelation: "evt-123",
	}
}

func TestHostSnapshotWrappersAndMonitorLoop(t *testing.T) {
	host := &Host{
		managers: map[string]*Manager{
			"echo-b": {
				cfg:      Config{PluginID: "echo-b", HeartbeatEvery: time.Hour},
				state:    State{PluginID: "echo-b", GenerationID: 2, Healthy: true, Capabilities: []string{"shared", "zeta"}, SocketPath: "/runtime/echo-b.sock", PID: 222},
				closedCh: make(chan struct{}),
			},
			"echo-a": {
				cfg:      Config{PluginID: "echo-a", HeartbeatEvery: time.Hour},
				state:    State{PluginID: "echo-a", GenerationID: 1, Healthy: true, Capabilities: []string{"shared", "alpha"}, SocketPath: "/runtime/echo-a.sock", PID: 111},
				closedCh: make(chan struct{}),
			},
		},
		plugins: []string{"echo-a", "echo-b"},
	}

	caps := host.CapabilityMap()
	if got := caps["shared"]; len(got) != 2 || got[0] != "echo-a" || got[1] != "echo-b" {
		t.Fatalf("shared capability map = %v, want stable plugin ids", got)
	}
	caps["shared"][0] = "mutated"
	if got := host.CapabilityMap()["shared"][0]; got != "echo-a" {
		t.Fatalf("capability map was aliased through snapshot: %q", got)
	}

	states := host.States()
	if len(states) != 2 || states[0].PluginID != "echo-a" || states[1].PluginID != "echo-b" {
		t.Fatalf("states order = %+v, want stable plugin order", states)
	}
	states[0].Capabilities[0] = "mutated"
	if got := host.States()[0].Capabilities[0]; got != "shared" {
		t.Fatalf("states capabilities were aliased through snapshot: %q", got)
	}

	routes := host.Routes()
	if len(routes) != 2 || routes[0].PluginID != "echo-a" || routes[0].Mode != "direct-plugin-id" {
		t.Fatalf("routes snapshot = %+v", routes)
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	host.MonitorLoop(ctx)
}

func TestHostDirectRoutingErrors(t *testing.T) {
	host := &Host{
		managers: map[string]*Manager{
			"echo": {cfg: Config{PluginID: "echo", HeartbeatEvery: time.Hour}, state: State{PluginID: "echo", GenerationID: 7}, closedCh: make(chan struct{})},
		},
		plugins: []string{"echo"},
	}

	if _, err := host.Plugin("missing"); err == nil {
		t.Fatal("expected missing plugin snapshot error")
	}
	if _, _, err := host.RouteTarget("missing"); err == nil {
		t.Fatal("expected missing route target error")
	}
	if _, err := host.Echo("missing", "msg"); err == nil {
		t.Fatal("expected echo missing plugin error")
	}
	if _, err := host.Heartbeat("missing"); err == nil {
		t.Fatal("expected heartbeat missing plugin error")
	}
	if _, err := host.CallRequest(RoutedRequest{PluginID: "missing", Call: RoutedCallEcho, Arg: "msg"}); err == nil {
		t.Fatal("expected call request missing plugin error")
	}
	if _, err := host.CallRequest(RoutedRequest{PluginID: "echo", Call: RoutedCall("bogus")}); err == nil {
		t.Fatal("expected unsupported routed call error")
	}
	if _, err := host.CallRequest(RoutedRequest{PluginID: "echo", Call: RoutedCallEcho, Arg: "msg"}); err == nil {
		t.Fatal("expected echo on not-started manager to fail")
	}
	if _, err := host.CallRequest(RoutedRequest{PluginID: "echo", Call: RoutedCallHeartbeat}); err == nil {
		t.Fatal("expected heartbeat on not-started manager to fail")
	}
	if _, err := host.RestartPlugin("echo"); err == nil {
		t.Fatal("expected restart on invalid test manager to fail")
	}
}

func TestNewHostRejectsInvalidConfig(t *testing.T) {
	pluginBin := buildPlugin(t)
	runtimeDir := t.TempDir()

	cases := []struct {
		name string
		cfg  HostConfig
	}{
		{name: "missing runtime", cfg: HostConfig{Plugins: []PluginConfig{{PluginID: "echo", PluginPath: pluginBin}}}},
		{name: "missing plugins", cfg: HostConfig{RuntimeDir: runtimeDir}},
		{name: "invalid plugin id", cfg: HostConfig{RuntimeDir: runtimeDir, Plugins: []PluginConfig{{PluginID: "../bad", PluginPath: pluginBin}}}},
		{name: "missing plugin path", cfg: HostConfig{RuntimeDir: runtimeDir, Plugins: []PluginConfig{{PluginID: "echo"}}}},
		{name: "duplicate plugin id", cfg: HostConfig{RuntimeDir: runtimeDir, Plugins: []PluginConfig{{PluginID: "echo", PluginPath: pluginBin}, {PluginID: "echo", PluginPath: pluginBin}}}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if host, err := NewHost(tc.cfg); err == nil {
				_ = host.Close()
				t.Fatal("expected NewHost to reject invalid config")
			}
		})
	}
}

func TestHostCloseAndStartAllFailureBranches(t *testing.T) {
	closeRuntimeDir := t.TempDir()
	closeLogger, err := eventlog.New(filepath.Join(closeRuntimeDir, "events.jsonl"))
	if err != nil {
		t.Fatalf("new close logger: %v", err)
	}
	closedManager := &Manager{cfg: Config{PluginID: "closed", RuntimeDir: closeRuntimeDir}, closed: true, log: closeLogger, closedCh: make(chan struct{})}
	hostWithCloseError := &Host{managers: map[string]*Manager{"closed": closedManager}, plugins: []string{"closed"}}
	if err := hostWithCloseError.Close(); err == nil {
		t.Fatal("expected host close to return first manager close error")
	}

	pluginBin := buildPlugin(t)
	runtimeDir := t.TempDir()
	good, err := New(Config{
		RuntimeDir:   filepath.Join(runtimeDir, "good"),
		PluginPath:   pluginBin,
		PluginID:     "good",
		DialTimeout:  2 * time.Second,
		CallTimeout:  200 * time.Millisecond,
		EventLogPath: filepath.Join(runtimeDir, "good", "events.jsonl"),
	})
	if err != nil {
		t.Fatalf("new good manager: %v", err)
	}
	bad := &Manager{cfg: Config{PluginID: "bad"}, closed: true, closedCh: make(chan struct{})}
	hostWithStartError := &Host{managers: map[string]*Manager{"good": good, "bad": bad}, plugins: []string{"good", "bad"}}
	if err := hostWithStartError.StartAll(); err == nil {
		t.Fatal("expected StartAll to fail on second manager")
	}
	if state := good.State(); state.Healthy || state.PID != 0 {
		t.Fatalf("started manager was not rolled back after StartAll failure: %+v", state)
	}
	_ = good.Close()
}

func TestNewHostClosesExistingManagersOnLaterNewFailure(t *testing.T) {
	pluginBin := buildPlugin(t)
	runtimeDir := t.TempDir()
	notExecutable := filepath.Join(runtimeDir, "not-executable")
	if err := os.WriteFile(notExecutable, []byte("#!/bin/sh\nexit 0\n"), 0o600); err != nil {
		t.Fatalf("write non-executable plugin: %v", err)
	}
	if host, err := NewHost(HostConfig{RuntimeDir: runtimeDir, Plugins: []PluginConfig{{PluginID: "good", PluginPath: pluginBin}, {PluginID: "bad", PluginPath: notExecutable}}}); err == nil {
		_ = host.Close()
		t.Fatal("expected second manager construction failure")
	}
}

func TestHostRoutesByPluginID(t *testing.T) {
	pluginBin := buildPlugin(t)
	runtimeDir := t.TempDir()

	host, err := NewHost(HostConfig{
		RuntimeDir:     runtimeDir,
		DialTimeout:    2 * time.Second,
		CallTimeout:    200 * time.Millisecond,
		HeartbeatEvery: 100 * time.Millisecond,
		Plugins: []PluginConfig{
			{PluginID: "echo-a", PluginPath: pluginBin},
			{PluginID: "echo-b", PluginPath: pluginBin},
		},
	})
	if err != nil {
		t.Fatalf("new host: %v", err)
	}
	defer host.Close()

	if err := host.StartAll(); err != nil {
		t.Fatalf("start all: %v", err)
	}

	msg, err := host.Echo("echo-b", "hello-router")
	if err != nil {
		t.Fatalf("host echo route: %v", err)
	}
	if msg != "hello-router" {
		t.Fatalf("host echo route message = %q want %q", msg, "hello-router")
	}

	hb, err := host.Heartbeat("echo-a")
	if err != nil {
		t.Fatalf("host heartbeat route: %v", err)
	}
	if hb.PluginID != "echo-a" || hb.Status != testpluginapi.StatusHealthy {
		t.Fatalf("unexpected heartbeat route result: %+v", hb)
	}
}

func TestProviderBundleProjectionRequiresCurrentPluginGeneration(t *testing.T) {
	pluginBin := buildPlugin(t)
	runtimeDir := t.TempDir()
	metadata := kernelProviderBundleTestMetadata("echo", 2)

	host, err := NewHost(HostConfig{
		RuntimeDir:     runtimeDir,
		DialTimeout:    2 * time.Second,
		CallTimeout:    200 * time.Millisecond,
		HeartbeatEvery: 100 * time.Millisecond,
		Plugins: []PluginConfig{{
			PluginID:               "echo",
			PluginPath:             pluginBin,
			ProviderBundleMetadata: &metadata,
		}},
	})
	if err != nil {
		t.Fatalf("new host: %v", err)
	}
	defer host.Close()
	if err := host.StartAll(); err != nil {
		t.Fatalf("start all: %v", err)
	}

	adminState := host.State()
	if len(adminState.Plugins) != 1 || adminState.Plugins[0].DeclaredProviderBundleMetadata != nil {
		t.Fatalf("stale generation metadata projected to admin state: %#v", adminState)
	}
	core := host.CoreSnapshot()
	if len(core.Plugins) != 1 || core.Plugins[0].DeclaredProviderBundleMetadata != nil {
		t.Fatalf("stale generation metadata projected to core state: %#v", core)
	}
}
