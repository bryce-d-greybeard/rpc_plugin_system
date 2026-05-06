package cli

import (
	"bytes"
	"errors"
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

func TestLogWritersReturnWriteErrors(t *testing.T) {
	wantErr := errors.New("pipe closed")
	event := eventlog.Event{Time: time.Unix(1, 0).UTC(), Level: eventlog.LevelInfo, Component: eventlog.ComponentKernel, Event: eventlog.EventPluginStarted}
	summary := eventlog.Summary{Total: 1, ByLevel: map[string]int{eventlog.LevelInfo: 1}, ByComponent: map[string]int{eventlog.ComponentKernel: 1}, ByEvent: map[string]int{eventlog.EventPluginStarted: 1}}
	for _, tt := range []struct {
		name string
		run  func() error
		want string
	}{
		{name: "events text", run: func() error { return WriteEventsText(errorWriter{err: wantErr}, []eventlog.Event{event}) }, want: "write text event"},
		{name: "events json", run: func() error { return WriteEventsJSON(errorWriter{err: wantErr}, []eventlog.Event{event}) }, want: "encode events"},
		{name: "summary text total", run: func() error { return WriteSummaryText(errorWriter{err: wantErr}, summary) }, want: "write summary total"},
		{name: "summary json", run: func() error { return WriteSummaryJSON(errorWriter{err: wantErr}, summary) }, want: "encode summary"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.run()
			if err == nil {
				t.Fatalf("expected error")
			}
			if !strings.Contains(err.Error(), tt.want) || !errors.Is(err, wantErr) {
				t.Fatalf("error = %v, want wrapped %q", err, tt.want)
			}
		})
	}
}

func TestWriteSummaryTextSortsKeys(t *testing.T) {
	var buf bytes.Buffer
	summary := eventlog.Summary{
		Total:       2,
		ByLevel:     map[string]int{eventlog.LevelWarn: 1, eventlog.LevelInfo: 1},
		ByComponent: map[string]int{"zeta": 1, "alpha": 1},
		ByEvent:     map[string]int{"z_event": 1, "a_event": 1},
	}
	if err := WriteSummaryText(&buf, summary); err != nil {
		t.Fatalf("write summary text: %v", err)
	}
	out := buf.String()
	for _, pair := range [][2]string{{"  info: 1", "  warn: 1"}, {"  alpha: 1", "  zeta: 1"}, {"  a_event: 1", "  z_event: 1"}} {
		if strings.Index(out, pair[0]) > strings.Index(out, pair[1]) {
			t.Fatalf("expected %q before %q in %q", pair[0], pair[1], out)
		}
	}
}

func TestWriteSummaryTextReturnsSectionAndCountErrors(t *testing.T) {
	wantErr := errors.New("short write")
	summary := eventlog.Summary{Total: 1, ByLevel: map[string]int{eventlog.LevelInfo: 1}}
	for _, tt := range []struct {
		name      string
		allow     int
		wantLabel string
	}{
		{name: "section", allow: 1, wantLabel: "write summary section"},
		{name: "count", allow: 2, wantLabel: "write summary count"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			err := WriteSummaryText(&failAfterWriter{remaining: tt.allow, err: wantErr}, summary)
			if err == nil {
				t.Fatalf("expected error")
			}
			if !strings.Contains(err.Error(), tt.wantLabel) || !errors.Is(err, wantErr) {
				t.Fatalf("error = %v, want wrapped %q", err, tt.wantLabel)
			}
		})
	}
}
