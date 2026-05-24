package eventlog

import (
	"encoding/json"
	"strings"
	"testing"
	"time"
)

func validProviderObservation() ProviderObservation {
	return ProviderObservation{
		Level:          LevelWarn,
		Component:      ComponentProviderBoundary,
		Event:          EventProviderOperationFailed,
		PluginID:       "provider.echo",
		GenerationID:   7,
		CapabilityID:   "mail.send",
		OperationID:    "send_message",
		CorrelationID:  "corr-123",
		Status:         ProviderStatusFailed,
		DurationMS:     42,
		ErrorClass:     "upstream_unavailable",
		DegradedReason: "rate_limited",
		Message:        "provider operation failed",
		Details:        map[string]any{"attempt": 2, "retryable": true, "region": "us-central"},
	}
}

func TestProviderBoundaryEventSerializesAndReadsTypedFields(t *testing.T) {
	obs := validProviderObservation()
	event, err := ProviderEvent(obs)
	if err != nil {
		t.Fatalf("ProviderEvent: %v", err)
	}
	if event.Component != ComponentProviderBoundary || event.Event != EventProviderOperationFailed || event.PluginID != obs.PluginID || event.GenerationID != obs.GenerationID || event.CapabilityID != obs.CapabilityID || event.OperationID != obs.OperationID || event.CorrelationID != obs.CorrelationID || event.Status != obs.Status || event.DurationMS != obs.DurationMS || event.ErrorClass != obs.ErrorClass || event.DegradedReason != obs.DegradedReason {
		t.Fatalf("event fields not preserved: %#v", event)
	}
	payload, err := json.Marshal(event)
	if err != nil {
		t.Fatalf("marshal provider event: %v", err)
	}
	for _, want := range []string{`"capability_id":"mail.send"`, `"operation_id":"send_message"`, `"correlation_id":"corr-123"`, `"status":"failed"`, `"duration_ms":42`, `"error_class":"upstream_unavailable"`, `"degraded_reason":"rate_limited"`} {
		if !strings.Contains(string(payload), want) {
			t.Fatalf("payload %s missing %s", payload, want)
		}
	}

	path := t.TempDir() + "/events.jsonl"
	logger, err := New(path)
	if err != nil {
		t.Fatalf("new logger: %v", err)
	}
	defer logger.Close()
	if err := logger.Write(event); err != nil {
		t.Fatalf("write provider event: %v", err)
	}
	events, err := ReadAll(path, Filters{})
	if err != nil {
		t.Fatalf("read provider event: %v", err)
	}
	if len(events) != 1 {
		t.Fatalf("len(events) = %d, want 1", len(events))
	}
	if got := events[0]; got.CapabilityID != obs.CapabilityID || got.OperationID != obs.OperationID || got.CorrelationID != obs.CorrelationID || got.Status != obs.Status || got.DurationMS != obs.DurationMS || got.ErrorClass != obs.ErrorClass || got.DegradedReason != obs.DegradedReason {
		t.Fatalf("read event fields not preserved: %#v", got)
	}
}

func TestProviderDiagnosticEventIsDistinctFromBoundaryEvent(t *testing.T) {
	obs := validProviderObservation()
	obs.Component = ComponentProviderDiagnostic
	obs.Event = EventProviderDiagnosticReported
	obs.Status = ProviderStatusDegraded
	event, err := ProviderEvent(obs)
	if err != nil {
		t.Fatalf("ProviderEvent diagnostic: %v", err)
	}
	if event.Component != ComponentProviderDiagnostic || event.Event != EventProviderDiagnosticReported {
		t.Fatalf("diagnostic event not distinct: %#v", event)
	}

	obs.Component = ComponentProviderBoundary
	if _, err := ProviderEvent(obs); err == nil {
		t.Fatal("expected diagnostic event with boundary component to be rejected")
	}
}

func TestProviderBoundaryEventsRequireMatchingStatuses(t *testing.T) {
	tests := []struct {
		event  string
		status string
	}{
		{EventProviderOperationStarted, ProviderStatusStarted},
		{EventProviderOperationSucceeded, ProviderStatusSucceeded},
		{EventProviderOperationFailed, ProviderStatusFailed},
		{EventProviderOperationDegraded, ProviderStatusDegraded},
		{EventProviderOperationUnavailable, ProviderStatusUnavailable},
	}
	for _, tc := range tests {
		t.Run(tc.event, func(t *testing.T) {
			obs := validProviderObservation()
			obs.Event = tc.event
			obs.Status = tc.status
			if _, err := ProviderEvent(obs); err != nil {
				t.Fatalf("ProviderEvent(%s/%s): %v", tc.event, tc.status, err)
			}

			obs.Status = ProviderStatusFailed
			if obs.Status == tc.status {
				obs.Status = ProviderStatusStarted
			}
			if _, err := ProviderEvent(obs); err == nil || !strings.Contains(err.Error(), "requires status") {
				t.Fatalf("mismatched status err = %v, want requires status", err)
			}
		})
	}
}

func TestProviderDiagnosticReportedAcceptsBoundedStatuses(t *testing.T) {
	for _, status := range []string{ProviderStatusStarted, ProviderStatusSucceeded, ProviderStatusFailed, ProviderStatusDegraded, ProviderStatusUnavailable, ProviderStatusRejected} {
		t.Run(status, func(t *testing.T) {
			obs := validProviderObservation()
			obs.Component = ComponentProviderDiagnostic
			obs.Event = EventProviderDiagnosticReported
			obs.Status = status
			if _, err := ProviderEvent(obs); err != nil {
				t.Fatalf("ProviderEvent diagnostic status %s: %v", status, err)
			}
		})
	}
}

