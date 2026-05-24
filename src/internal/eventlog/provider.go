package eventlog

import (
	"fmt"
	"path/filepath"
	"regexp"
	"strings"
	"unicode"
)

const (
	ComponentProviderBoundary   = "provider_boundary"
	ComponentProviderDiagnostic = "provider_diagnostic"
)

const (
	EventProviderOperationStarted     = "provider_operation_started"
	EventProviderOperationSucceeded   = "provider_operation_succeeded"
	EventProviderOperationFailed      = "provider_operation_failed"
	EventProviderOperationDegraded    = "provider_operation_degraded"
	EventProviderOperationUnavailable = "provider_operation_unavailable"
	EventProviderDiagnosticReported   = "provider_diagnostic_reported"
	EventProviderDiagnosticRejected   = "provider_diagnostic_rejected"
)

const (
	ProviderStatusStarted     = "started"
	ProviderStatusSucceeded   = "succeeded"
	ProviderStatusFailed      = "failed"
	ProviderStatusDegraded    = "degraded"
	ProviderStatusUnavailable = "unavailable"
	ProviderStatusRejected    = "rejected"
)

var providerPluginIDPattern = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9._-]*$`)

var providerEventStatus = map[string]string{
	EventProviderOperationStarted:     ProviderStatusStarted,
	EventProviderOperationSucceeded:   ProviderStatusSucceeded,
	EventProviderOperationFailed:      ProviderStatusFailed,
	EventProviderOperationDegraded:    ProviderStatusDegraded,
	EventProviderOperationUnavailable: ProviderStatusUnavailable,
	EventProviderDiagnosticRejected:   ProviderStatusRejected,
}

var providerStatuses = map[string]struct{}{
	ProviderStatusStarted:     {},
	ProviderStatusSucceeded:   {},
	ProviderStatusFailed:      {},
	ProviderStatusDegraded:    {},
	ProviderStatusUnavailable: {},
	ProviderStatusRejected:    {},
}

var unsafeDetailMarkers = []string{
	"raw_prompt",
	"prompt",
	"payload",
	"request_body",
	"response_body",
	"body",
	"credential",
	"credentials",
	"secret",
	"client_secret",
	"token",
	"access_token",
	"refresh_token",
	"password",
	"passwd",
	"api_key",
	"apikey",
	"private_key",
	"privatekey",
	"bearer",
	"authorization",
	"jwt",
	"oauth",
	"authority",
	"authority_ref",
	"authority-use-ref",
	"authority_use_ref",
	"use_ref",
	"handle",
	"socket",
	"session",
	"signer",
	"path",
	"filepath",
	"file_path",
	"absolute_path",
}

// ProviderObservation is the typed input for additive provider observability events.
type ProviderObservation struct {
	Level          string
	Component      string
	Event          string
	PluginID       string
	GenerationID   uint64
	CapabilityID   string
	OperationID    string
	CorrelationID  string
	Status         string
	DurationMS     int64
	ErrorClass     string
	DegradedReason string
	Message        string
	Details        map[string]any
}

// ProviderEvent validates and converts a provider observation into the durable event shape.
func ProviderEvent(obs ProviderObservation) (Event, error) {
	if err := validateProviderObservation(obs); err != nil {
		return Event{}, err
	}
	details, err := safeProviderDetails(obs.Details)
	if err != nil {
		return Event{}, err
	}
	level := obs.Level
	if level == "" {
		level = LevelInfo
	}
	return Event{
		Level:          level,
		Component:      obs.Component,
		Event:          obs.Event,
		PluginID:       obs.PluginID,
		GenerationID:   obs.GenerationID,
		CapabilityID:   obs.CapabilityID,
		OperationID:    obs.OperationID,
		CorrelationID:  obs.CorrelationID,
		Status:         obs.Status,
		DurationMS:     obs.DurationMS,
		ErrorClass:     obs.ErrorClass,
		DegradedReason: obs.DegradedReason,
		Message:        obs.Message,
		Details:        details,
	}, nil
}

func validateProviderObservation(obs ProviderObservation) error {
	if !providerPluginIDPattern.MatchString(obs.PluginID) {
		return fmt.Errorf("invalid plugin id %q", obs.PluginID)
	}
	if obs.GenerationID == 0 {
		return fmt.Errorf("generation id is required")
	}
	if err := requireProviderField("capability id", obs.CapabilityID); err != nil {
		return err
	}
	if err := requireProviderField("operation id", obs.OperationID); err != nil {
		return err
	}
	if err := requireProviderField("correlation id", obs.CorrelationID); err != nil {
		return err
	}
	for _, field := range []struct {
		name  string
		value string
	}{
		{"capability id", obs.CapabilityID},
		{"operation id", obs.OperationID},
		{"correlation id", obs.CorrelationID},
		{"error class", obs.ErrorClass},
		{"degraded reason", obs.DegradedReason},
		{"message", obs.Message},
	} {
		if err := validateSafeProviderText(field.name, field.value); err != nil {
			return err
		}
	}
	if obs.DurationMS < 0 {
		return fmt.Errorf("duration_ms must be non-negative")
	}
	if _, ok := providerStatuses[obs.Status]; !ok {
		return fmt.Errorf("unknown provider status %q", obs.Status)
	}
	if err := validateProviderEventComponent(obs.Component, obs.Event, obs.Status); err != nil {
		return err
	}
	return nil
}

func requireProviderField(name, value string) error {
	if strings.TrimSpace(value) == "" {
		return fmt.Errorf("%s is required", name)
	}
	return validateSafeProviderText(name, value)
}

func validateSafeProviderText(name, value string) error {
	if strings.TrimSpace(value) == "" {
		return nil
	}
	if strings.IndexFunc(value, unicode.IsControl) >= 0 {
		return fmt.Errorf("%s contains control characters", name)
	}
	if unsafeProviderDetailText(value) || looksLikeProviderPrivatePath(value) {
		return fmt.Errorf("unsafe provider %s", name)
	}
	return nil
}

func validateProviderEventComponent(component, event, status string) error {
	switch event {
	case EventProviderOperationStarted, EventProviderOperationSucceeded, EventProviderOperationFailed, EventProviderOperationDegraded, EventProviderOperationUnavailable:
		if component != ComponentProviderBoundary {
			return fmt.Errorf("provider boundary event %q requires component %q", event, ComponentProviderBoundary)
		}
		if want := providerEventStatus[event]; status != want {
			return fmt.Errorf("provider event %q requires status %q", event, want)
		}
	case EventProviderDiagnosticReported:
		if component != ComponentProviderDiagnostic {
			return fmt.Errorf("provider diagnostic event %q requires component %q", event, ComponentProviderDiagnostic)
		}
	case EventProviderDiagnosticRejected:
		if component != ComponentProviderDiagnostic {
			return fmt.Errorf("provider diagnostic event %q requires component %q", event, ComponentProviderDiagnostic)
		}
		if status != ProviderStatusRejected {
			return fmt.Errorf("provider event %q requires status %q", event, ProviderStatusRejected)
		}
	default:
		return fmt.Errorf("unknown provider event %q", event)
	}
	return nil
}

func safeProviderDetails(details map[string]any) (map[string]any, error) {
	if len(details) == 0 {
		return nil, nil
	}
	safe := make(map[string]any, len(details))
	for key, value := range details {
		if unsafeProviderDetailText(key) {
			return nil, fmt.Errorf("unsafe provider detail key %q", key)
		}
		if err := validateProviderDetailValue(key, value); err != nil {
			return nil, err
		}
		safe[key] = value
	}
	return safe, nil
}

func validateProviderDetailValue(key string, value any) error {
	switch v := value.(type) {
	case nil, bool, float64, float32, int, int8, int16, int32, int64, uint, uint8, uint16, uint32, uint64:
		return nil
	case string:
		if unsafeProviderDetailText(v) || looksLikeProviderPrivatePath(v) {
			return fmt.Errorf("unsafe provider detail value for key %q", key)
		}
		return nil
	case []string:
		for _, item := range v {
			if unsafeProviderDetailText(item) || looksLikeProviderPrivatePath(item) {
				return fmt.Errorf("unsafe provider detail value for key %q", key)
			}
		}
		return nil
	case []any:
		for _, item := range v {
			if err := validateProviderDetailValue(key, item); err != nil {
				return err
			}
		}
		return nil
	case map[string]any:
		_, err := safeProviderDetails(v)
		return err
	default:
		return fmt.Errorf("unsupported provider detail value for key %q", key)
	}
}

func unsafeProviderDetailText(text string) bool {
	normalized := strings.ToLower(strings.NewReplacer("-", "_", " ", "_", ".", "_", ":", "_").Replace(text))
	for _, marker := range unsafeDetailMarkers {
		if strings.Contains(normalized, strings.ToLower(marker)) {
			return true
		}
	}
	return false
}

func looksLikeProviderPrivatePath(text string) bool {
	trimmed := strings.TrimSpace(text)
	if trimmed == "" {
		return false
	}
	if filepath.IsAbs(trimmed) {
		return true
	}
	return len(trimmed) >= 3 && unicode.IsLetter(rune(trimmed[0])) && trimmed[1] == ':' && (trimmed[2] == '\\' || trimmed[2] == '/')
}
