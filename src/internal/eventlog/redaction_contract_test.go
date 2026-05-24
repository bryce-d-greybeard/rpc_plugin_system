package eventlog

import "testing"

func TestUnsafeProviderTextNormalizesUnsafeMarkersDeterministically(t *testing.T) {
	unsafe := []string{
		"raw-prompt",
		"provider payload",
		"request.body",
		"response:body",
		"client-secret",
		"ACCESS_TOKEN",
		"private key",
		"bearer credential",
		"authority-use-ref",
		"authority.ref",
		"use ref",
		"file handle",
		"browser session",
		"socket signer",
		"absolute.path",
	}
	for _, text := range unsafe {
		t.Run(text, func(t *testing.T) {
			if !UnsafeProviderText(text) {
				t.Fatalf("UnsafeProviderText(%q) = false, want true", text)
			}
		})
	}

	safe := []string{
		"mail.send",
		"send_message",
		"corr-123",
		"upstream_unavailable",
		"rate_limited",
		"digest sha256:abcdef",
		"count 42",
	}
	for _, text := range safe {
		t.Run(text, func(t *testing.T) {
			if UnsafeProviderText(text) {
				t.Fatalf("UnsafeProviderText(%q) = true, want false", text)
			}
		})
	}
}

func TestProviderRedactionContractAcrossWriteAndDisplay(t *testing.T) {
	obs := ProviderObservation{
		Level:         LevelWarn,
		Component:     ComponentProviderDiagnostic,
		Event:         EventProviderDiagnosticReported,
		PluginID:      "provider.echo",
		GenerationID:  7,
		CapabilityID:  "mail.send",
		OperationID:   "send_message",
		CorrelationID: "corr-123",
		Status:        ProviderStatusFailed,
		ErrorClass:    "upstream_unavailable",
		Message:       "provider failed",
		Details:       map[string]any{"digest": "sha256:abcdef", "count": 2},
	}
	if _, err := ProviderEvent(obs); err != nil {
		t.Fatalf("safe ProviderEvent: %v", err)
	}

	for _, mutate := range []func(*ProviderObservation){
		func(o *ProviderObservation) { o.CapabilityID = "mail.payload" },
		func(o *ProviderObservation) { o.OperationID = "send_token" },
		func(o *ProviderObservation) { o.CorrelationID = "authority-ref:abc" },
		func(o *ProviderObservation) { o.ErrorClass = "raw response.body" },
		func(o *ProviderObservation) { o.DegradedReason = "/home/provider/private" },
		func(o *ProviderObservation) { o.Message = "bearer credential omitted" },
		func(o *ProviderObservation) { o.Details = map[string]any{"session_handle": "abc"} },
	} {
		bad := obs
		bad.Details = map[string]any{"digest": "sha256:abcdef", "count": 2}
		mutate(&bad)
		if _, err := ProviderEvent(bad); err == nil {
			t.Fatalf("ProviderEvent accepted unsafe observation: %#v", bad)
		}
	}

	display := RedactForDisplay(Event{
		Component:     ComponentProviderDiagnostic,
		Event:         EventProviderDiagnosticReported,
		PluginID:      "provider.echo",
		GenerationID:  7,
		SocketPath:    "/run/provider/private.sock",
		CapabilityID:  "mail.payload",
		OperationID:   "send_message",
		CorrelationID: "authority_ref:abc",
		Status:        ProviderStatusFailed,
		Message:       "response_body omitted",
		Details:       map[string]any{"payload": "secret"},
	})
	for name, value := range map[string]string{
		"socket":      display.SocketPath,
		"capability":  display.CapabilityID,
		"correlation": display.CorrelationID,
		"message":     display.Message,
	} {
		if value != ProviderRedactedValue {
			t.Fatalf("%s = %q, want %q", name, value, ProviderRedactedValue)
		}
	}
	if got, ok := display.Details["redacted"].(bool); !ok || !got {
		t.Fatalf("display details = %#v, want redacted marker", display.Details)
	}
}
