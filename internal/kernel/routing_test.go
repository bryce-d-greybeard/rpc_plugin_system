package kernel

import (
	"testing"
	"time"

	"rpc_plugin_system/internal/testpluginapi"
)

func TestRouteTargetReturnsExplicitDirectRoute(t *testing.T) {
	echoPlugin := buildPlugin(t)
	runtimeDir := t.TempDir()

	host, err := NewHost(HostConfig{
		RuntimeDir:     runtimeDir,
		DialTimeout:    2 * time.Second,
		CallTimeout:    200 * time.Millisecond,
		HeartbeatEvery: 100 * time.Millisecond,
		Plugins:        []PluginConfig{{PluginID: "echo", PluginPath: echoPlugin}},
	})
	if err != nil {
		t.Fatalf("new host: %v", err)
	}
	defer host.Close()

	if err := host.StartAll(); err != nil {
		t.Fatalf("start all: %v", err)
	}

	route, _, err := host.RouteTarget("echo")
	if err != nil {
		t.Fatalf("route target: %v", err)
	}
	if route.PluginID != "echo" || route.Mode != "direct-plugin-id" || !route.Healthy || route.GenerationID == 0 {
		t.Fatalf("unexpected route target: %+v", route)
	}
}

func TestRoutingRemainsTargetedWhenOtherPluginBecomesUnhealthy(t *testing.T) {
	echoPlugin := buildPlugin(t)
	failurePlugin := buildFailurePlugin(t)
	runtimeDir := t.TempDir()
	oldEnv := setEnvMap(t, map[string]string{
		"RPC_PLUGIN_SYSTEM_PLUGIN_BEHAVIOR_CLOSE_ON_ECHO": "true",
	})
	defer restoreEnvMap(oldEnv)

	host, err := NewHost(HostConfig{
		RuntimeDir:     runtimeDir,
		DialTimeout:    2 * time.Second,
		CallTimeout:    200 * time.Millisecond,
		HeartbeatEvery: 100 * time.Millisecond,
		Plugins: []PluginConfig{
			{PluginID: "healthy", PluginPath: echoPlugin},
			{PluginID: "flaky", PluginPath: failurePlugin},
		},
	})
	if err != nil {
		t.Fatalf("new host: %v", err)
	}
	defer host.Close()

	if err := host.StartAll(); err != nil {
		t.Fatalf("start all: %v", err)
	}

	if _, err := host.Echo("flaky", "break-me"); err == nil {
		t.Fatal("expected flaky route to fail")
	}

	flaky, err := host.Plugin("flaky")
	if err != nil {
		t.Fatalf("plugin flaky: %v", err)
	}
	if flaky.Healthy {
		t.Fatal("flaky plugin should no longer be healthy after routed failure")
	}

	routes := host.Routes()
	if len(routes) != 2 {
		t.Fatalf("route count = %d, want 2", len(routes))
	}
	var healthyRoute, flakyRoute Route
	for _, route := range routes {
		switch route.PluginID {
		case "healthy":
			healthyRoute = route
		case "flaky":
			flakyRoute = route
		}
	}
	if !healthyRoute.Healthy {
		t.Fatalf("healthy route should remain healthy: %+v", healthyRoute)
	}
	if flakyRoute.Healthy {
		t.Fatalf("flaky route should be unhealthy after failure: %+v", flakyRoute)
	}

	msg, err := host.Echo("healthy", "still-works")
	if err != nil {
		t.Fatalf("healthy route should still work: %v", err)
	}
	if msg != "still-works" {
		t.Fatalf("healthy route message = %q want %q", msg, "still-works")
	}

	hb, err := host.Heartbeat("healthy")
	if err != nil {
		t.Fatalf("healthy heartbeat route should still work: %v", err)
	}
	if hb.PluginID != "healthy" || hb.Status != testpluginapi.StatusHealthy {
		t.Fatalf("unexpected healthy heartbeat route result: %+v", hb)
	}
}
