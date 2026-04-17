package eventlog

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"sort"
	"strings"
	"time"
)

// Filters selects a subset of events during log reads.
type Filters struct {
	Level     string
	Component string
	Event     string
	PluginID  string
	Method    string
	Limit     int
	Since     time.Time
	Reverse   bool
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

func readOne(path string, filters Filters) ([]Event, error) {
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
	}
	if err := scanner.Err(); err != nil && err != io.EOF {
		return nil, fmt.Errorf("scan event log: %w", err)
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
	sort.Strings(paths)
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
	if filters.Method != "" && event.Method != filters.Method {
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
