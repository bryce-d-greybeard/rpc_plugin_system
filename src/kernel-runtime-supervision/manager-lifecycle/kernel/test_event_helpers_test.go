package kernel

import (
	"bufio"
	"encoding/json"
	"os"
	"testing"

	"rpc_plugin_system/control-plane-ops/event-log-write/eventlog"
)

func readEvents(t *testing.T, path string) []eventlog.Event {
	t.Helper()
	file, err := os.Open(path)
	if err != nil {
		t.Fatalf("open event log %s: %v", path, err)
	}
	defer file.Close()

	var events []eventlog.Event
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		var event eventlog.Event
		if err := json.Unmarshal(scanner.Bytes(), &event); err != nil {
			t.Fatalf("decode event log %s: %v", path, err)
		}
		events = append(events, event)
	}
	if err := scanner.Err(); err != nil {
		t.Fatalf("scan event log %s: %v", path, err)
	}
	return events
}

func assertEvent(t *testing.T, events []eventlog.Event, want string, predicate func(eventlog.Event) bool) {
	t.Helper()
	for _, event := range events {
		if event.Event == want && (predicate == nil || predicate(event)) {
			return
		}
	}
	t.Fatalf("missing event %q", want)
}

func countEvent(events []eventlog.Event, want string, predicate func(eventlog.Event) bool) int {
	count := 0
	for _, event := range events {
		if event.Event == want && (predicate == nil || predicate(event)) {
			count++
		}
	}
	return count
}
