package eventlog

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"
	"time"
)

// Filters selects a subset of events during log reads.
type Filters struct {
	Level         string
	Component     string
	Event         string
	PluginID      string
	GenerationID  uint64
	Method        string
	CapabilityID  string
	OperationID   string
	CorrelationID string
	Limit         int
	Since         time.Time
	Reverse       bool
}

// Summary reports coarse counts for operator use.
type Summary struct {
	Total       int            `json:"total"`
	ByLevel     map[string]int `json:"by_level"`
	ByEvent     map[string]int `json:"by_event"`
	ByComponent map[string]int `json:"by_component"`
}

// ReadAll loads matching JSONL events from one log file and its rotated backups.
func ReadAll(path string, filters Filters) ([]Event, error) {
	paths, err := existingLogPaths(path)
	if err != nil {
		return nil, err
	}
	var events []Event
	for _, p := range paths {
		batch, err := readOne(p, filters)
		if err != nil {
			return nil, err
		}
		events = append(events, batch...)
	}
	if filters.Reverse {
		reverse(events)
	}
	if filters.Limit > 0 && len(events) > filters.Limit {
		events = events[:filters.Limit]
	}
	return events, nil
}

// Summarize returns counts for operator-facing summaries.
func Summarize(events []Event) Summary {
	summary := Summary{
		Total:       len(events),
		ByLevel:     map[string]int{},
		ByEvent:     map[string]int{},
		ByComponent: map[string]int{},
	}
	for _, event := range events {
		summary.ByLevel[event.Level]++
		summary.ByEvent[event.Event]++
		summary.ByComponent[event.Component]++
	}
	return summary
}

// FormatText renders one event into a concise human-readable line.
func FormatText(event Event) string {
	event = RedactForDisplay(event)
	parts := []string{event.Time.Format("2006-01-02 15:04:05Z07:00")}
	parts = append(parts, "["+strings.ToUpper(event.Level)+"]")
	if event.Component != "" {
		parts = append(parts, event.Component)
	}
	if event.Event != "" {
		parts = append(parts, event.Event)
	}
	if event.PluginID != "" {
		parts = append(parts, "plugin="+event.PluginID)
	}
	if event.GenerationID != 0 {
		parts = append(parts, fmt.Sprintf("gen=%d", event.GenerationID))
	}
	if event.PID != 0 {
		parts = append(parts, fmt.Sprintf("pid=%d", event.PID))
	}
	if event.Method != "" {
		parts = append(parts, "method="+event.Method)
	}
	if event.CapabilityID != "" {
		parts = append(parts, "capability="+event.CapabilityID)
	}
	if event.OperationID != "" {
		parts = append(parts, "operation="+event.OperationID)
	}
	if event.CorrelationID != "" {
		parts = append(parts, "correlation="+event.CorrelationID)
	}
	if event.Status != "" {
		parts = append(parts, "status="+event.Status)
	}
	if event.DurationMS != 0 {
		parts = append(parts, fmt.Sprintf("duration_ms=%d", event.DurationMS))
	}
	if event.ErrorClass != "" {
		parts = append(parts, "error_class="+event.ErrorClass)
	}
	if event.DegradedReason != "" {
		parts = append(parts, "degraded_reason="+event.DegradedReason)
	}
	if event.Message != "" {
		parts = append(parts, event.Message)
	}
	if event.Reason != "" {
		parts = append(parts, "reason="+event.Reason)
	}
	if event.Error != "" {
		parts = append(parts, "error="+event.Error)
	}
	return strings.Join(parts, " | ")
}

func readOne(path string, filters Filters) ([]Event, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("open event log: %w", err)
	}
	defer file.Close()

	var events []Event
	reader := bufio.NewReader(file)
	lineNo := 0
	for {
		line, err := reader.ReadBytes('\n')
		if err == io.EOF && len(line) == 0 {
			break
		}
		lineNo++
		if err != nil && err != io.EOF {
			return nil, fmt.Errorf("read event log %s:%d: %w", path, lineNo, err)
		}
		line = bytes.TrimRight(line, "\r\n")
		var event Event
		if err := json.Unmarshal(line, &event); err != nil {
			return nil, fmt.Errorf("decode event log %s:%d: %w", path, lineNo, err)
		}
		if !match(event, filters) {
			continue
		}
		events = append(events, event)
		if err == io.EOF {
			break
		}
	}
	return events, nil
}

func existingLogPaths(path string) ([]string, error) {
	var paths []string
	for i := DefaultMaxBackups; i >= 1; i-- {
		p := backupPath(path, i)
		if _, err := os.Stat(p); err == nil {
			paths = append(paths, p)
		}
	}
	if _, err := os.Stat(path); err == nil {
		paths = append(paths, path)
	}
	if len(paths) == 0 {
		return nil, fmt.Errorf("open event log: %w", os.ErrNotExist)
	}
	return paths, nil
}

func match(event Event, filters Filters) bool {
	if filters.Level != "" && event.Level != filters.Level {
		return false
	}
	if filters.Component != "" && event.Component != filters.Component {
		return false
	}
	if filters.Event != "" && event.Event != filters.Event {
		return false
	}
	if filters.PluginID != "" && event.PluginID != filters.PluginID {
		return false
	}
	if filters.GenerationID != 0 && event.GenerationID != filters.GenerationID {
		return false
	}
	if filters.Method != "" && event.Method != filters.Method {
		return false
	}
	if filters.CapabilityID != "" && event.CapabilityID != filters.CapabilityID {
		return false
	}
	if filters.OperationID != "" && event.OperationID != filters.OperationID {
		return false
	}
	if filters.CorrelationID != "" && event.CorrelationID != filters.CorrelationID {
		return false
	}
	if !filters.Since.IsZero() && event.Time.Before(filters.Since) {
		return false
	}
	return true
}

func reverse(events []Event) {
	for i, j := 0, len(events)-1; i < j; i, j = i+1, j-1 {
		events[i], events[j] = events[j], events[i]
	}
}

// RedactForDisplay returns an operator-display copy of event with unsafe provider text removed.
func RedactForDisplay(event Event) Event {
	if !isProviderObservationEvent(event) {
		return event
	}
	redact := func(value string) string {
		if value == "" {
			return ""
		}
		if UnsafeProviderText(value) || looksLikeProviderPrivatePath(value) {
			return ProviderRedactedValue
		}
		return value
	}
	event.SocketPath = redact(event.SocketPath)
	event.CapabilityID = redact(event.CapabilityID)
	event.OperationID = redact(event.OperationID)
	event.CorrelationID = redact(event.CorrelationID)
	event.ErrorClass = redact(event.ErrorClass)
	event.DegradedReason = redact(event.DegradedReason)
	event.Message = redact(event.Message)
	event.Error = redact(event.Error)
	event.Reason = redact(event.Reason)
	if len(event.Details) != 0 {
		if details, err := safeProviderDetails(event.Details); err == nil {
			event.Details = details
		} else {
			event.Details = map[string]any{"redacted": true}
		}
	}
	return event
}

func isProviderObservationEvent(event Event) bool {
	switch event.Component {
	case ComponentProviderBoundary, ComponentProviderDiagnostic:
		return true
	}
	switch event.Event {
	case EventProviderOperationStarted, EventProviderOperationSucceeded, EventProviderOperationFailed, EventProviderOperationDegraded, EventProviderOperationUnavailable, EventProviderDiagnosticReported, EventProviderDiagnosticRejected:
		return true
	}
	return false
}
