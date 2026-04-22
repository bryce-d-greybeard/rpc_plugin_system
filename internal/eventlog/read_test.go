package eventlog

import (
	"os"
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

func TestReadAllIncludesRotatedBackups(t *testing.T) {
	dir := t.TempDir()
	path := dir + "/events.jsonl"
	logger, err := NewWithOptions(path, Options{MaxBytes: 250, MaxBackups: 3})
	if err != nil {
		t.Fatalf("new logger: %v", err)
	}
	defer logger.Close()

	for i := 0; i < 12; i++ {
		if err := logger.Write(Event{Time: time.Unix(int64(i+1), 0).UTC(), Level: LevelInfo, Event: EventPluginStarted, PluginID: "echo", Message: strings.Repeat("x", 40)}); err != nil {
			t.Fatalf("write event %d: %v", i, err)
		}
	}

	if _, err := os.Stat(dir + "/events.1.jsonl"); err != nil {
		t.Fatalf("expected rotated backup: %v", err)
	}
	events, err := ReadAll(path, Filters{})
	if err != nil {
		t.Fatalf("read rotated logs: %v", err)
	}
	if len(events) == 0 {
		t.Fatal("expected events from rotated logs")
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

func TestSummarizeCountsByLevelAndEvent(t *testing.T) {
	summary := Summarize([]Event{{Level: LevelInfo, Component: ComponentKernel, Event: EventPluginStarted}, {Level: LevelWarn, Component: ComponentRPC, Event: EventRPCFailed}, {Level: LevelWarn, Component: ComponentRPC, Event: EventRPCFailed}})
	if summary.Total != 3 {
		t.Fatalf("total = %d, want 3", summary.Total)
	}
	if summary.ByLevel[LevelWarn] != 2 {
		t.Fatalf("warn count = %d, want 2", summary.ByLevel[LevelWarn])
	}
	if summary.ByEvent[EventRPCFailed] != 2 {
		t.Fatalf("event count = %d, want 2", summary.ByEvent[EventRPCFailed])
	}
}
