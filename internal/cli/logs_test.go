package cli

import (
	"bytes"
	"strings"
	"testing"
	"time"

	"rpc_plugin_system/internal/eventlog"
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
