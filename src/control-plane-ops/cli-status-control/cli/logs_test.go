package cli

import (
	"bytes"
	"strings"
	"testing"
	"time"

	"rpc_plugin_system/control-plane-ops/event-log-write/eventlog"
)

func TestWriteEventsText(t *testing.T) {
	var buf bytes.Buffer
	events := []eventlog.Event{{Time: time.Unix(1, 0).UTC(), Level: eventlog.LevelInfo, Component: eventlog.ComponentKernel, Event: eventlog.EventPluginStarted, PluginID: "echo", Message: "started"}}
	if err := WriteEventsText(&buf, events); err != nil {
		t.Fatalf("write text: %v", err)
	}
	out := buf.String()
	for _, want := range []string{"[INFO]", "plugin=echo", "started"} {
		if !strings.Contains(out, want) {
			t.Fatalf("text output missing %q in %q", want, out)
		}
	}
}

func TestWriteEventsJSON(t *testing.T) {
	var buf bytes.Buffer
	events := []eventlog.Event{{Time: time.Unix(1, 0).UTC(), Level: eventlog.LevelInfo, Component: eventlog.ComponentKernel, Event: eventlog.EventPluginStarted, PluginID: "echo"}}
	if err := WriteEventsJSON(&buf, events); err != nil {
		t.Fatalf("write json: %v", err)
	}
	out := buf.String()
	for _, want := range []string{"\"event\": \"plugin_started\"", "\"plugin_id\": \"echo\""} {
		if !strings.Contains(out, want) {
			t.Fatalf("json output missing %q in %q", want, out)
		}
	}
}

func TestWriteSummaryText(t *testing.T) {
	var buf bytes.Buffer
	summary := eventlog.Summary{Total: 3, ByLevel: map[string]int{eventlog.LevelInfo: 1, eventlog.LevelWarn: 2}, ByComponent: map[string]int{eventlog.ComponentRPC: 2}, ByEvent: map[string]int{eventlog.EventRPCFailed: 2}}
	if err := WriteSummaryText(&buf, summary); err != nil {
		t.Fatalf("write summary text: %v", err)
	}
	out := buf.String()
	for _, want := range []string{"total: 3", "levels:", "warn: 2", "events:", "rpc_call_failed: 2"} {
		if !strings.Contains(out, want) {
			t.Fatalf("summary text missing %q in %q", want, out)
		}
	}
}

func TestWriteSummaryJSON(t *testing.T) {
	var buf bytes.Buffer
	summary := eventlog.Summary{Total: 2, ByLevel: map[string]int{eventlog.LevelInfo: 2}, ByComponent: map[string]int{eventlog.ComponentKernel: 2}, ByEvent: map[string]int{eventlog.EventPluginStarted: 2}}
	if err := WriteSummaryJSON(&buf, summary); err != nil {
		t.Fatalf("write summary json: %v", err)
	}
	out := buf.String()
	for _, want := range []string{"\"total\": 2", "\"plugin_started\": 2"} {
		if !strings.Contains(out, want) {
			t.Fatalf("summary json missing %q in %q", want, out)
		}
	}
}
