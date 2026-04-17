package eventlog

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"strings"
)

// Filters selects a subset of events during log reads.
type Filters struct {
	Level     string
	Component string
	Event     string
	PluginID  string
	Method    string
	Limit     int
}

// ReadAll loads matching JSONL events from one log file.
func ReadAll(path string, filters Filters) ([]Event, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("open event log: %w", err)
	}
	defer file.Close()

	var events []Event
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		var event Event
		if err := json.Unmarshal(scanner.Bytes(), &event); err != nil {
			return nil, fmt.Errorf("decode event log line: %w", err)
		}
		if !match(event, filters) {
			continue
		}
		events = append(events, event)
		if filters.Limit > 0 && len(events) >= filters.Limit {
			break
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("scan event log: %w", err)
	}
	return events, nil
}

// FormatText renders one event into a concise human-readable line.
func FormatText(event Event) string {
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
	if filters.Method != "" && event.Method != filters.Method {
		return false
	}
	return true
}
