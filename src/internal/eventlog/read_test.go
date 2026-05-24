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

func TestReadAllFiltersProviderFieldsAndGeneration(t *testing.T) {
	path := t.TempDir() + "/events.jsonl"
	logger, err := New(path)
	if err != nil {
		t.Fatalf("new logger: %v", err)
	}
	defer logger.Close()
	for _, event := range []Event{
		{Time: time.Unix(1, 0).UTC(), Level: LevelInfo, Component: ComponentProviderDiagnostic, Event: EventProviderDiagnosticReported, PluginID: "provider.echo", GenerationID: 7, CapabilityID: "mail.send", OperationID: "send_message", CorrelationID: "corr-123", Status: ProviderStatusDegraded},
		{Time: time.Unix(2, 0).UTC(), Level: LevelInfo, Component: ComponentProviderDiagnostic, Event: EventProviderDiagnosticReported, PluginID: "provider.echo", GenerationID: 8, CapabilityID: "mail.read", OperationID: "read_message", CorrelationID: "corr-456", Status: ProviderStatusSucceeded},
	} {
		if err := logger.Write(event); err != nil {
			t.Fatalf("write event: %v", err)
		}
	}

	events, err := ReadAll(path, Filters{PluginID: "provider.echo", GenerationID: 7, CapabilityID: "mail.send", OperationID: "send_message", CorrelationID: "corr-123"})
	if err != nil {
		t.Fatalf("ReadAll: %v", err)
	}
	if len(events) != 1 || events[0].GenerationID != 7 || events[0].CapabilityID != "mail.send" {
		t.Fatalf("events = %#v, want generation 7 mail.send only", events)
	}
}

func TestRedactForDisplayRedactsUnsafeProviderFieldsAndDetails(t *testing.T) {
	event := Event{
		Time:           time.Unix(1, 0).UTC(),
		Level:          LevelWarn,
		Component:      ComponentProviderDiagnostic,
		Event:          EventProviderDiagnosticReported,
		PluginID:       "provider.echo",
		GenerationID:   7,
		SocketPath:     "/tmp/provider/private.sock",
		CapabilityID:   "mail.payload",
		OperationID:    "send_message",
		CorrelationID:  "authority_ref:abc",
		Status:         ProviderStatusFailed,
		ErrorClass:     "raw response_body",
		DegradedReason: "/home/provider/private",
		Message:        "bearer token omitted",
		Details:        map[string]any{"payload": "secret"},
	}
	redacted := RedactForDisplay(event)
	for name, value := range map[string]string{
		"socket_path":     redacted.SocketPath,
		"capability":      redacted.CapabilityID,
		"correlation":     redacted.CorrelationID,
		"error_class":     redacted.ErrorClass,
		"degraded_reason": redacted.DegradedReason,
		"message":         redacted.Message,
	} {
		if value != "[redacted]" {
			t.Fatalf("%s = %q, want redacted", name, value)
		}
	}
	if redacted.OperationID != "send_message" || redacted.Status != ProviderStatusFailed {
		t.Fatalf("safe fields changed: %#v", redacted)
	}
	if got, ok := redacted.Details["redacted"].(bool); !ok || !got {
		t.Fatalf("details = %#v, want redacted marker", redacted.Details)
	}
	line := FormatText(event)
	for _, forbidden := range []string{"/tmp/provider/private.sock", "mail.payload", "authority_ref", "response_body", "/home/provider/private", "bearer token", "secret"} {
		if strings.Contains(line, forbidden) {
			t.Fatalf("line %q contains forbidden %q", line, forbidden)
		}
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

func TestReadAllHandlesLongJSONLLines(t *testing.T) {
	path := t.TempDir() + "/events.jsonl"
	logger, err := New(path)
	if err != nil {
		t.Fatalf("new logger: %v", err)
	}
	defer logger.Close()

	message := strings.Repeat("x", 70<<10)
	if err := logger.Write(Event{Time: time.Unix(1, 0).UTC(), Level: LevelInfo, Event: EventPluginStarted, Message: message}); err != nil {
		t.Fatalf("write long event: %v", err)
	}

	events, err := ReadAll(path, Filters{})
	if err != nil {
		t.Fatalf("read long event: %v", err)
	}
	if len(events) != 1 {
		t.Fatalf("len(events) = %d, want 1", len(events))
	}
	if events[0].Message != message {
		t.Fatalf("message length = %d, want %d", len(events[0].Message), len(message))
	}
}

func TestReadAllReportsCorruptLinePathAndLine(t *testing.T) {
	path := t.TempDir() + "/events.jsonl"
	content := `{"time":"1970-01-01T00:00:01Z","level":"info","component":"kernel","event":"plugin_started"}` + "\n" +
		`{"time":"broken"` + "\n"
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("write corrupt log: %v", err)
	}

	_, err := ReadAll(path, Filters{})
	if err == nil {
		t.Fatal("expected corrupt line error")
	}
	text := err.Error()
	for _, want := range []string{path, ":2", "decode event log"} {
		if !strings.Contains(text, want) {
			t.Fatalf("error %q missing %q", text, want)
		}
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
