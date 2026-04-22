package kernel

import (
	"path/filepath"
	"testing"
	"time"

	"rpc_plugin_system/internal/testpluginapi"
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
