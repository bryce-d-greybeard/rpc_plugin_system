package testpluginapi

import (
	"reflect"
	"testing"
)

func TestStatusValuesRemainStable(t *testing.T) {
	got := []Status{
		StatusStarting,
		StatusHealthy,
		StatusDegraded,
		StatusOverloaded,
		StatusUnhealthy,
	}

	want := []Status{"starting", "healthy", "degraded", "overloaded", "unhealthy"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("status constants changed: got %v want %v", got, want)
	}
}

func TestServiceMethodNamesRemainStable(t *testing.T) {
	got := []string{
		MethodCapabilities,
		MethodHeartbeat,
		MethodEcho,
		MethodSleep,
		MethodCrash,
		MethodShutdown,
	}

	want := []string{
		"TestPlugin.Capabilities",
		"TestPlugin.Heartbeat",
		"TestPlugin.Echo",
		"TestPlugin.Sleep",
		"TestPlugin.Crash",
		"TestPlugin.Shutdown",
	}

	if !reflect.DeepEqual(got, want) {
		t.Fatalf("method names changed: got %v want %v", got, want)
	}
}
