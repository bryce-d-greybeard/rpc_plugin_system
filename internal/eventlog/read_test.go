package eventlog

import (
	"strings"
	"testing"
	"time"
)

func TestReadAllFiltersEvents(t *testing.T) {
	path := t.TempDir() + "/events.jsonl"
	logger, err := New(path)
	if err != nil {
		t.Fatalf("new logger: %v", err)
	}
	defer logger.Close()

	for _, event := range []Event{
		{Time: time.Unix(1, 0).UTC(), Level: LevelInfo, Component: ComponentKernel, Event: EventPluginStarted, PluginID: "echo"},
		{Time: time.Unix(2, 0).UTC(), Level: LevelWarn, Component: ComponentRPC, Event: EventRPCFailed, PluginID: "echo", Method: "TestPlugin.Echo"},
		{Time: time.Unix(3, 0).UTC(), Level: LevelError, Component: ComponentAuth, Event: EventAuthFailed, PluginID: "failure"},
	} {
		if err := logger.Write(event); err != nil {
			t.Fatalf("write event: %v", err)
		}
	}

	events, err := ReadAll(path, Filters{Level: LevelWarn, PluginID: "echo"})
	if err != nil {
		t.Fatalf("read all: %v", err)
	}
	if len(events) != 1 {
		t.Fatalf("len(events) = %d, want 1", len(events))
	}
	if events[0].Event != EventRPCFailed {
		t.Fatalf("event = %q, want %q", events[0].Event, EventRPCFailed)
	}
}

func TestFormatTextIncludesUsefulFields(t *testing.T) {
	line := FormatText(Event{
		Time:         time.Date(2026, 4, 17, 9, 0, 0, 0, time.UTC),
		Level:        LevelWarn,
		Component:    ComponentKernel,
		Event:        EventRestartRequested,
		PluginID:     "echo",
		GenerationID: 3,
		PID:          123,
		Method:       "TestPlugin.Heartbeat",
		Message:      "restart requested",
		Reason:       "monitor loop",
		Error:        "boom",
	})
	for _, want := range []string{"[WARN]", "plugin=echo", "gen=3", "pid=123", "reason=monitor loop", "error=boom"} {
		if !strings.Contains(line, want) {
			t.Fatalf("formatted line missing %q: %s", want, line)
		}
	}
}