func TestProviderEventRejectsUnknownProviderEvent(t *testing.T) {
	obs := validProviderObservation()
	obs.Event = "provider_operation_confused"
	if _, err := ProviderEvent(obs); err == nil || !strings.Contains(err.Error(), "unknown provider event") {
		t.Fatalf("err = %v, want unknown provider event", err)
	}
}

func TestProviderEventRejectsUnknownStatus(t *testing.T) {
	obs := validProviderObservation()
	obs.Status = "confused"
	if _, err := ProviderEvent(obs); err == nil || !strings.Contains(err.Error(), "unknown provider status") {
		t.Fatalf("err = %v, want unknown provider status", err)
	}
}

func TestProviderEventRejectsMissingRequiredIdentityFields(t *testing.T) {
	tests := []struct {
		name string
		mut  func(*ProviderObservation)
		want string
	}{
		{"plugin id", func(obs *ProviderObservation) { obs.PluginID = "" }, "invalid plugin id"},
		{"plugin id path", func(obs *ProviderObservation) { obs.PluginID = "../provider" }, "invalid plugin id"},
		{"generation", func(obs *ProviderObservation) { obs.GenerationID = 0 }, "generation id is required"},
		{"capability", func(obs *ProviderObservation) { obs.CapabilityID = "" }, "capability id is required"},
		{"operation", func(obs *ProviderObservation) { obs.OperationID = "" }, "operation id is required"},
		{"correlation", func(obs *ProviderObservation) { obs.CorrelationID = "" }, "correlation id is required"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			obs := validProviderObservation()
			tc.mut(&obs)
			_, err := ProviderEvent(obs)
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("err = %v, want substring %q", err, tc.want)
			}
		})
	}
}

func TestProviderEventRejectsNegativeDuration(t *testing.T) {
	obs := validProviderObservation()
	obs.DurationMS = -1
	if _, err := ProviderEvent(obs); err == nil || !strings.Contains(err.Error(), "duration_ms") {
		t.Fatalf("err = %v, want duration_ms rejection", err)
	}
}

func TestProviderEventRejectsUnsafeDetailKeysAndValues(t *testing.T) {
	tests := []struct {
		name    string
		details map[string]any
	}{
		{"payload key", map[string]any{"payload": "safe-ish"}},
		{"secret key", map[string]any{"api_key": "safe-ish"}},
		{"authority key", map[string]any{"authority_ref": "safe-ish"}},
		{"path key", map[string]any{"private_path": "safe-ish"}},
		{"payload value", map[string]any{"note": "raw payload omitted"}},

		{"secret value", map[string]any{"note": "bearer token abc"}},
		{"authority value", map[string]any{"note": "authority_ref:abc"}},
		{"path value", map[string]any{"note": "/home/provider/private"}},
		{"nested unsafe", map[string]any{"outer": map[string]any{"session": "abc"}}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			obs := validProviderObservation()
			obs.Details = tc.details
			if _, err := ProviderEvent(obs); err == nil {
				t.Fatal("expected unsafe details to be rejected")
			}
		})
	}
}

func TestProviderFiltersByCapabilityOperationCorrelation(t *testing.T) {
	path := t.TempDir() + "/events.jsonl"
	logger, err := New(path)
	if err != nil {
		t.Fatalf("new logger: %v", err)
	}
	defer logger.Close()

	base := validProviderObservation()
	for _, variant := range []struct {
		capability  string
		operation   string
		correlation string
	}{
		{"mail.send", "send_message", "corr-123"},
		{"mail.read", "read_message", "corr-456"},
	} {
		obs := base
		obs.CapabilityID = variant.capability
		obs.OperationID = variant.operation
		obs.CorrelationID = variant.correlation
		event, err := ProviderEvent(obs)
		if err != nil {
			t.Fatalf("ProviderEvent: %v", err)
		}
		if err := logger.Write(event); err != nil {
			t.Fatalf("write event: %v", err)
		}
	}

	for _, filters := range []Filters{
		{CapabilityID: "mail.send"},
		{OperationID: "send_message"},
		{CorrelationID: "corr-123"},
		{CapabilityID: "mail.send", OperationID: "send_message", CorrelationID: "corr-123"},
	} {
		events, err := ReadAll(path, filters)
		if err != nil {
			t.Fatalf("ReadAll(%#v): %v", filters, err)
		}
		if len(events) != 1 {
			t.Fatalf("ReadAll(%#v) len = %d, want 1", filters, len(events))
		}
		if events[0].CapabilityID != "mail.send" || events[0].OperationID != "send_message" || events[0].CorrelationID != "corr-123" {
			t.Fatalf("wrong filtered event: %#v", events[0])
		}
	}
}

func TestFormatTextIncludesProviderFields(t *testing.T) {
	line := FormatText(Event{
		Time:           time.Date(2026, 5, 23, 22, 0, 0, 0, time.UTC),
		Level:          LevelWarn,
		Component:      ComponentProviderBoundary,
		Event:          EventProviderOperationDegraded,
		PluginID:       "provider.echo",
		GenerationID:   7,
		CapabilityID:   "mail.send",
		OperationID:    "send_message",
		CorrelationID:  "corr-123",
		Status:         ProviderStatusDegraded,
		DurationMS:     12,
		ErrorClass:     "upstream_unavailable",
		DegradedReason: "rate_limited",
	})
	for _, want := range []string{"capability=mail.send", "operation=send_message", "correlation=corr-123", "status=degraded", "duration_ms=12", "error_class=upstream_unavailable", "degraded_reason=rate_limited"} {
		if !strings.Contains(line, want) {
			t.Fatalf("formatted line missing %q: %s", want, line)
		}
	}
}
