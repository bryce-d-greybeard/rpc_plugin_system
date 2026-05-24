package cli

import (
	"encoding/json"
	"fmt"
	"io"
	"sort"

	"rpc_plugin_system/internal/eventlog"
)

// WriteEventsText writes events as concise human-readable lines.
func WriteEventsText(w io.Writer, events []eventlog.Event) error {
	for _, event := range events {
		if _, err := fmt.Fprintln(w, eventlog.FormatText(event)); err != nil {
			return fmt.Errorf("write text event: %w", err)
		}
	}
	return nil
}

// WriteEventsJSON writes events as indented JSON.
func WriteEventsJSON(w io.Writer, events []eventlog.Event) error {
	events = redactEventsForDisplay(events)
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	if err := enc.Encode(events); err != nil {
		return fmt.Errorf("encode events: %w", err)
	}
	return nil
}

// WriteSummaryText writes a human-readable summary block.
func WriteSummaryText(w io.Writer, summary eventlog.Summary) error {
	if _, err := fmt.Fprintf(w, "total: %d\n", summary.Total); err != nil {
		return fmt.Errorf("write summary total: %w", err)
	}
	for _, section := range []struct {
		name   string
		counts map[string]int
	}{
		{name: "levels", counts: summary.ByLevel},
		{name: "components", counts: summary.ByComponent},
		{name: "events", counts: summary.ByEvent},
	} {
		if _, err := fmt.Fprintf(w, "%s:\n", section.name); err != nil {
			return fmt.Errorf("write summary section: %w", err)
		}
		keys := make([]string, 0, len(section.counts))
		for key := range section.counts {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		for _, key := range keys {
			if _, err := fmt.Fprintf(w, "  %s: %d\n", key, section.counts[key]); err != nil {
				return fmt.Errorf("write summary count: %w", err)
			}
		}
	}
	return nil
}

// WriteSummaryJSON writes a summary as indented JSON.
func WriteSummaryJSON(w io.Writer, summary eventlog.Summary) error {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	if err := enc.Encode(summary); err != nil {
		return fmt.Errorf("encode summary: %w", err)
	}
	return nil
}

func redactEventsForDisplay(events []eventlog.Event) []eventlog.Event {
	if len(events) == 0 {
		return events
	}
	redacted := make([]eventlog.Event, len(events))
	for i, event := range events {
		redacted[i] = eventlog.RedactForDisplay(event)
	}
	return redacted
}
